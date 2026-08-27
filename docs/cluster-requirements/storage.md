---
title: "Storage"
description: "StorageClass and capacity requirements."
weight: 30
---

# Storage

## StorageClass

A **default StorageClass** supporting `ReadWriteOnce` must exist. Dynamic provisioning is
required - shaide creates PVCs during installation.

| Target | Typical class |
| --- | --- |
| EKS | `gp3` (EBS CSI driver) |
| GKE | `standard-rwo` (PD CSI driver) |
| AKS | `managed-csi` (Azure Disk CSI) |
| RKE2 | `hostpath` |

## Capacity

**200 GB minimum, but dependent on model size.**  
Model weights dominate - a single 30B model can exceed 60 GB on disk.

| Consumer | Typical |
| --- | --- |
| Internal registry (images + model weights) | 100 GB+ |
| Application state | 10 GB |
| Observability data | 50 GB, retention-dependent |

Budget additional capacity per model you intend to serve.

## Provisioning machine

The host running the installer needs **100 GB** of free local disk for the bundle and
extracted artifacts, in a directory preserved across installer runs and upgrades.
