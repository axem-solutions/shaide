---
title: "Model catalog"
description: "Models validated on shaide."
weight: 50
---

# Model catalog

Models validated against the platform. Others generally work if vLLM supports them.

## Generative

| Model | Notes |
| --- | --- |
| GPT-OSS-20B | General purpose |
| Gemma-4-31B-it | Instruction-tuned |
| Qwen3.5-27B-FP8 | FP8 quantized |
| Qwen3-Coder-30B-A3B-Instruct | Code |
| DeepSeek-Coder-V2-Lite-Instruct | Code, lightweight |
| Devstral-Small-2-24B-Instruct-2512 | Code |
| GLM-4.7-Flash | Low latency |

## Embedding

| Model | Notes |
| --- | --- |
| BGE-M3 | Multilingual |
| EmbeddingGemma-300m | Lightweight |
| jina-embeddings-v5-text-small-retrieval | Retrieval-optimized |
| nomic-embed-text-v1.5 | General purpose |

## Serving

Model IDs exposed through the API are assigned at deployment and need not match upstream
Hugging Face names. List what your deployment actually serves:

```bash
curl https://<endpoint>/v1/models -H "Authorization: Bearer <key>"
```

VRAM requirements are in [Compute](../cluster-requirements/compute.md). Adding models is
covered in [Model management](../operations/model-management.md).
