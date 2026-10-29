package fleet

// ExpectedReconcilerRevision is the reconciler revision this control plane was built to work
// with. It mirrors clusteragent.Revision in the fleet module (a separate Go module, so it cannot
// be imported); bump both together. A reconciler reporting a lower number, or none, is outdated.
const ExpectedReconcilerRevision = 1
