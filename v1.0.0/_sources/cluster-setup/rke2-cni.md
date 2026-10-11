---
title: "RKE2 CNI selection"
description: "Choosing and configuring the CNI for an RKE2 cluster."
weight: 60
---

# RKE2 CNI selection

This document outlines the supported Container Network Interface (CNI) plugins for the RKE2-based AI Platform, comparing their performance, operational complexity, and suitability for AI workloads.

## Overview

The platform uses Ansible to automate RKE2 deployment. The CNI is controlled by the `rke2_cni` variable in `infra/on-prem/ansible/group_vars/all/main.yml`.

| Feature | **Canal** (Default) | **Calico** | **Cilium** | **Multus** |
| :--- | :--- | :--- | :--- | :--- |
| **Architecture** | Flannel + Calico | BGP or eBPF | Pure eBPF | Meta-plugin (Multi-homing) |
| **Performance** | Moderate (VXLAN) | High | **Superior** (eBPF datapath) | Depends on primary + secondary CNI (e.g., SR-IOV) |
| **Observability** | Basic (iptables) | Good | **Excellent** (Hubble) | N/A |
| **AI Suitability** | Low / General | Good | **High** | Optional, but essential for multi-NIC/SR-IOV workloads |
| **Ops Complexity** | Very Low | Moderate | High | High |

### Quick Reference

| CNI | PoC | Production | FIPS | Current Ansible Playbook |
| :--- | :---: | :---: | :---: | :--- |
| **Canal** | Yes | Limited (general workloads only; avoid for distributed training) | **Yes (only option)** | Full — default, no extra artifacts |
| **Calico** | Yes | Yes (10G/25G Ethernet, BGP fabric) | No | Full — set `rke2_cni: calico`, add artifact |
| **Cilium** | No (too complex) | **Yes (recommended for AI)** | No | Full — set `rke2_cni: cilium`, add artifact |
| **Multus** | No | Yes (mandatory for RDMA/RoCE) | No | Needs further development — current template supports only one CNI; pairing with a primary CNI requires a template change |


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
  - Requires `rke2-images.cilium...` air-gap artifact.
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

> **Automation Limitation:** The current Ansible template (`roles/rke2_install/templates/config.yaml.j2`) renders only a single CNI entry. RKE2 requires Multus to be paired with a primary CNI (e.g., `multus` + `canal`), but the `rke2_cni` variable is a single string. Selecting `multus` alone results in a non-functional cluster. Multus deployment requires a template change to support a secondary CNI variable before it can be fully automated.


## Operational Guide (Ansible)

### CNI Selection Recommendation

**Short answer: Canal for PoC, Cilium for everything intended to run in production.**

For a proof-of-concept or a short-lived demo environment the right choice is Canal. It is the RKE2 default, requires no additional artifacts, and gets a cluster running in minutes. There is no engineering overhead and no vendor relationship to manage. If the goal is to validate the platform concept quickly, Canal removes all unnecessary friction.

For a development or production environment — any cluster that is expected to grow, serve real workloads, or remain in operation beyond the initial evaluation — Cilium is the recommended CNI. Its eBPF datapath delivers the network throughput that GPU-intensive and distributed training workloads require, and Hubble provides the observability needed to diagnose performance problems at scale. These capabilities are not available in Canal and cannot be retrofitted without a full cluster rebuild.

One consideration worth raising with stakeholders: Cilium's enterprise feature set (advanced network policy, FQDN filtering, Tetragon for runtime security) is governed by an Isovalent/Cisco enterprise licence. The open-source distribution is fully functional for this platform's current requirements, but if enterprise support or those additional features are on the roadmap, a commercial agreement should be factored into the project plan before the platform goes into production.

**Recommendation summary:**
- **PoC / demo:** Canal — deploy in minutes, zero commitment.
- **Dev / production:** Cilium — invest once, gain throughput, observability, and a clear upgrade path.
- **FIPS-regulated:** Canal — no alternative within RKE2.

