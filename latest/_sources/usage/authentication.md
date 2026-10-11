---
title: "Authentication"
description: "API keys and access control."
weight: 60
---

# Authentication

All API requests require a bearer token:

```
Authorization: Bearer <your-api-key>
```

Most OpenAI-compatible clients send this automatically when given an `api_key` or
`OPENAI_API_KEY`.

## Key management

Keys, users and access control are handled by the shaide server and administered through
the control panel. The initial administrator account is created during installation from
the shaide admin password.

See the [shaide_server](https://github.com/axem-solutions/shaide_server) and
[shaide_control_panel](https://github.com/axem-solutions/shaide_control_panel)
repositories for details.

## Handling

- Treat keys as secrets - never commit them or place them in container images.
- Supply them via environment variables or a secret manager.
- Issue separate keys per application so one can be revoked independently.

## Failures

| Status | Cause |
| --- | --- |
| `401` | Missing, malformed or revoked key |
| `403` | Valid key without permission for the requested model |
