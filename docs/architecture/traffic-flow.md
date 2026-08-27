---
title: "Traffic flow"
description: "How a request reaches a model replica."
weight: 15
---

# Traffic flow

## Generative

```
Public ingress (cloud LB, Gateway, or MetalLB on-prem)
  → shaide server
  → per-model Istio Gateway
  → GAIE endpoint picker (InferencePool)
  → ModelService decode pod
```

The endpoint picker selects a replica per request, accounting for KV-cache locality and
load rather than round-robin.

## Embedding

Embedding traffic bypasses the Gateway and GAIE - the inference gateway understands
chat-completion requests only:

```
shaide server → ms-<slug>-embeddings ClusterIP → ModelService decode pod
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
