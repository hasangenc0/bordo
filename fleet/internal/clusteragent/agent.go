// Package clusteragent implements the Bordo on-cluster reconciler (ADR-0006
// Phase 2). It runs INSIDE a region's k3s cluster as a Deployment, authenticates
// to the control plane with the region's cluster token, PULLS desired state, and
// applies it using the pod's in-cluster RBAC (a ServiceAccount scoped to the
// bordo-apps namespace) — never a cluster-admin kubeconfig held by the control
// plane.
package clusteragent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	defaultInterval  = 30 * time.Second
	defaultNamespace = "bordo-apps"
)

// Revision identifies what this reconciler build can do. It is sent with every pull so the control
// plane can tell when a region is running an older reconciler than the platform expects. Bump it
// (and control-plane/internal/fleet.ExpectedReconcilerRevision with it) whenever a change here is
// something the control plane depends on.
//
//	1  reports its revision; restarts itself when the control plane asks
const Revision = 1

// revisionHeader carries Revision on the desired-state pull.
const revisionHeader = "X-Bordo-Reconciler-Revision"

// Config is the reconciler's runtime configuration (from env, set by its
// Deployment manifest).
type Config struct {
	CPURL        string        // BORDO_CP_URL — control-plane URL reachable from the cluster
	ClusterToken string        // BORDO_CLUSTER_TOKEN — region-scoped reconciler credential
	Interval     time.Duration // BORDO_RECONCILE_INTERVAL seconds (default 30)
	Namespace    string        // BORDO_APPS_NAMESPACE (default bordo-apps)
	// DetectPublicIP (BORDO_DETECT_PUBLIC_IP, default on) lets the reconciler ask an IP-echo
	// service for the machine's public address so the control plane can build NodePort URLs.
	DetectPublicIP bool
}

// ConfigFromEnv builds and validates a Config from environment variables.
func ConfigFromEnv() (Config, error) {
	c := Config{
		CPURL:        strings.TrimRight(os.Getenv("BORDO_CP_URL"), "/"),
		ClusterToken: os.Getenv("BORDO_CLUSTER_TOKEN"),
		Interval:     defaultInterval,
		Namespace:    defaultNamespace,

		DetectPublicIP: !strings.EqualFold(os.Getenv("BORDO_DETECT_PUBLIC_IP"), "false"),
	}
	if v := os.Getenv("BORDO_RECONCILE_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.Interval = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("BORDO_APPS_NAMESPACE"); v != "" {
		c.Namespace = v
	}
	if c.CPURL == "" {
		return c, errors.New("BORDO_CP_URL is required")
	}
	if c.ClusterToken == "" {
		return c, errors.New("BORDO_CLUSTER_TOKEN is required")
	}
	return c, nil
}

// Applier applies a bundle of Kubernetes manifests. It is injectable so the
// reconcile logic can be unit-tested without a cluster.
type Applier func(ctx context.Context, namespace, manifests string) error

// Observer reads what is actually running in the namespace after an apply.
// Injectable for tests.
type Observer func(ctx context.Context, namespace string) (*Observation, error)

// Observation is a snapshot of the namespace's services and pods: the facts the
// control plane cannot know on its own (assigned nodePorts, pod readiness,
// image pull failures).
type Observation struct {
	Services []ServiceInfo `json:"services"`
	Pods     []PodInfo     `json:"pods"`
}

// ServiceInfo is one Service; NodePorts is set for NodePort/LoadBalancer services.
type ServiceInfo struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Port      int    `json:"port"`
	NodePorts []int  `json:"node_ports,omitempty"`
}

