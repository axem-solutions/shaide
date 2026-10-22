---
title: "Traffic flow"
description: "How a request reaches a model replica."
weight: 15
---

# Traffic flow

## Generative

```
Public ingress (cloud LB or MetalLB on-prem)
  → shaide server
  → per-model Istio Gateway
  → llm-d router endpoint picker (InferencePool)
  → model server pod
```

The endpoint picker selects a replica per request, accounting for prefix-cache affinity and
token load rather than round-robin. If it is unavailable the Gateway still forwards the
request to a pod (`FailOpen`).

## Embedding

shaide server reaches embedders through a dedicated Service. The llm-d v1.0 router
also accepts `/v1/embeddings`, so the Gateway path works for embedders as well:

```
shaide server → ms-<slug>-embeddings ClusterIP → model server pod
```

Transparent to clients: both paths are reached through the same OpenAI-compatible
endpoint.

## Ingress by target

| Target | Ingress |
| --- | --- |
| EKS | AWS Load Balancer Controller |
| GKE | Gateway API with managed certificates |
| AKS | Application Gateway for Containers (AGC) |
| RKE2 | MetalLB |

## TLS

Terminated at ingress. Certificates are issued by cert-manager or supplied by you. See
[TLS certificates](../operations/tls-certificates.md).
