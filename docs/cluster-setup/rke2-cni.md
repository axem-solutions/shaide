---
title: "RKE2 CNI selection"
description: "Choosing and configuring the CNI for an RKE2 cluster."
weight: 60
---

# RKE2 CNI selection

This document outlines the Container Network Interface (CNI) plugins supported by RKE2,
compares their performance, operational complexity, and suitability for AI workloads, and
shows how to configure and verify the chosen CNI on an [on-prem RKE2](on-prem-rke2.md)
cluster.

## Overview

RKE2 selects the CNI with the `cni` key in `/etc/rancher/rke2/config.yaml` on the server
nodes. It must be set before the first server starts.

| Feature | **Canal** (Default) | **Calico** | **Cilium** | **Multus** |
| :--- | :--- | :--- | :--- | :--- |
| **Architecture** | Flannel + Calico | BGP or eBPF | Pure eBPF | Meta-plugin (Multi-homing) |
| **Performance** | Moderate (VXLAN) | High | **Superior** (eBPF datapath) | Depends on primary + secondary CNI (e.g., SR-IOV) |
| **Observability** | Basic (iptables) | Good | **Excellent** (Hubble) | N/A |
| **AI Suitability** | Low / General | Good | **High** | Optional, but essential for multi-NIC/SR-IOV workloads |
| **Ops Complexity** | Very Low | Moderate | High | High |

### Quick Reference

| CNI | PoC | Production | FIPS | `cni` value |
| :--- | :---: | :---: | :---: | :--- |
| **Canal** | Yes | Limited (general workloads only; avoid for distributed training) | **Yes (only option)** | `canal` (default, can be omitted) |
| **Calico** | Yes | Yes (10G/25G Ethernet, BGP fabric) | No | `calico` |
| **Cilium** | No (too complex) | **Yes (recommended for AI)** | No | `cilium` |
| **Multus** | No | Yes (mandatory for RDMA/RoCE) | No | `[multus, <primary>]`, e.g. `[multus, cilium]` |


## Supported CNI Plugins

### 1. Canal (Default)
- **Description**
  - Combines Flannel (VXLAN routing) with Calico (Network Policy).
- **Pros**
  - "Just works" out of the box.
  - Extremely low maintenance.
  - Bundled in the core RKE2 air-gap tarball.
- **Cons**
  - VXLAN encapsulation overhead (~50 bytes) consumes CPU and adds latency.
  - Limited observability into high-concurrency traffic.
  - Firewalld conflicts.
  - NetworkManager interface management requirements.
  - `iptables`/`xtables-nft` requirements to avoid hostPort-related failures and Canal IP exhaustion symptoms.
- **AI Context**
  - Suitable for management clusters or small-scale inference.
  - Not recommended for multi-node distributed training (e.g., DeepSpeed, Megatron) where network throughput is the primary bottleneck.

### 2. Calico
- **Description**
  - A high-performance networking solution supporting BGP (native routing) and eBPF data planes.
- **Pros**
  - Can run without encapsulation (Native Routing).
  - Eliminates VXLAN overhead when Top-of-Rack (ToR) switches support BGP peering.
- **Cons**
  - BGP configuration adds significant network engineering complexity.
  - When using eBPF dataplane on kernels older than 5.7, Calico must disable checksum offloading and may see TCP throughput capped around ~2.5Gbps. BGP native routing mode is not affected.
  - Relies on `iptables`/`xtables-nft`.
  - Has SELinux caveats on some distros.
  - Calico eBPF dataplane support in RKE2 is version-gated (January 2026 releases and newer).
- **AI Context**
  - A strong choice for 10G/25G Ethernet fabrics where eBPF performance is required but Cilium's complexity is not desired.

### 3. Cilium
- **Description**
  - The industry standard for high-performance networking using eBPF to bypass the Linux `iptables` stack.
- **Pros**
  - Massive throughput.
  - Built-in L7 filtering.
  - **Hubble** provides deep observability into pod-to-pod latency and packet drops.
- **Cons**
  - Requires kernel >= 5.8 for kube-proxy replacement (the absolute minimum of 4.9.17 allows only basic pod connectivity). Ubuntu 24.04 (kernel 6.8+) satisfies this.
  - Higher RAM consumption for the Cilium agent.
  - Requires the Cilium air-gap image tarball.
- **AI Context**
  - **Highly Recommended.**
  - The ability to visualize latency via Hubble is invaluable for debugging "slow-node" problems in large-scale GPU clusters.

> **Note:** GKE Dataplane V2 (Google Kubernetes Engine) is implemented using Cilium. The legacy GKE dataplane is implemented using Calico. Choosing Cilium on-prem aligns with the networking model used in modern managed Kubernetes offerings.

