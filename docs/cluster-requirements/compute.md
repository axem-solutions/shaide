---
title: "Compute"
description: "Node, GPU and driver requirements."
weight: 20
---

# Compute

## Nodes

| Role | Minimum | Purpose |
| --- | --- | --- |
| CPU | 1 node - 4 vCPU, 16 GB RAM, 100 GB disk | Registry, gateway, application layer |
| GPU | 1 node with NVIDIA GPUs | Model inference |

All target nodes must report `Ready`. Nodes are selected by label, so workload placement
labels must be applied before installation.

## GPU

GPUs must be advertised to Kubernetes as the `nvidia.com/gpu` extended resource, via
either:

- the **NVIDIA GPU Operator** - typical on-prem, EKS, AKS
- the **NVIDIA device plugin** - typical on GKE, often provider-installed

Both `Capacity` and `Allocatable` must show a non-zero `nvidia.com/gpu` count.

## Drivers

**NVIDIA drivers must be installed on every GPU node before installation.** shaide does
not install drivers - they are tied to your node image and kernel.

Reference: [NVIDIA GPU Operator platform support](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/platform-support.html).

## VRAM sizing

VRAM per replica must cover the model weights plus its KV cache.

| Model size | Precision | VRAM before KV cache |
| --- | --- | --- |
| ~20B | FP8 | ~24 GB |
| ~30B | FP8 | ~35 GB |
| ~30B | BF16 | ~70 GB |

See [Model catalog](../usage/model-catalog.md) for validated models.

## Per-target notes

| Target | Note |
| --- | --- |
| EKS | GPU AMIs require the device plugin |
| GKE | GPU node pools require driver installation enabled |
| AKS | GPU VM sizes are quota-limited per region |
| RKE2 | Drivers and the GPU Operator are your responsibility |
