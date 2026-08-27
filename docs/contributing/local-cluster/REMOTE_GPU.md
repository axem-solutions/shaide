---
title: "Local cluster: Remote gpu"
description: "Local development cluster - Remote gpu."
weight: 70
---

# Local cluster: Remote gpu

## What kind of "remote node" is this?

It must be a **standalone VM (or bare-metal box)** — not a node that's already a
worker in another Kubernetes cluster (e.g. one of the GPU node pools in
`infra/azure`'s AKS cluster).

Why: vCluster's Private Nodes join mechanism works by running a join script
on the target machine that installs/configures `kubeadm`, `containerd`, and
`kubelet` and points them at *this* vind cluster's control plane
(`kubeadm join`-style). A kubelet can only register with one control plane
at a time — a machine already running as an AKS node's kubelet is already
registered with AKS's API server. You cannot dual-join it to your local
vind cluster without first draining/removing it from AKS, and even then
it's not a supported or sensible pattern (that VM is presumably sized and
managed for the AKS node pool's own lifecycle, autoscaling, and upgrades).

**What "we can serve GPU nodes in Azure" gets you here** is Azure quota/
capacity and VM SKUs — not a node you can borrow from the existing AKS
cluster. The manual below provisions a **new, separate Azure VM** dedicated
to this purpose.

## Prerequisites

- Docker installed and running (for the vind control-plane + worker
  containers)
- [vCluster CLI](https://www.vcluster.com/docs/getting-started/setup) v0.31.0+
- Azure CLI (`az`) logged in, with quota for a GPU VM SKU in your target
  region (GPU SKUs are frequently quota-restricted — check before you start)
- vCluster Platform running locally (coordinates the VPN) — this is a
  separate local service from the vind cluster itself

## Important limitation — plan before you start

Private Nodes / VPN **must be enabled at initial cluster creation** —
there is no upgrade path to add it to an already-running vcluster:

> "Private Nodes must be enabled during the initial creation of the
> virtual cluster. You cannot upgrade an existing virtual cluster to use
> Private Nodes."

If you already have a `local-k8s` cluster running from `cluster.yaml`
(Docker-only nodes, no VPN), it must be deleted and recreated to add this
capability — you can't bolt it on. The steps below use a fresh cluster
name (`local-k8s-gpu`) so your existing `local-k8s` cluster is untouched;
rename to `local-k8s` and delete the old one first if you want to keep a
single cluster.

vCluster's docs show `experimental.docker.nodes` (the Docker-container
workers this repo's `cluster.yaml` uses) and `privateNodes.vpn` referenced
together in one config example, so combining them — Docker-container
workers for general use + a VPN-joined GPU box for GPU workloads — appears
supported. It isn't fully spelled out end-to-end in the docs, so treat
Part 1 below as something to validate on a throwaway cluster name first.

---

## Part 1 — Create the local docker-based cluster (with Private Nodes enabled)

### 1.1 Select the Docker driver

```bash
vcluster use driver docker
```

### 1.2 Start vCluster Platform (VPN coordinator)

```bash
vcluster platform start
```

### 1.3 Create the cluster

Reuses this directory's `cluster.yaml` for the Docker-container nodes
(`worker-1`/`worker-2`/`worker-3`), with Private Nodes VPN layered on top:

```bash
vcluster create local-k8s-gpu --values cluster.yaml \
  --set privateNodes.vpn.enabled=true \
  --set privateNodes.vpn.nodeToNode.enabled=true
```

### 1.4 Verify

```bash
kubectl get nodes
# expect: control-plane node + worker-1/2/3, all Ready — same as a normal
# vind cluster; the GPU node isn't joined yet.
```

---

## Part 2 — Provision the remote GPU VM in Azure

```bash
az group create --name rg-vind-gpu --location <region>

az vm create \
  --resource-group rg-vind-gpu \
  --name vind-gpu-worker \
  --image Ubuntu2404 \
  --size Standard_NC4as_T4_v3 \
  --admin-username azureuser \
  --generate-ssh-keys \
  --public-ip-sku Standard
```

- `Standard_NC4as_T4_v3` (1× NVIDIA T4) is a relatively low-cost GPU SKU —
  swap for whatever series/quota you actually have available.
- OS must be one of vCluster's supported Private Node OSes: Ubuntu 24.04/
  22.04, RHEL 8/9/10, CentOS Stream 9, or Rocky 9/10.
- The VM needs a public IP only long enough to SSH in and run the join
  script once — after joining, it talks to the control plane over the VPN
  tunnel, not the public IP. Lock down / remove the public IP afterward if
  you want it off the public internet.

## Part 3 — Join the GPU VM as a Private Node

### 3.1 Generate a join token (on your laptop)

```bash
vcluster token create
```

This prints a join command/script URL.

### 3.2 Run the join script on the VM

```bash
ssh azureuser@<vm-public-ip>

curl -L -o join-script.sh "<url from vcluster token create>"
chmod +x join-script.sh
sudo ./join-script.sh
```

### 3.3 Verify the GPU node joined

Back on your laptop:

```bash
kubectl get nodes -o wide
# expect: control-plane, worker-1/2/3, and vind-gpu-worker, all Ready
```

---

## Part 4 — Make the GPU actually schedulable

Joining the node makes it a normal kubelet-managed node — it does **not**
by itself install NVIDIA drivers or expose `nvidia.com/gpu` as an
allocatable resource. You still need the NVIDIA driver + container
toolkit + device plugin on that node. vCluster's docs confirm the full
NVIDIA GPU Operator ecosystem works unmodified inside a tenant vcluster,
but don't spell out the exact install sequence for a Private Node
specifically — validate this step yourself:

- **Option A**: deploy the NVIDIA GPU Operator into `local-k8s-gpu`
  (targeting `vind-gpu-worker` via nodeSelector) and let it install the
  driver/toolkit/device-plugin on the node.
- **Option B**: pre-install the NVIDIA driver + container toolkit on the
  Azure VM via cloud-init/Ansible before running the join script, then
  only deploy the device plugin via GPU Operator afterward.

Confirm success with:

```bash
kubectl describe node vind-gpu-worker | grep -A5 Allocatable
# look for nvidia.com/gpu in the allocatable resources
```

---

## Part 5 — Cleanup

```bash
# remove the node from the vcluster first
kubectl delete node vind-gpu-worker

# then delete the Azure VM + resource group
az group delete --name rg-vind-gpu --yes --no-wait

# and the vcluster itself
vcluster delete local-k8s-gpu
```

## Cost note

Unlike the Docker-container workers in `cluster.yaml`, the GPU VM is a
real billed Azure resource for as long as it exists — deallocate
(`az vm deallocate`) or delete it when not actively testing, not just
`vcluster pause`, which only pauses the local Docker containers.
