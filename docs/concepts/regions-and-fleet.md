# Regions and Fleet

## Concepts

### VM
Any Linux machine reachable over SSH. Could be a VPS, a bare-metal server, a cloud VM,
or a Raspberry Pi. Bordo has no cloud-provider dependencies.

### Region
A named group of one or more VMs running a single k3s cluster. Regions are logical —
you define the name; Bordo doesn't prescribe geography.

Example regions: `us-east-1`, `eu-west-1`, `home-lab`, `staging`.

### Cluster
A k3s Kubernetes cluster. One per region. Bordo bootstraps it automatically when you
add a region with `bordo region add`.

### Fleet
The set of all regions registered with a Bordo control plane. The fleet controller
watches cluster health and syncs desired workload state across regions.

### Node
A VM that has joined a regional cluster. A region can grow from one node (the control
node) to many worker nodes.

## Adding a region

```bash
# Add a region — Bordo SSHes in and installs k3s
bordo region add \
  --name us-east-1 \
  --host 1.2.3.4 \
  --key ~/.ssh/id_ed25519 \
  --user ubuntu

# Check fleet status
bordo region list
```

What happens during `region add`:
1. Bordo opens an SSH connection to the target VM.
2. Installs Docker (if absent) and k3s as the Kubernetes distribution.
3. Retrieves the cluster kubeconfig and stores it encrypted in the control-plane state.
4. Registers the region in the project registry.
5. Deploys the Bordo fleet agent (a Kubernetes DaemonSet) to the cluster.
6. The region appears in `bordo region list` as `healthy`.

## Multi-region deployments

Projects can be deployed to multiple regions simultaneously:

```bash
# Deploy to all regions
bordo deploy my-api v1.2.3 --region all

# Deploy to specific regions
bordo deploy my-api v1.2.3 --region us-east-1,eu-west-1

# Sequential rollout (safer for breaking changes)
bordo deploy my-api v1.2.3 --region us-east-1 --then eu-west-1 --wait-healthy
```

## Growing a region

Add more VMs to scale a region horizontally:

```bash
bordo node add \
  --region us-east-1 \
  --host 5.6.7.8 \
  --key ~/.ssh/id_ed25519 \
  --role worker
```

## Fleet health

```bash
bordo region list         # all regions + health
bordo region status us-east-1   # detailed status for one region
```

Or in chat: `show fleet status`.
