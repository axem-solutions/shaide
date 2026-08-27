---
title: "Verify installation"
description: "Confirming a successful install."
weight: 30
---

# Verify installation

## Pods

```bash
kubectl get pods -A | grep -E 'shaide|harbor|istio|serving'
```

All pods should be `Running` or `Completed`. Model serving pods are the slowest to become
ready - large weights take several minutes to load on first start.

## Endpoint

```bash
kubectl -n shaide get svc shaide-server
```

An `EXTERNAL-IP` should be assigned. 

## API

```bash
curl https://<endpoint>/v1/models -H "Authorization: Bearer <key>"
```

Expect a JSON list of served models.

## Generation

```bash
curl https://<endpoint>/v1/chat/completions \
  -H "Authorization: Bearer <key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"<model-id>","messages":[{"role":"user","content":"ping"}]}'
```

## GPU allocation

```bash
kubectl describe node <gpu-node> | grep -A5 "Allocated resources"
```

`nvidia.com/gpu` requests should be non-zero once models are scheduled.

## If something fails

| Symptom | Check |
| --- | --- |
| Pods `Pending` | Node capacity, GPU availability, PVC binding |
| Pods `ImagePullBackOff` | Registry trust - [Node registry trust](../operations/node-registry-trust.md) |
| `503` from the API | Model still loading; watch pod status |
| No `EXTERNAL-IP` | No LoadBalancer provider - [Networking](../cluster-requirements/networking.md) |

See [Troubleshooting](../operations/troubleshooting.md).
