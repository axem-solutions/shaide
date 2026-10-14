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
| A model is missing from the selector | The log at the start of `Select models` names each skipped model directory and why |
| Requires a terminal | Missing `-it` on `docker run` |
| Times out reaching cluster | kubeconfig or network path |

## Gateway

Requests do not reach a model: check that the shared Gateway is programmed and its
routes are attached.

```bash
kubectl -n gateway-system get gateway shared-gateway
kubectl get httproute -A
```

With the `istio` GatewayClass, the Gateway runs as the `shared-gateway-istio` Envoy
deployment. Its admin interface on port `15000` is bound to localhost, so query it
from inside the pod:

```bash
kubectl exec -n gateway-system deploy/shared-gateway-istio -c istio-proxy -- \
  curl -s localhost:15000/<endpoint>
```

| Endpoint | Shows |
| --- | --- |
| `/config_dump` | Full xDS config: listeners, routes, clusters, endpoints (pipe to `jq`) |
| `/listeners` | Active listeners |
| `/clusters` | Upstream health and connection counts |
| `/stats` | Counters and gauges |
| `/ready` | Proxy readiness |

Other ports on the pod: `15021` readiness probe (also the cloud load balancer health
check), `15090` Prometheus metrics, `80` ingress traffic.

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