// PodInfo is one pod's state. Reason carries the container waiting reason
// (ImagePullBackOff, CrashLoopBackOff, ...) when it is not running.
type PodInfo struct {
	Name   string `json:"name"`
	Phase  string `json:"phase"`
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`
	// Message is the container's waiting message, e.g. the registry's pull error.
	Message string `json:"message,omitempty"`
	Image   string `json:"image,omitempty"` // first container's image, to tell a new rollout from the old pod
}

// Agent is the on-cluster reconciler.
type Agent struct {
	cfg     Config
	httpc   *http.Client
	apply   Applier
	observe Observer

	ipURLs    []string
	ipCached  string
	ipChecked time.Time
}

// publicIPServices are queried in order; each returns the caller's address as plain text.
var publicIPServices = []string{"https://api.ipify.org", "https://ifconfig.me/ip"}

const publicIPRefresh = time.Hour

// publicIP returns the machine's public address as seen from outside, cached for an hour.
// Pod egress leaves through the host's network, so an IP-echo service reports the machine's
// address. Returns "" when detection is off or fails; it is only a convenience.
func (a *Agent) publicIP(ctx context.Context) string {
	if !a.cfg.DetectPublicIP {
		return ""
	}
	if a.ipCached != "" && time.Since(a.ipChecked) < publicIPRefresh {
		return a.ipCached
	}
	if a.ipCached == "" && !a.ipChecked.IsZero() && time.Since(a.ipChecked) < 5*time.Minute {
		return "" // recently failed; don't hammer the echo services
	}
	a.ipChecked = time.Now()
	for _, u := range a.ipURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		ip := strings.TrimSpace(string(b))
		if resp.StatusCode == http.StatusOK && net.ParseIP(ip) != nil {
			a.ipCached = ip
			return ip
		}
	}
	return a.ipCached
}

// New builds an Agent that applies via kubectl using the pod's in-cluster RBAC.
func New(cfg Config) *Agent {
	return &Agent{
		cfg:     cfg,
		httpc:   &http.Client{Timeout: 30 * time.Second},
		apply:   kubectlApply,
		observe: kubectlObserve,
		ipURLs:  publicIPServices,
	}
}

// desiredState is the control plane's answer to a pull.
type desiredState struct {
	Region     string   `json:"region"`
	Manifests  string   `json:"manifests"`
	ReleaseIDs []string `json:"release_ids"`
	// Restart asks the reconciler to exit so the cluster starts it again from its current image.
	Restart bool `json:"restart"`
}

// fetchDesiredState pulls the region's manifests from the control plane, along
// with the IDs of the releases included in the bundle (so the apply result can
// be reported back).
func (a *Agent) fetchDesiredState(ctx context.Context) (string, []string, error) {
	ds, err := a.pullDesiredState(ctx)
	if err != nil {
		return "", nil, err
	}
	return ds.Manifests, ds.ReleaseIDs, nil
}

func (a *Agent) pullDesiredState(ctx context.Context) (desiredState, error) {
	var out desiredState
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.cfg.CPURL+"/fleet/desired-state", nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.ClusterToken)
	req.Header.Set(revisionHeader, strconv.Itoa(Revision))
	resp, err := a.httpc.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return out, fmt.Errorf("desired-state: %s", e.Error)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}

// reportStatus tells the control plane which releases this cycle applied, so it
// can advance them from reconciling to deployed (or failed). Best-effort: a
// reporting hiccup must not stop the reconcile loop, so errors are only logged.
func (a *Agent) reportStatus(ctx context.Context, releaseIDs []string, applyErr error, obs *Observation, publicIP string) {
	body := struct {
		AppliedReleaseIDs []string     `json:"applied_release_ids"`
		Error             string       `json:"error"`
		Observed          *Observation `json:"observed,omitempty"`
		PublicIP          string       `json:"public_ip,omitempty"`
	}{AppliedReleaseIDs: releaseIDs, Observed: obs, PublicIP: publicIP}
	if applyErr != nil {
		body.Error = applyErr.Error()
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.CPURL+"/fleet/status", bytes.NewReader(buf))
	if err != nil {
		fmt.Printf("[bordo-reconciler] status report: %v\n", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.ClusterToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpc.Do(req)
	if err != nil {
		fmt.Printf("[bordo-reconciler] status report: %v\n", err)
		return
	}
	resp.Body.Close()
}

// ReconcileOnce pulls desired state and applies it. Empty desired state is a
// no-op (nothing to run yet), not an error. After applying, it reports the
// result so the control plane can advance the releases' status.
func (a *Agent) ReconcileOnce(ctx context.Context) error {
	ds, err := a.pullDesiredState(ctx)
	if err != nil {
		return err
	}
	if ds.Restart {
		return ErrRestart
	}
	manifests, releaseIDs := ds.Manifests, ds.ReleaseIDs
	if strings.TrimSpace(manifests) == "" {
		return nil
	}
	applyErr := a.apply(ctx, a.cfg.Namespace, manifests)
	var obs *Observation
	if a.observe != nil {
		o, oerr := a.observe(ctx, a.cfg.Namespace)
		if oerr != nil {
			fmt.Printf("[bordo-reconciler] observe: %v\n", oerr)
		} else {
			obs = o
		}
	}
	if len(releaseIDs) > 0 || obs != nil {
		a.reportStatus(ctx, releaseIDs, applyErr, obs, a.publicIP(ctx))
	}
	return applyErr
}

// ErrRestart is returned by ReconcileOnce when the control plane asked this reconciler to restart.
var ErrRestart = errors.New("restart requested by the control plane")

// Run reconciles immediately, then on every interval until ctx is cancelled. A
// failed cycle is logged and retried next tick rather than killing the loop. When the control
// plane asks for a restart (`bordo region upgrade`), Run returns nil: the process exits cleanly
// and the Deployment starts it again, pulling the current image.
func (a *Agent) Run(ctx context.Context) error {
	fmt.Printf("[bordo-reconciler] revision %d starting; pulling desired state from %s every %s\n", Revision, a.cfg.CPURL, a.cfg.Interval)
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()
	for {
		if err := a.ReconcileOnce(ctx); errors.Is(err, ErrRestart) {
			fmt.Println("[bordo-reconciler] restart requested; exiting so the cluster starts the latest image")
			return nil
		} else if err != nil && ctx.Err() == nil {
			fmt.Printf("[bordo-reconciler] reconcile error: %v\n", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// kubectlApply pipes manifests to `kubectl apply`. In-cluster, kubectl uses the
// pod's ServiceAccount automatically (no kubeconfig), so RBAC is enforced by the
// cluster, scoped to the namespace the SA is bound to.
func kubectlApply(ctx context.Context, namespace, manifests string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-n", namespace, "-f", "-",
		// Delete Bordo-managed Deployments/Services that are no longer in the desired state
		// (superseded or removed releases). Only objects labelled bordo.io/managed are touched,
		// and only these two kinds; the reconciler's Role grants nothing broader.
		"--prune", "-l", "bordo.io/managed=true",
		"--prune-allowlist=apps/v1/Deployment", "--prune-allowlist=core/v1/Service")
	cmd.Stdin = strings.NewReader(manifests)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply: %w\n%s", err, out.String())
	}
	return nil
}

// kubectlObserve lists the namespace's services and pods. It needs only the
// get/list on services and pods the reconciler's Role already grants.
func kubectlObserve(ctx context.Context, namespace string) (*Observation, error) {
	cmd := exec.CommandContext(ctx, "kubectl", "get", "services,pods", "-n", namespace, "-o", "json")
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("kubectl get: %w: %s", err, strings.TrimSpace(errOut.String()))
	}
	return parseObservation(out.Bytes())
}

func parseObservation(raw []byte) (*Observation, error) {
	var list struct {
		Items []struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				Type       string `json:"type"`
				Containers []struct {
					Image string `json:"image"`
				} `json:"containers"`
				Ports []struct {
					Port     int `json:"port"`
					NodePort int `json:"nodePort"`
				} `json:"ports"`
			} `json:"spec"`
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					Ready bool `json:"ready"`
					State struct {
						Waiting *struct {
							Reason  string `json:"reason"`
							Message string `json:"message"`
						} `json:"waiting"`
						Terminated *struct {
							Reason string `json:"reason"`
						} `json:"terminated"`
					} `json:"state"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	obs := &Observation{}
	for _, it := range list.Items {
		switch it.Kind {
		case "Service":
			svc := ServiceInfo{Name: it.Metadata.Name, Type: it.Spec.Type}
			for _, p := range it.Spec.Ports {
				if svc.Port == 0 {
					svc.Port = p.Port
				}
				if p.NodePort != 0 {
					svc.NodePorts = append(svc.NodePorts, p.NodePort)
				}
			}
			obs.Services = append(obs.Services, svc)
		case "Pod":
			pod := PodInfo{Name: it.Metadata.Name, Phase: it.Status.Phase, Ready: len(it.Status.ContainerStatuses) > 0}
			if len(it.Spec.Containers) > 0 {
				pod.Image = it.Spec.Containers[0].Image
			}
			for _, cs := range it.Status.ContainerStatuses {
				if !cs.Ready {
					pod.Ready = false
				}
				if pod.Reason == "" && cs.State.Waiting != nil {
					pod.Reason = cs.State.Waiting.Reason
					pod.Message = truncate(cs.State.Waiting.Message, 400)
				}
				if pod.Reason == "" && cs.State.Terminated != nil {
					pod.Reason = cs.State.Terminated.Reason
				}
			}
			obs.Pods = append(obs.Pods, pod)
		}
	}
	return obs, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
