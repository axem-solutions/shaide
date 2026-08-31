---
title: "What is shaide?"
description: "Overview and value proposition."
weight: 10
---

# What is shaide?

shaide is a self-hosted AI platform that serves models at scale on your own Kubernetes
clusters. It runs many models side by side, each with multiple replicas, and routes
traffic across them.

## The problems it solves

**Assembly.** Building enterprise AI infrastructure means selecting and integrating an
inference engine, a serving orchestrator, a gateway, a model registry, storage and
observability - then repeating it per environment. shaide ships that stack as one
installable platform.

**Sovereignty.** shaide runs entirely inside your perimeter. Nothing phones home, and
there is no dependency on a vendor cloud - up to fully air-gapped clusters with no
internet access.

## What you get

| Capability | Detail |
| --- | --- |
| Distributed inference | vLLM engine, llm-d multi-instance orchestration |
| Multi-model serving | Generative and embedding models concurrently |
| OpenAI-compatible API | Existing SDKs and agents work unchanged |
| One-command install | Self-contained interactive installer |
| Air-gap support | Internal registry mirrors all images and weights |
| Infrastructure as code | Every layer a Pulumi program |

## What it is not

- Not a hosted service - you run it.
- Not a cluster provisioner - it installs onto an existing cluster.
- Not a model trainer - it serves models, it does not fine-tune them.
- Not an inference engine - it orchestrates vLLM, but does not implement the model runtime itself.

## Next

[Core concepts](concepts.md) · [Architecture](../architecture/overview.md)
