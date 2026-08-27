---
title: "Ports"
description: "Network ports used by the platform."
weight: 20
---

# Ports

## External

| Port | Protocol | Purpose |
| --- | --- | --- |
| 443 | TCP | Platform ingress (HTTPS) |
| 80 | TCP | HTTP, typically redirected to 443 |

## Cluster access

Required from the provisioning machine:

| Port | Protocol | Target | Purpose |
| --- | --- | --- | --- |
| 6443 | TCP | control plane | Kubernetes API |
| 22 | TCP | all nodes | SSH - on-prem image preload only |
| 30000-32767 | TCP/UDP | all nodes | NodePort range |

## Internal services

| Component | Service | Port |
| --- | --- | --- |
| shaide server | `shaide-server` | 80 → 8080 |
| Control panel | `control-panel` | 3000 |
| Web app | `webapp` | 8787 |
| Object storage | `rustfs` | 9000 (9001 console) |
| Vector database | `qdrant` | 6333, 6334 |

Internal services are `ClusterIP` and not exposed externally.
