---
title: "Cluster requirements"
description: "What a target cluster must provide to run shaide."
weight: 10
---

# Cluster requirements

shaide installs onto an existing Kubernetes cluster. Any cluster meeting the requirements
below can run it, regardless of distribution or provider.

This chapter lists requirements only. For building a cluster, see
[Cluster setup](../cluster-setup/overview.md).

For the provsioning machine that runs the installer, see 
[Provisioning prerequisites](../getting-started/provisioning-prerequisites.md).

## Summary

| Area | Requirement |
| --- | --- |
| Kubernetes | 1.30+, identical version across all nodes |
| Access | kubeconfig with `cluster-admin` |
| CPU nodes | 1+, 4 vCPU / 16 GB RAM / 100 GB disk |
| GPU nodes | 1+, NVIDIA GPUs exposed as `nvidia.com/gpu` |
| GPU drivers | Pre-installed - shaide does not install them |
| Storage | Default StorageClass, `ReadWriteOnce`, 200 GB+ |
| Ingress | An external load balancer - cloud-native, or MetalLB on-prem |
| Egress | Not required for cluster nodes - the provisioning machine needs it |

## Details

| Page | Covers |
| --- | --- |
| [Compute](compute.md) | Nodes, GPUs, drivers, sizing |
| [Storage](storage.md) | StorageClasses and capacity |
| [Networking](networking.md) | Ingress, ports, DNS, TLS, egress |
| [Verification](verification.md) | Commands to validate a cluster |

## Access

The installer creates CRDs, namespaces and RBAC bindings, so it requires full
administrative rights:

```bash
kubectl auth can-i '*' '*'   # must return: yes
```

## Kubernetes version

**1.30 or later**, consistent across nodes. Mixed-version clusters are not supported
during installation.
