---
title: "Chat completions"
description: "Generating text with the chat completions endpoint."
weight: 20
---

# Chat completions

`POST /v1/chat/completions` - the primary generation endpoint.

## Request

```bash
curl https://<endpoint>/v1/chat/completions \
  -H "Authorization: Bearer <key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<model-id>",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user",   "content": "Explain Kubernetes in one sentence."}
    ]
  }'
```

`<model-id>` must match an entry from `GET /v1/models`.

## Python

```python
from openai import OpenAI

client = OpenAI(base_url="https://<endpoint>/v1", api_key="<key>")

response = client.chat.completions.create(
    model="<model-id>",
    messages=[{"role": "user", "content": "Explain Kubernetes in one sentence."}],
)
print(response.choices[0].message.content)
```

## Streaming

Set `stream=True` to receive server-sent events as tokens are produced:

```python
stream = client.chat.completions.create(
    model="<model-id>",
    messages=[{"role": "user", "content": "Write a haiku about GPUs."}],
    stream=True,
)
for chunk in stream:
    print(chunk.choices[0].delta.content or "", end="")
```

Streaming is recommended for interactive use - it reports first-token latency rather than
waiting for the full generation.

## Parameters

| Parameter | Purpose |
| --- | --- |
| `temperature` | Randomness. `0` is near-deterministic |
| `top_p` | Nucleus sampling threshold |
| `max_tokens` | Cap on generated tokens |
| `stop` | Sequences that end generation |
| `seed` | Reproducibility hint |
| `presence_penalty` / `frequency_penalty` | Repetition control |

## Tool calling

Tool calling depends on the served model, not on shaide. Models trained for tool use
accept the standard `tools` parameter; others ignore it.

```python
response = client.chat.completions.create(
    model="<model-id>",
    messages=[{"role": "user", "content": "What is the weather in Budapest?"}],
    tools=[{
        "type": "function",
        "function": {
            "name": "get_weather",
            "parameters": {
                "type": "object",
                "properties": {"city": {"type": "string"}},
                "required": ["city"],
            },
        },
    }],
)
```
