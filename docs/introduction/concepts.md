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
| **Model server** | The vLLM pods serving one model, defined by its `model.yaml` |
| **InferencePool** | A set of interchangeable replicas serving the same model |
| **llm-d router / EPP** | The llm-d Endpoint Picker, which selects a replica per request for the Gateway |
| **Gateway** | The Istio + Gateway API ingress layer fronting model serving |
| **Internal registry** | The in-cluster OCI registry holding images and model weights |

| **Stack** | A Pulumi deployment unit with its own configuration |
| **Nodegroup** | A labelled set of nodes targeted by workload placement |
| **KV cache** | Per-request attention state; a primary driver of VRAM use |
| **Air-gapped** | A cluster with no internet access |

## How they fit together

A **model** is deployed as a **model server**, pods running **vLLM** under **llm-d**.
Replicas of that model form an **InferencePool**. Requests arrive at the **Gateway**, and
the **llm-d router** picks a replica. Images and weights come from the **internal
registry**, populated by the installer from the origin registries and Hugging Face.

shaide server reaches embedding models through their own Service, without the Gateway.
