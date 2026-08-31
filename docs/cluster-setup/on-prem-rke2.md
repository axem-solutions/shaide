---
title: "On-prem RKE2"
description: "Creating an air-gapped RKE2 cluster ready to run shaide."
weight: 50
---

# On-prem RKE2

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

## 8. Load balancing — MetalLB

Bare-metal Kubernetes has no `LoadBalancer` implementation, so a `Service` of that type
stays `<pending>` forever. MetalLB provides one.

```bash
helm repo add metallb https://metallb.github.io/metallb && helm repo update
helm install metallb metallb/metallb --namespace metallb-system --create-namespace
```

Allocate an address range from your LAN that is **not** part of any DHCP scope, and
advertise it over L2:

```bash
kubectl apply -f - <<'EOF'
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: default-pool
  namespace: metallb-system
spec:
  addresses:
    - 10.0.10.200-10.0.10.220
---
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: default
  namespace: metallb-system
spec:
  ipAddressPools:
    - default-pool
EOF
```

Point your gateway hostname at an address from that range.

## 9. Storage

RKE2 ships no dynamic provisioner. Install one and mark it default — Local Path
Provisioner is the usual choice for single-node or node-pinned storage:

```bash
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml

kubectl patch storageclass local-path \
  -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

Model weights land on whichever node the volume is created on, so ensure that node has
sufficient disk — see [Storage](../cluster-requirements/storage.md).

## 10. GPU Operator

Install the NVIDIA GPU Operator so GPUs are advertised as `nvidia.com/gpu`. Drivers must
already be present on the nodes; disable driver installation so the operator uses them:

```bash
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia && helm repo update
helm install gpu-operator nvidia/gpu-operator \
  --namespace gpu-operator --create-namespace \
  --set driver.enabled=false
```

Verify:

```bash
kubectl describe node <gpu-node> | grep -A2 nvidia.com/gpu
```

## 11. Verify the cluster is conformant

Run the checks in [Verification](../cluster-requirements/verification.md) — in particular
the LoadBalancer test, which must return an `EXTERNAL-IP` from the MetalLB pool.

## Air-gapped notes

Each component above pulls images from the internet. On a disconnected cluster, mirror the
MetalLB, Local Path Provisioner and GPU Operator images into your registry first, and pass
the chart archives from local paths rather than the Helm repositories. See
[Air-gapped installation](../installation/air-gapped.md).

## Next steps

This guide covers the RKE2 cluster and the supporting components shaide depends on. The
in-cluster gateway (Istio, Gateway API CRDs, the shared Gateway resource) is deployed by
the platform itself.

With a conformant cluster and `kubectl` configured, continue with the
[installer](../installation/installer-guide.md).
