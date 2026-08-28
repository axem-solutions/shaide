---
title: "Deployment targets"
description: "Where shaide can run."
weight: 40
---

# Deployment targets

shaide runs on any conformant Kubernetes cluster meeting the
[cluster requirements](../cluster-requirements/overview.md).

| Target | Managed | Air-gap | Guide |
| --- | --- | --- | --- |
| AWS EKS | Yes | No | [AWS EKS](../cluster-setup/aws-eks.md) |
| GCP GKE | Yes | No | [GCP GKE](../cluster-setup/gcp-gke.md) |
| Azure AKS | Yes | No | [Azure AKS](../cluster-setup/azure-aks.md) |
| On-prem RKE2 | No | Yes | [On-prem RKE2](../cluster-setup/on-prem-rke2.md) |

## Choosing

**Managed cloud** - fastest to stand up. Storage, network load balancing and GPU drivers are
largely handled by the provider. Requires egress unless you configure otherwise.

**On-prem RKE2** - full sovereignty and the only fully air-gapped option. You own
storage, load balancing (MetalLB) and GPU drivers. 

The platform and installer are identical across targets - only the cluster underneath
differs.
