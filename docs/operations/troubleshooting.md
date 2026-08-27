---
title: "Troubleshooting"
description: "Diagnosing common problems by symptom."
weight: 90
---

# Troubleshooting

Indexed by symptom.

## API

| Symptom | Likely cause | Check |
| --- | --- | --- |
| `401` | Invalid or missing key | [Authentication](../usage/authentication.md) |
| `404` on a model | ID does not match a served model | `GET /v1/models` |
| `429` | Replicas saturated | [Scaling](scaling.md) |
| `503` | Model still loading, or no healthy replica | `kubectl -n shaide-serving get pods` |
| Connection refused | No ingress address | [Networking](../cluster-requirements/networking.md) |

## Pods

**`Pending`**

```bash
kubectl describe pod <pod> | grep -A10 Events
```

- `Insufficient nvidia.com/gpu` - out of GPU capacity
- `pod has unbound immediate PersistentVolumeClaims` - StorageClass problem
- `node(s) didn't match node selector` - missing node labels

**`ImagePullBackOff`** - nodes cannot reach or do not trust the internal registry. See
[Node registry trust](node-registry-trust.md).

**`CrashLoopBackOff`**

```bash
kubectl logs <pod> --previous
```

For serving pods this is usually insufficient VRAM for the configured model.

## GPUs not detected

```bash
kubectl describe node <gpu-node> | grep nvidia.com/gpu
```

Zero or absent means the driver stack is not working. Drivers must be installed before
shaide - see [Compute](../cluster-requirements/compute.md).

## Installer

| Symptom | Cause |
| --- | --- |
| Cannot read previous state | Wrong passphrase or state directory |
| Bundle validation fails | Bundle incomplete - see [Installer bundle](../installation/bundle.md) |
| Requires a terminal | Missing `-it` on `docker run` |
| Times out reaching cluster | kubeconfig or network path |

## TLS

Certificate not issued: check cert-manager and any DNS validation records.

```bash
kubectl get certificate -A
kubectl describe certificate <name>
```

See [TLS certificates](tls-certificates.md).

## Logs

```bash
kubectl logs -n shaide deploy/shaide-server
kubectl logs -n shaide-serving <model-pod>
```

Grafana ships with the platform for aggregated logs - see
[Observability](../architecture/observability.md).