### Security Constraint (FIPS)
**FIPS environments must use Canal.** Only Canal is rebuilt for FIPS compliance in RKE2 — Calico, Cilium, and Multus must not be selected in FIPS-regulated deployments.

### Recommended Configuration for AI Clusters
For clusters focused on large-scale model training:
1.  **Host OS:** Ubuntu 24.04 (Kernel 6.8+) to leverage modern eBPF features.
2.  **CNI:** Cilium (in eBPF mode).
3.  **Secondary Network (Optional):** Multus for RDMA/InfiniBand support if high-speed fabrics are available. See the Multus automation limitation note above before planning deployment.
4.  **Capacity Planning:** The default pod CIDR (`10.42.0.0/16`) allocates a `/24` per node (256 pod IPs). For high-density GPU nodes running many containers, verify this is sufficient before deployment.

### Switching CNI (Air-Gapped)
To switch the CNI in an air-gapped environment:

1.  **Download Artifacts:** Ensure the required CNI image tarball is in `infra/on-prem/ansible/artifacts/rke2/`.
    *   Example: `rke2-images.cilium.linux-amd64.tar.zst`
    *   Canal requires no additional tarball — its images are bundled in `rke2-images.linux-amd64.tar.zst`.
2.  **Update Variables:** Modify `infra/on-prem/ansible/group_vars/all/main.yml`:
    ```yaml
    rke2_cni: "cilium"
    ```
3.  **Deploy:** Run the provision playbook.
    *   *Warning:* Changing CNI on an existing cluster is a destructive operation. It is recommended to perform this during initial setup or a planned maintenance window requiring a cluster reset.

### Troubleshooting

#### CNI IP Exhaustion

IP exhaustion is CNI-specific. Each CNI manages IP allocation differently. Use the procedure that matches the active `rke2_cni`.

**Canal (host-local IPAM)**

Canal delegates IP allocation to Flannel, which uses the `host-local` IPAM plugin and writes per-pod lease files to `/var/lib/cni/networks/k8s-pod-network/` on each node. After hard resets or crashes, these files can become stale, causing pods to fail with `no IP addresses available in range set` even when IPs are actually free.

Use the dedicated recovery playbook:

```bash
ansible-playbook -i inventory/hosts.yml rke2_canal_ipam_reset.yml
# Or target a single node:
ansible-playbook -i inventory/hosts.yml rke2_canal_ipam_reset.yml --limit <hostname>
```

This removes stale lease files (preserving `last_reserved_ip*`) and restarts the appropriate RKE2 service on each node serially. This playbook is **Canal-only** — it has no effect on Calico or Cilium deployments.

**Calico (Calico IPAM)**

Calico uses its own IPAM, storing allocations as `IPAMBlock` and `IPAMHandle` custom resources in the Kubernetes API — not as files on disk. The `rke2_canal_ipam_reset.yml` playbook does not apply.

Diagnose and release stuck allocations with:
```bash
calicoctl ipam show --show-blocks
calicoctl ipam check
calicoctl ipam release --ip=<ip>
```

**Cilium (Cilium IPAM)**

Cilium manages IP allocation through `CiliumNode` resources in the Kubernetes API. The `rke2_canal_ipam_reset.yml` playbook does not apply.

Diagnose with:
```bash
kubectl get ciliumnode <node> -o yaml
cilium ipam
```

Restarting the Cilium agent on the affected node typically resolves transient allocation failures:
```bash
kubectl rollout restart daemonset/cilium -n kube-system
```


## Sources

- RKE2 Network Options: https://docs.rke2.io/networking/basic_network_options
- RKE2 Known Issues: https://docs.rke2.io/known_issues
- RKE2 Multus and SR-IOV: https://docs.rke2.io/networking/multus_sriov
- RKE2 FIPS Support: https://docs.rke2.io/security/fips_support.html
- GKE Dataplane V2: https://docs.cloud.google.com/kubernetes-engine/docs/concepts/dataplane-v2
