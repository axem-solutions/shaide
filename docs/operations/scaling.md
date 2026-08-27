---
title: "Scaling"
description: "Adding capacity for more concurrent load."
weight: 20
---

# Scaling

## Replicas

Capacity for concurrent requests is a function of replicas per model. Increase the
replica count in the serving stack configuration and re-apply.

Each replica needs its own GPU allocation - VRAM requirements are per replica, not
per model. See [Compute](../cluster-requirements/compute.md).

## Nodes

When replicas cannot be scheduled, add GPU nodes:

```bash
kubectl get pods -n shaide-serving --field-selector status.phase=Pending
kubectl describe pod <pending-pod> | grep -A10 Events
```

`Insufficient nvidia.com/gpu` means the cluster is out of GPU capacity.

## Signals

| Signal | Meaning |
| --- | --- |
| `429` responses | Replicas saturated |
| Rising time-to-first-token | Queueing |
| Pods `Pending` | Out of GPU capacity |

Grafana dashboards ship with the platform - see
[Observability](../architecture/observability.md).

## Cost

Scale unused node pools to zero when idle. GPU nodes dominate cost, and models hold VRAM
for as long as they are scheduled.
