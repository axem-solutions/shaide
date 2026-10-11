---
title: "Networking"
description: "Ingress, port, DNS and egress requirements."
weight: 40
---

# Networking

## Ingress

**The cluster must be able to provision an external load balancer.** 

| Environment | Provided by |
| --- | --- |
| EKS | AWS Load Balancer Controller |
| GKE | Google Cloud load balancing |
| AKS | Application Gateway for Containers (AGC) |
| RKE2 / on-prem | **MetalLB**, or another `LoadBalancer` provider you install |

Managed clouds satisfy this natively. On bare Kubernetes there is no built-in
`LoadBalancer` implementation, so one must be installed - a `Service` of type
`LoadBalancer` will otherwise sit at `<pending>` forever.

Verify with the LoadBalancer check in [Verification](verification.md).

### How shaide uses it

**Shared gateway** - the shaide server Service is `ClusterIP` and attaches by `HTTPRoute` to the 
shared Gateway API `Gateway` that the platform installs.

See [Gateway and routing](../architecture/gateway.md).

## Ports

Required from the provisioning machine to the cluster:

| Port | Protocol | Target | Purpose |
| --- | --- | --- | --- |
| 6443 | TCP | control plane | Kubernetes API |
| 443 | TCP | all nodes | Platform ingress |
| 22 | TCP | all nodes | SSH - on-prem image preload only |
| 30000-32767 | TCP/UDP | all nodes | NodePort range |

## Pod networking

Pod-to-pod traffic must be unrestricted within shaide namespaces. If you enforce
default-deny `NetworkPolicy`, note that [MCP datasources](../architecture/mcp.md) manages
its own policies.

## DNS and TLS

HTTPS requires a DNS record pointing at the ingress address, plus a certificate. shaide
can issue certificates via cert-manager, or you can supply your own. See
[TLS certificates](../operations/tls-certificates.md).

## Egress

**Not required for cluster nodes.** They pull all images and model weights from the
internal registry.

The **provisioning machine** does need internet access - it fetches images from their
origin registries and model weights from Hugging Face, then pushes both into the internal
registry. See [Air-gapped installation](../installation/air-gapped.md).