### 4. Multus
- **Description**
  - A CNI "meta-plugin" that allows a single Pod to attach to multiple network interfaces.
- **Pros**
  - Allows separating K8s control plane traffic (eth0) from high-speed data fabric traffic (net1).
- **Cons**
  - Cannot be deployed standalone.
  - Must be combined with a conventional primary CNI.
  - Must be listed first in RKE2 `cni:`.
  - Configuration involves `NetworkAttachmentDefinitions` (CRDs) and is harder to debug.
  - `host-local` IPAM is not recommended for multi-node clusters.
  - SR-IOV additionally requires compatible NIC hardware, IOMMU enablement, and supported drivers/operators.
- **AI Context**
  - **Mandatory for RDMA/RoCE.**
  - If nodes use InfiniBand or 100G/200G RoCE for **GPUDirect RDMA**, Multus is the only way to expose those high-speed interfaces directly to containers while maintaining standard K8s networking.


## Choosing a CNI

**Short answer: Canal for PoC, Cilium for everything intended to run in production.**

For a proof-of-concept or a short-lived demo environment the right choice is Canal. It is the RKE2 default, requires no additional artifacts, and gets a cluster running in minutes. There is no engineering overhead and no vendor relationship to manage. If the goal is to validate the platform concept quickly, Canal removes all unnecessary friction.

For a development or production environment (any cluster that is expected to grow, serve real workloads, or remain in operation beyond the initial evaluation), Cilium is the recommended CNI. Its eBPF datapath delivers the network throughput that GPU-intensive and distributed training workloads require, and Hubble provides the observability needed to diagnose performance problems at scale. These capabilities are not available in Canal and cannot be retrofitted without a full cluster rebuild.

One consideration worth raising with stakeholders: Cilium's enterprise feature set (advanced network policy, FQDN filtering, Tetragon for runtime security) is governed by an Isovalent/Cisco enterprise licence. The open-source distribution is fully functional for this platform's current requirements, but if enterprise support or those additional features are on the roadmap, a commercial agreement should be factored into the project plan before the platform goes into production.

**Recommendation summary:**
- **PoC / demo:** Canal - deploy in minutes, zero commitment.
- **Dev / production:** Cilium - invest once, gain throughput, observability, and a clear upgrade path.
- **FIPS-regulated:** Canal - no alternative within RKE2.

### Security Constraint (FIPS)
**FIPS environments must use Canal.** Only Canal is rebuilt for FIPS compliance in RKE2. Calico, Cilium, and Multus must not be selected in FIPS-regulated deployments.

### Recommended Configuration for AI Clusters
For clusters focused on large-scale model training:
1.  **Host OS:** Ubuntu 24.04 (Kernel 6.8+) to leverage modern eBPF features.
2.  **CNI:** Cilium (in eBPF mode).
3.  **Secondary Network (Optional):** Multus for RDMA/InfiniBand support if high-speed fabrics are available.
4.  **Capacity Planning:** The default pod CIDR (`10.42.0.0/16`) allocates a `/24` per node (256 pod IPs). For high-density GPU nodes running many containers, verify this is sufficient before deployment.


## Configuring the CNI

Changing the CNI on an existing cluster is destructive and requires a cluster reset. Choose
it during initial setup, or plan a maintenance window.

### 1. Set the CNI before the first server starts

On every server node, before running `systemctl start rke2-server` (step 1 and step 4 of
[On-prem RKE2](on-prem-rke2.md)), add the `cni` key to `/etc/rancher/rke2/config.yaml`.
All server nodes must use the same value. Agent nodes inherit the CNI from the servers.

```yaml
# /etc/rancher/rke2/config.yaml
cni: cilium
```

For Multus, list it first, followed by the primary CNI:

```yaml
cni:
  - multus
  - cilium
```

### 2. Provide the CNI images (air-gapped only)

