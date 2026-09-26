// Package controller implements the multi-region fleet controller (BRD-012).
//
// The Controller manages one watcher goroutine per registered regional cluster.
// Each watcher polls the k3s API server (/api/v1/nodes) every 30 seconds using
// a TLS HTTP client built from the cluster's certificate credentials. Node Ready
// status is reported to the control-plane via POST /v1/fleet/regions/{name}/health.
//
// On cluster unreachability the watcher backs off exponentially (up to 5 min)
// before resuming the normal poll cycle.
//
// See tracker/done/BRD-012-multi-region-fleet-controller.md.
package controller
