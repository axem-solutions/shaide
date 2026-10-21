---
title: "MCP servers"
description: "Adding Model Context Protocol datasources and making them available to shaide."
weight: 15
---

# MCP servers

shaide can run **MCP (Model Context Protocol) servers** as datasources — containerised
connectors that give models access to systems such as issue trackers, wikis or internal
APIs.

Each datasource runs as its own deployment in a dedicated namespace (`app-mcp`), with
its own network policies and, where needed, its own CA trust. The optional
[Knowledge Center](knowledge-center.md) adds its own MCP server, KC-MCP, to the same
namespace.

## Enabling MCP support

MCP is optional. The shaide server discovers MCP servers in one namespace, `app-mcp` by
default:

```yaml
app-shaide:mcpNamespace: app-mcp
```

No datasource deployment is required; with none, shaide simply lists no MCP servers.

> Earlier installations used `mcp-gateway`. A stack that set that namespace explicitly
> keeps it — nothing is moved or deleted. To migrate, redeploy the MCP servers into
> `app-mcp` and set both `app-shaide:mcpNamespace` and `app-mcp:namespace` to `app-mcp`.
> The Knowledge Center refuses to deploy until this is done.

## Adding a datasource

Datasources are declared as a list in the MCP stack configuration. The minimum is a name
and an image:

```yaml
app-mcp:shaideNamespace: app-shaide
app-mcp:shaideServiceAccountName: shaide-server
app-mcp:datasources:
  - name: example
    image: <registry>/mcp-example:<tag>
    port: 8080
    args:
      - --transport
      - streamable-http
```

Resource names are derived from `name` — the example above becomes `mcp-example`.

> `shaideServiceAccountName` must match the value in the `app_shaide` stack. A mismatch
> produces `403 Forbidden` when shaide tries to watch MCP pods.

### Datasource fields

| Field | Required | Purpose |
| --- | --- | --- |
| `name` | Yes | Identifier; resources are named `mcp-<name>` |
| `image` | Yes | Fully qualified container image |
| `port` | No | MCP server listen port. Default `8080` |
| `args` | No | Command-line arguments for the container |
| `env` | No | Environment variables — typically the datasource URL |
| `secretEnv` | No | Environment variables sourced from Kubernetes Secrets |
| `replicas` | No | Pod replica count. Default `1` |
| `healthPath` | No | HTTP probe path. Default `/health` |
| `disableProbes` | No | `true` for images with no health endpoint |
| `cpuRequest` / `memoryRequest` | No | Defaults `100m` / `128Mi` |
| `cpuLimit` / `memoryLimit` | No | Defaults `500m` / `512Mi` — raise for large payloads |

### Credentials

Pass secrets by reference, never inline:

```yaml
    secretEnv:
      - name: EXAMPLE_API_TOKEN
        secretName: mcp-secrets
        secretKey: EXAMPLE_API_TOKEN
```

The Secret must already exist in the MCP namespace.

### Reaching a private datasource

Kubernetes `NetworkPolicy` accepts IP ranges, not hostnames, so egress to an internal
system is allowed by CIDR:

```yaml
  - name: internal-wiki
    image: <registry>/mcp-wiki:<tag>
    egressHost: wiki.internal.example.com   # annotation only, for operator reference
    egressCIDR: 10.20.5.10/32               # what the policy actually enforces
    egressPort: 443
```

Omit `egressCIDR` to skip the egress policy entirely — appropriate for datasources reached
over the public internet.

### Internal TLS

When a datasource presents a certificate signed by an internal CA, supply the CA and tell
the container runtime which variable to trust it through:

```yaml
app-mcp:companyCACert: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
app-mcp:companyCATrustEnvVar: NODE_EXTRA_CA_CERTS
```

Pick the variable that matches the image's runtime:

| Runtime | Variable |
| --- | --- |
| Node.js | `NODE_EXTRA_CA_CERTS` |
| Python | `REQUESTS_CA_BUNDLE` |
| Go | `SSL_CERT_FILE` |

A single datasource can override this with its own `caCert` and `caTrustEnvVar`.

## Deploying

Apply the MCP stack after `app_shaide`. Datasources appear as pods in the MCP namespace:

```bash
kubectl -n app-mcp get pods
```

## How shaide uses them

The shaide server watches the MCP namespace for pods annotated with
`mcp.shaide/datasource` (the server id) and `mcp.shaide/url` (where to reach it). Every
MCP pod this stack creates also carries `app.kubernetes.io/component: mcp-server`. Do not
set `MCP_SERVERS` on the server: a static list replaces this discovery. The watch is event-driven, so state changes are
reflected immediately — shaide distinguishes **Starting**, **Running** and **Restarting**
per datasource rather than polling.

Once a datasource is running, its tools are available to models served by the platform.
Datasource status is surfaced in the control panel.

## Troubleshooting

| Symptom | Cause |
| --- | --- |
| Datasource never appears in shaide | `app-shaide:mcpNamespace` differs from `app-mcp:namespace` |
| `403 Forbidden` on the pod watch | `shaideServiceAccountName` does not match the `app_shaide` stack |
| Pod killed during startup | Slow datasource — raise `startupProbeFailureThreshold` (default 30 checks ≈ 150s) |
| Pod never becomes ready | Image has no `/health` endpoint — set a `healthPath` or `disableProbes: true` |
| TLS errors reaching the datasource | Missing CA cert, or the wrong `caTrustEnvVar` for the image's runtime |
| Connection timeouts to an internal system | `egressCIDR` missing or wrong — the egress policy is dropping the traffic |

Design detail and the full component breakdown: [MCP datasources](../architecture/mcp.md).
