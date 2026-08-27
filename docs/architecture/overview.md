---
title: "Architecture overview"
description: "How the platform is structured."
weight: 10
---

# Architecture overview

![shaide architecture](../assets/architecture.png)

shaide is built in layers, each an independent Pulumi program.

| Layer | Page |
| --- | --- |
| Internal OCI registry | [Platform services](platform-services.md) |
| Istio, Gateway API, GAIE | [Gateway and routing](gateway.md) |
| vLLM + llm-d model stacks | [Model serving](model-serving.md) |
| shaide server, control panel | [Application layer](application-layer.md) |
| MCP datasources | [MCP datasources](mcp.md) |
| Loki, Grafana, Alloy | [Observability](observability.md) |
| The interactive installer | [Installer](installer.md) |

## Deployment order

```
1. Internal OCI registry       infra/cloud-harbor
2. Gateway + Istio             infra/gateway-provider
3. Model serving               app_serving
4. Application layer           app_shaide
5. Optional add-ons            app_mcp, monitoring
```

Ordering is strict: serving depends on the gateway, which depends on the registry.

## Traffic

See [Traffic flow](traffic-flow.md).

## Supporting detail

- [Model deployment flow](model-deployment-flow.md) - definition to running service
- [Model storage](model-storage.md) - weight storage, pull and caching
