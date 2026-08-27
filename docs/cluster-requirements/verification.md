---
title: "Verification"
description: "Commands to confirm a cluster meets the requirements."
weight: 50
---

# Verification

Run these against the target cluster before installing. All six must pass.

## 1. Version consistency

```bash
kubectl get nodes -o custom-columns="NAME:.metadata.name,VERSION:.status.nodeInfo.kubeletVersion"
```

Expect: every node on the same version, 1.30 or later.

## 2. Admin access

```bash
kubectl auth can-i '*' '*'
```

Expect: `yes`.

## 3. Node readiness

```bash
kubectl get nodes
kubectl get nodes --show-labels
```

Expect: all target nodes `Ready`, with workload placement labels applied.

## 4. GPU availability

```bash
kubectl describe node <gpu-node> | grep -A2 "nvidia.com/gpu"
```

Expect: non-zero `nvidia.com/gpu` under both `Capacity` and `Allocatable`. Zero or absent
means the driver stack is not working.

## 5. Default StorageClass

```bash
kubectl get storageclass
```

Expect: one class marked `(default)`.

## 6. LoadBalancer provisioning

```bash
kubectl create svc loadbalancer lb-test --tcp=80:80
kubectl get svc lb-test -w
kubectl delete svc lb-test
```

Expect: an `EXTERNAL-IP` is assigned. A persistent `<pending>` means the cluster has no
LoadBalancer provider - install MetalLB, or another `LoadBalancer` implementation, before
installing shaide.

## Next

Continue to [Prerequisites](../getting-started/prerequisites.md) for what the installer
itself requires.
