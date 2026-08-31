---
title: "Agent integrations"
description: "Wiring agent frameworks and developer tools to shaide."
weight: 40
---

# Agent integrations

Anything that speaks the OpenAI API works against shaide. In most cases two environment
variables are sufficient:

```bash
export OPENAI_BASE_URL="https://<endpoint>/v1"
export OPENAI_API_KEY="<key>"
```

## LangChain

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="<model-id>",
    base_url="https://<endpoint>/v1",
    api_key="<key>",
)
```

## LlamaIndex

```python
from llama_index.llms.openai_like import OpenAILike

llm = OpenAILike(
    model="<model-id>",
    api_base="https://<endpoint>/v1",
    api_key="<key>",
    is_chat_model=True,
)
```

## Continue (VS Code / JetBrains)

```json
{
  "models": [{
    "title": "shaide",
    "provider": "openai",
    "model": "<model-id>",
    "apiBase": "https://<endpoint>/v1",
    "apiKey": "<key>"
  }]
}
```

## Generic OpenAI SDK

```javascript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://<endpoint>/v1",
  apiKey: "<key>",
});
```

## Running many agents

shaide serves multiple models, each with multiple replicas, behind a single endpoint.
Agents share that endpoint and the platform load-balances across replicas - no per-agent
routing configuration is needed.

To scale capacity, add replicas rather than endpoints. See
[Scaling](../operations/scaling.md).

## Troubleshooting

| Symptom | Cause |
| --- | --- |
| `404` on a model | `model` does not match `GET /v1/models` |
| Client sends to `api.openai.com` | Base URL not applied - some SDKs need `api_base`, not `base_url` |
| Tool calling ignored | Served model was not trained for tool use |
| `429` under load | Replicas saturated - add replicas |

## Extending models with MCP

shaide can run Model Context Protocol servers as datasources, giving models access to
internal systems — issue trackers, wikis, APIs. Tools exposed by a running datasource
become available to models served by the platform.

See [MCP servers](../operations/mcp-servers.md).
