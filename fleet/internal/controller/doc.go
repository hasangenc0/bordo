// Package controller will implement the multi-region fleet controller.
//
// Responsibilities (BRD-012):
//   - Maintain a client-go shared informer per registered regional cluster
//   - Detect node join/leave events and propagate health to the control plane
//   - Reconcile desired workload state in each cluster
//   - Auto-reconnect on cluster unreachability
//
// See tracker/backlog/BRD-012-multi-region-fleet-controller.md.
package controller
