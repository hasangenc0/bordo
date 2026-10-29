package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// oauthDeviceURL starts GitHub's device flow; a variable so tests can point it elsewhere.
var oauthDeviceURL = "https://github.com/login/device/code"

// UserScopes are the OAuth scopes Bordo needs from a user login: create and push repos,
// manage the build workflow, and push GHCR images, and let the cluster pull them (read:packages).
const UserScopes = "repo workflow write:packages read:packages"

// DeviceStart is what the user needs to authorize a device flow: open VerificationURI and
// type UserCode.
type DeviceStart struct {
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceStatus is the state of the most recent device flow.
type DeviceStatus struct {
	// Status is idle, pending, connected, expired, denied or error.
	Status  string `json:"status"`
	Login   string `json:"login,omitempty"`
	Warning string `json:"warning,omitempty"`
	Error   string `json:"error,omitempty"`
}

// DeviceFlow runs GitHub's device flow on the server, so a browser (which cannot call
// github.com's device endpoints because of CORS) can connect GitHub through the console.
// One flow is active at a time; starting another replaces it.
type DeviceFlow struct {
	store *Store

	mu     sync.Mutex
	status DeviceStatus
	cancel context.CancelFunc
}

// NewDeviceFlow returns a DeviceFlow that saves the resulting login in store.
func NewDeviceFlow(store *Store) *DeviceFlow {
	return &DeviceFlow{store: store, status: DeviceStatus{Status: "idle"}}
}

// Status returns the state of the most recent flow.
func (d *DeviceFlow) Status() DeviceStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.status
}

func (d *DeviceFlow) set(st DeviceStatus) {
	d.mu.Lock()
	d.status = st
	d.mu.Unlock()
}

// Start requests a device code from GitHub and polls for authorization in the background.
func (d *DeviceFlow) Start(clientID string) (*DeviceStart, error) {
	form := url.Values{"client_id": {clientID}, "scope": {UserScopes}}
	var da struct {
		DeviceStart
		DeviceCode string `json:"device_code"`
		Error      string `json:"error"`
		ErrorDesc  string `json:"error_description"`
	}
	if err := postForm(oauthDeviceURL, form, &da); err != nil {
		return nil, err
	}
	if da.DeviceCode == "" {
		msg := da.ErrorDesc
		if msg == "" {
			msg = "check the client id and that \"Enable Device Flow\" is on for the OAuth App"
		}
		return nil, fmt.Errorf("GitHub did not start the device flow: %s", msg)
	}
	if da.Interval <= 0 {
		da.Interval = 5
	}
	if da.ExpiresIn <= 0 {
		da.ExpiresIn = 900
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(da.ExpiresIn)*time.Second)
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
	}
	d.cancel = cancel
	d.status = DeviceStatus{Status: "pending"}
	d.mu.Unlock()

	go d.poll(ctx, cancel, clientID, da.DeviceCode, da.Interval)
	return &da.DeviceStart, nil
}

func (d *DeviceFlow) poll(ctx context.Context, cancel context.CancelFunc, clientID, deviceCode string, interval int) {
	defer cancel()
	// A newer flow replaces d.status; only report for this one while it is still current.
	current := func() bool { return ctx.Err() != context.Canceled }
	for {
		select {
		case <-ctx.Done():
			if current() {
				d.set(DeviceStatus{Status: "expired", Error: "the code expired, start again"})
			}
			return
		case <-time.After(time.Duration(interval) * time.Second):
		}
		form := url.Values{
			"client_id":   {clientID},
			"device_code": {deviceCode},
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		}
		var r struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
			Error        string `json:"error"`
			ErrorDesc    string `json:"error_description"`
			Interval     int    `json:"interval"`
		}
		if err := postForm(oauthTokenURL, form, &r); err != nil {
			continue // transient network error: keep polling until the code expires
		}
		if r.AccessToken != "" {
			if ctx.Err() != nil {
				return
			}
			sess := UserSession{Token: r.AccessToken, RefreshToken: r.RefreshToken, ClientID: clientID, ExpiresAt: ExpiryFrom(r.ExpiresIn)}
			if err := d.store.SaveUserSession(context.Background(), sess); err != nil {
				d.set(DeviceStatus{Status: "error", Error: "saving the GitHub login: " + err.Error()})
				return
			}
			login, _ := GetCurrentUserLogin(r.AccessToken)
			d.set(DeviceStatus{Status: "connected", Login: login, Warning: PullTokenProblem(r.AccessToken)})
			return
		}
		switch r.Error {
		case "authorization_pending", "":
		case "slow_down":
			if r.Interval > 0 {
				interval = r.Interval
			} else {
				interval += 5
			}
		case "expired_token":
			d.set(DeviceStatus{Status: "expired", Error: "the code expired, start again"})
			return
		case "access_denied":
			d.set(DeviceStatus{Status: "denied", Error: "authorization was denied"})
			return
		default:
			d.set(DeviceStatus{Status: "error", Error: strings.TrimSpace(r.Error + " " + r.ErrorDesc)})
			return
		}
	}
}

func postForm(endpoint string, form url.Values, out any) error {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}
