# On-Prem — Creating an RKE2 Cluster

Simple, command-based guide to stand up an air-gap-capable RKE2 cluster on bare
metal or VMs, with a GPU-ready worker node, for LLM inference workloads.

## Prerequisites

- One or more Linux servers (Ubuntu 22.04/24.04 or RHEL/Rocky 9 recommended), each
  reachable over SSH with root/sudo access
- Network connectivity between all nodes on at least: `6443/tcp` (Kubernetes API),
  `9345/tcp` (RKE2 supervisor), `10250/tcp` (kubelet), and `8472/udp` (Flannel VXLAN,
  if using the default CNI)
- For GPU nodes: the NVIDIA driver and container toolkit already installed on the
  host OS (see [NVIDIA's driver install docs](https://docs.nvidia.com/datacenter/tesla/tesla-installation-notes/index.html))
  — RKE2 itself does not install GPU drivers
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) installed on your workstation

## 1. Install the first server (control plane) node

SSH into the node that will be the first control-plane node, then:

```bash
curl -sfL https://get.rke2.io | sh -

systemctl enable rke2-server.service
systemctl start rke2-server.service
```

Watch the logs while it comes up:

```bash
journalctl -u rke2-server -f
```

RKE2 installs its own bundled `kubectl`/`crictl` under `/var/lib/rancher/rke2/bin` —
add it to `PATH`, or just use your workstation's own `kubectl` once you have the
kubeconfig (step 3).

```bash
export PATH="$PATH:/var/lib/rancher/rke2/bin"
```

## 2. Get the join token

Still on the first server node:

```bash
cat /var/lib/rancher/rke2/server/node-token
```

Copy this value — it's needed to join every additional node.

## 3. Fetch the kubeconfig to your workstation

```bash
scp <user>@<first-node-ip>:/etc/rancher/rke2/rke2.yaml ~/.kube/rke2-cluster.yaml

# The file points at 127.0.0.1 by default — point it at the real node address:
sed -i "s/127.0.0.1/<first-node-ip>/" ~/.kube/rke2-cluster.yaml

export KUBECONFIG=~/.kube/rke2-cluster.yaml
kubectl get nodes
```

## 4. Join additional server nodes (optional, for HA control plane)

On each additional server node:

```bash
curl -sfL https://get.rke2.io | sh -

mkdir -p /etc/rancher/rke2
cat <<EOF > /etc/rancher/rke2/config.yaml
server: https://<first-node-ip>:9345
token: <node-token-from-step-2>
EOF

systemctl enable rke2-server.service
systemctl start rke2-server.service
```

## 5. Join worker nodes

On each worker node:

```bash
curl -sfL https://get.rke2.io | INSTALL_RKE2_TYPE="agent" sh -

mkdir -p /etc/rancher/rke2
cat <<EOF > /etc/rancher/rke2/config.yaml
server: https://<first-node-ip>:9345
token: <node-token-from-step-2>
EOF

systemctl enable rke2-agent.service
systemctl start rke2-agent.service
```

## 6. Verify all nodes joined

From your workstation:

```bash
kubectl get nodes -o wide
```

All nodes should show `Ready` once the CNI (Canal/Flannel by default) finishes
initializing — this can take a minute or two after a node first joins.

## 7. Label the GPU worker node

```bash
kubectl label node <gpu-node-name> nodegroup=gpu
```

This is the label convention `app_serving`/`app_shaide`/`monitoring` expect for
`nodeSelector: {nodegroup: gpu}` in their stack configs.

The GPU Operator itself (which exposes `nvidia.com/gpu` as a schedulable resource) is
installed by the on-prem services stack, not here — see "Next steps" below.

## Uninstall

RKE2 installs uninstall scripts on every node:

```bash
# On a server node:
/usr/local/bin/rke2-uninstall.sh

# On an agent (worker) node:
/usr/local/bin/rke2-agent-uninstall.sh
```

## Next steps

This guide only covers the RKE2 cluster itself. Two things it deliberately leaves
out, owned by later stages of the platform instead:

- **MetalLB** (LoadBalancer IP assignment on bare metal) — deployed by the on-prem
  services stack, not by this guide or by `infra/gateway-provider`.
- **Istio + the shared Gateway** (ingress routing, TLS) — set up by
  `infra/gateway-provider`, which assumes MetalLB is already in place.

With a running cluster and `kubectl` configured, continue with the platform
deployment order: on-prem services stack (Harbor, MetalLB, GPU Operator) →
`infra/gateway-provider` → `app_serving` → `app_shaide`. See the root
[`README.md`](../../README.md) for the full architecture.
