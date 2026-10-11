---
title: "Embeddings"
description: "Generating vector embeddings."
weight: 30
---

# Embeddings

`POST /v1/embeddings` - converts text into vectors for search, clustering and retrieval.

## Request

```bash
curl https://<endpoint>/v1/embeddings \
  -H "Authorization: Bearer <key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<embedding-model-id>",
    "input": "The quick brown fox."
  }'
```

## Python

```python
response = client.embeddings.create(
    model="<embedding-model-id>",
    input=["first document", "second document"],
)
vectors = [d.embedding for d in response.data]
```

`input` accepts a string or an array. Batching is significantly more efficient than
one request per document.

## Notes

- Embedding models are served on a separate internal path from generative models and do
  not pass through the inference gateway. This is transparent to clients.
- Vector dimensions are model-specific. Do not mix vectors from different models in one
  index.
- Embedding model IDs appear in `GET /v1/models` alongside generative ones. See
  [Model catalog](model-catalog.md).