RKE2 imports every image tarball found in `/var/lib/rancher/rke2/agent/images/` on startup.
Download the tarballs matching your RKE2 version from the
[RKE2 releases page](https://github.com/rancher/rke2/releases) and copy them to that
directory on **every** node before starting the RKE2 service.

| CNI | Tarball (in addition to `rke2-images-core.linux-amd64.tar.zst`) |
| :--- | :--- |
| Canal | `rke2-images-canal.linux-amd64.tar.zst` (already included in the all-in-one `rke2-images.linux-amd64.tar.zst`) |
| Calico | `rke2-images-calico.linux-amd64.tar.zst` |
| Cilium | `rke2-images-cilium.linux-amd64.tar.zst` |
| Multus | `rke2-images-multus.linux-amd64.tar.zst`, plus the tarball of the primary CNI |

```bash
mkdir -p /var/lib/rancher/rke2/agent/images/
cp rke2-images-core.linux-amd64.tar.zst rke2-images-cilium.linux-amd64.tar.zst \
  /var/lib/rancher/rke2/agent/images/
```


## Verifying the CNI

Run these checks after all nodes have joined. They confirm that the intended CNI is
running, that pods get addresses, and that traffic flows between nodes.

### 1. The CNI pods are running on every node

```bash
# Canal
kubectl -n kube-system get pods -l k8s-app=canal -o wide
# Calico
kubectl -n calico-system get pods -l k8s-app=calico-node -o wide
# Cilium
kubectl -n kube-system get pods -l k8s-app=cilium -o wide
# Multus (in addition to the primary CNI)
kubectl -n kube-system get pods -l app=rke2-multus -o wide
```

Expected: one `Running` pod per node, and `kubectl get nodes` reports every node as `Ready`.

For Cilium, also check the agent's own health report:

```bash
kubectl -n kube-system exec ds/cilium -- cilium-dbg status --brief
```

Expected: `OK`.

### 2. The node uses the intended CNI config

On any node:

```bash
ls /etc/cni/net.d/
```

Expected: the first file (in lexical order) belongs to the chosen CNI, for example
`05-cilium.conflist` for Cilium or `00-multus.conf` for Multus.

### 3. Pods on different nodes can reach each other

Start two test pods pinned to two different nodes:

```bash
kubectl run cni-test-a --image=busybox:1.36 --restart=Never \
  --overrides='{"spec":{"nodeName":"<node-a>"}}' -- sleep 3600
kubectl run cni-test-b --image=busybox:1.36 --restart=Never \
  --overrides='{"spec":{"nodeName":"<node-b>"}}' -- sleep 3600
kubectl wait --for=condition=Ready pod/cni-test-a pod/cni-test-b --timeout=120s

kubectl get pods cni-test-a cni-test-b -o wide
kubectl exec cni-test-a -- ping -c 3 <cni-test-b-pod-ip>
```

Expected: both pods have an IP from the pod CIDR (`10.42.0.0/16` by default) and the ping
succeeds with no packet loss.

### 4. Cluster DNS resolves Services

```bash
kubectl exec cni-test-a -- nslookup kubernetes.default.svc.cluster.local
```

Expected: the lookup returns the ClusterIP of the `kubernetes` Service.

### 5. Multus only: the NetworkAttachmentDefinition API exists

```bash
kubectl get crd network-attachment-definitions.k8s.cni.cncf.io
```

Expected: the CRD is listed.

Clean up the test pods:

```bash
kubectl delete pod cni-test-a cni-test-b
```


## Troubleshooting

### CNI IP Exhaustion

IP exhaustion is CNI-specific. Each CNI manages IP allocation differently. Use the procedure that matches the active CNI.

**Canal (host-local IPAM)**

Canal delegates IP allocation to Flannel, which uses the `host-local` IPAM plugin and writes per-pod lease files to `/var/lib/cni/networks/k8s-pod-network/` on each node. After hard resets or crashes, these files can become stale, causing pods to fail with `no IP addresses available in range set` even when IPs are actually free.

To recover, on each affected node, one node at a time:

```bash
# Use rke2-agent on worker nodes
systemctl stop rke2-server
find /var/lib/cni/networks/k8s-pod-network/ -type f ! -name 'last_reserved_ip*' -delete
systemctl start rke2-server
```

Then confirm that new pods on that node get an IP (step 3 of [Verifying the CNI](#verifying-the-cni)).

**Calico (Calico IPAM)**

Calico uses its own IPAM, storing allocations as `IPAMBlock` and `IPAMHandle` custom resources in the Kubernetes API, not as files on disk.

Diagnose and release stuck allocations with:
```bash
calicoctl ipam show --show-blocks
calicoctl ipam check
calicoctl ipam release --ip=<ip>
```

**Cilium (Cilium IPAM)**

Cilium manages IP allocation through `CiliumNode` resources in the Kubernetes API.

Diagnose with:
```bash
kubectl get ciliumnode <node> -o yaml
```

Restarting the Cilium agent typically resolves transient allocation failures:
```bash
kubectl rollout restart daemonset/cilium -n kube-system
```


## Sources

- RKE2 Network Options: https://docs.rke2.io/networking/basic_network_options
- RKE2 Air-Gap Install: https://docs.rke2.io/install/airgap
- RKE2 Known Issues: https://docs.rke2.io/known_issues
- RKE2 Multus and SR-IOV: https://docs.rke2.io/networking/multus_sriov
- RKE2 FIPS Support: https://docs.rke2.io/security/fips_support.html
- GKE Dataplane V2: https://docs.cloud.google.com/kubernetes-engine/docs/concepts/dataplane-v2
