---
title: "Core concepts"
description: "Vocabulary used throughout the documentation."
weight: 20
---

# Core concepts

| Term | Meaning |
| --- | --- |
| **vLLM** | The inference engine that executes a model on GPUs |
| **llm-d** | Orchestration framework running multiple coordinated vLLM instances |
| **ModelService** | The deployable unit for one model - pods, config and service |
| **InferencePool** | A set of interchangeable replicas serving the same model |
| **GAIE / EPP** | Gateway API Inference Extension and its Endpoint Picker, which selects a replica per request |
| **Gateway** | The Istio + Gateway API ingress layer fronting model serving |
| **Internal registry** | The in-cluster OCI registry holding images and model weights |

| **Stack** | A Pulumi deployment unit with its own configuration |
| **Nodegroup** | A labelled set of nodes targeted by workload placement |
| **KV cache** | Per-request attention state; a primary driver of VRAM use |
| **Air-gapped** | A cluster with no internet access |

## How they fit together

A **model** is deployed as a **ModelService**, which creates pods running **vLLM** under
**llm-d**. Replicas of that model form an **InferencePool**. Requests arrive at the
**Gateway**, and **GAIE** picks a replica. Images and weights come from the **internal
registry**, populated by the installer from the origin registries and Hugging Face.

Embedding models bypass the Gateway and GAIE - the inference gateway understands
chat-completion traffic only.
