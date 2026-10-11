---
title: "OpenAI-compatible API"
description: "How to connect to a running shaide deployment using OpenAI-compatible clients."
weight: 10
---

# OpenAI-compatible API

shaide exposes an OpenAI-compatible HTTP API. Any client, SDK, agent framework or tool
that can talk to the OpenAI API can talk to shaide - the only thing that changes is the
base URL and the API key.

## Base URL

```
https://<your-shaide-endpoint>/v1
```

`<your-shaide-endpoint>` is the address the platform was exposed on during installation.
Recover it from the cluster if you no longer have it:

```bash
kubectl -n shaide get svc shaide-server \
  -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

## Authentication

Requests are authenticated with a bearer token, supplied the same way as an OpenAI API
key:

```
Authorization: Bearer <your-api-key>
```

Keys are issued and managed by the shaide server. See
[Authentication](authentication.md).

## Checking connectivity

The cheapest call is the model list. It requires no request body and confirms routing,
TLS and authentication in one shot:

```bash
curl https://<your-shaide-endpoint>/v1/models \
  -H "Authorization: Bearer <your-api-key>"
```

A healthy platform responds with the models currently served:

```json
{
  "object": "list",
  "data": [
    { "id": "gpt-oss-20b",         "object": "model", "owned_by": "shaide" },
    { "id": "nomic-embed-text-v1.5", "object": "model", "owned_by": "shaide" }
  ]
}
```

The `id` values are what you pass as `model` in subsequent requests. They are set when a
model is deployed and are not necessarily the upstream Hugging Face names.

## Supported endpoints

| Endpoint | Purpose | Page |
| --- | --- | --- |
| `GET /v1/models` | List served models | this page |
| `POST /v1/chat/completions` | Chat and text generation, streaming supported | [Chat completions](chat-completions.md) |
| `POST /v1/embeddings` | Vector embeddings | [Embeddings](embeddings.md) |

## Compatibility scope

shaide implements the subset of the OpenAI API that maps onto self-hosted inference.
Anything that depends on OpenAI-hosted services does not apply.

**Supported:** chat completions (streaming and non-streaming), embeddings, model listing,
and the common sampling parameters - `temperature`, `top_p`, `max_tokens`, `stop`,
`presence_penalty`, `frequency_penalty`, `seed`.

**Not supported:** Assistants, Threads, Files, fine-tuning, image generation,
audio/speech, moderation.

Tool and function calling depends on the served model rather than on shaide. Models
trained for tool use expose it through the standard `tools` parameter; models that were
not will ignore it.

## Choosing a client

Point any of these at your endpoint:

```python
# Python
from openai import OpenAI
client = OpenAI(base_url="https://<endpoint>/v1", api_key="<key>")
```

```javascript
// TypeScript / JavaScript
import OpenAI from "openai";
const client = new OpenAI({ baseURL: "https://<endpoint>/v1", apiKey: "<key>" });
```

```bash
# Environment variables — respected by most OpenAI-compatible tools
export OPENAI_BASE_URL="https://<endpoint>/v1"
export OPENAI_API_KEY="<key>"
```

For framework-specific wiring, see [Agent integrations](agent-integrations.md).

## Errors

shaide uses standard HTTP status codes and the OpenAI error envelope.

| Status | Meaning | Usual cause |
| --- | --- | --- |
| `401` | Unauthorized | Missing or invalid API key |
| `404` | Not found | `model` value does not match a served model - check `/v1/models` |
| `429` | Too many requests | All replicas saturated; retry with backoff |
| `503` | Service unavailable | Model still starting, or no healthy replica |

A `503` immediately after installation usually means model weights are still loading.
Large models can take several minutes to become ready on first start:

```bash
kubectl -n shaide-serving get pods -w
```
