---
title: "Knowledge Center"
description: "Installing the optional Knowledge Center and its MCP server."
weight: 16
---

# Knowledge Center

The Knowledge Center (KC) stores uploaded documents, indexes them, and serves scoped
retrieval to agents through its own MCP server, **KC-MCP**. It is optional. One choice
installs both:

| Component | Namespace | Stack | Resources |
| --- | --- | --- | --- |
| KC | `app-shaide` | app-shaide | Deployment and ClusterIP Service `knowledge-center:8080` |
| KC-MCP | `app-mcp` | app-mcp | Deployment and ClusterIP Service `kc-mcp:8080` |

KC reuses the platform's RustFS for documents and the shared Qdrant for vectors. Its
collections are named `collection-<id>`, separate from the server's workspace-code
collections. It adds no storage workload or volume.

## Request paths

```text
client ──> shaide-server ──(internal token)──> KC ──> RustFS, Qdrant
                 ▲                              │
                 └──(embedding user account)────┘   embeddings via the server's /v1/embeddings
RustFS ──(webhook token)──> shaide-server /v1/object-storage/event
agent ──> shaide-server MCP gateway ──(gateway token + signed context)──> KC-MCP ──(scoped token)──> KC
```

- shaide-server owns authentication, metadata and upload orchestration. It reaches KC at
  `http://knowledge-center:8080` (`KNOWLEDGE_CENTER_URL`).
- Only KC (and RustFS itself) receive the RustFS credentials, from the storage-owned
  `rustfs-credentials` Secret. shaide-server has no object-storage settings.
- shaide-server discovers KC-MCP like any MCP server, through pod annotations
  `mcp.shaide/datasource: knowledge-center` and
  `mcp.shaide/url: http://kc-mcp.app-mcp.svc.cluster.local:8080`. It signs a short-lived
  context only for that exact origin (`MCP_COLLECTION_CONTEXT_TARGETS`). `MCP_SERVERS` stays
  unset, so Kubernetes discovery of other MCP servers continues.
- There is no public KC search endpoint, and inference requests get no implicit retrieval.
  Agents call KC-MCP explicitly through `/v1/mcp/proxy/knowledge-center/mcp`.

## Configuration

The installer passes the same answer and the same `knowledgecenter.Credentials`
(`pkg/iac/knowledgecenter`) to both stacks. Neither stack generates or rotates a secret.

| Value | Format | app-shaide | app-mcp |
| --- | --- | --- | --- |
| `knowledgeCenterEnabled` | bool | yes | yes |
| `knowledgeCenterImage` / `knowledgeCenterMcpImage` | image reference; the installer mirrors the public GHCR images into Harbor | KC | KC-MCP |
| `kcInternalToken`, `kcWebhookToken` | ≥32 chars, no whitespace, distinct | yes | – |
| `kcGatewayToken`, `kcScopedToken` | unpadded base64url of ≥32 bytes, distinct | yes | yes |
| `kcContextSigningKey` | RSA private key PEM, 2048–8192 bit; ≤32768 original encoded bytes | shaide-server only | – |
| `kcContextPublicKey` | matching public key PEM | KC | KC-MCP |
| `kcEmbeddingUsername`, `kcEmbeddingPassword` | ordinary user account | KC | – |

The gateway token file is at most 4096 **encoded bytes**, matching shaide-server's file
reader; this bound does not apply to the scoped token. Signing PEM begins with the exact
`PRIVATE KEY` or `RSA PRIVATE KEY` delimiter, contains no PEM headers or additional material,
and may have trailing whitespace. The signing file is at most 32768 **original encoded bytes**,
including body and surrounding whitespace; the limit is checked before parsing, not against
DER size, rune count or normalized PEM. It does not restrict public keys or scoped/service
tokens. Valid credential bytes are preserved without normalization.

No KC token may equal `jwtSecret`, and the embedding password may not equal
`adminAuthKey`. Signing PEM must differ from `jwtSecret`, including after surrounding
whitespace is trimmed for comparison. Resolve and direct Load check these rules before
provider or Kubernetes registration; errors name fields without echoing credentials.

Secrets reach the pods as read-only files under `/run/secrets/kc-context` (mode `0440`,
`fsGroup` 10001), readable by the images' user 10001.

Secret updates do not automatically roll these consumers. Credential changes require
deliberate restarts of the affected pods after coordinated Secret updates; automatic secret
rotation and rollout wiring are outside this change.

## Before the first upload (not automated)

Installing KC does not create the embedding account or register an embedding model.
An administrator must, through the existing shaide-server admin and user APIs:

1. Create an **ordinary (non-admin)** user whose name and password match
   `kcEmbeddingUsername` / `kcEmbeddingPassword`.
2. Register the embedding model to use and verify its vector dimensions.

Until both exist, uploads fail with `embedding_auth` or `embedding_model_not_found`.

## Health and readiness

| Component | Probe | What it shows |
| --- | --- | --- |
| KC | `GET /v1/health` on 8080 | The HTTP listener is up. It does **not** check RustFS, Qdrant or the embedding account. |
| KC-MCP | TCP on 8080 | The listener accepts connections. KC-MCP has no HTTP health route and does not check KC. |

The control panel's Knowledge Center entry follows `KNOWLEDGE_CENTER_ENABLED`, which reflects
configured presence, not pod health. Upload, indexing and retrieval checks establish whether
the dependencies actually work.

## Network policies

These are deployed only with KC:

| Policy | Selects | Allows |
| --- | --- | --- |
| `knowledge-center` | KC | ingress from shaide-server and `app-mcp/kc-mcp` on 8080; egress to Qdrant 6334, RustFS 9000, shaide-server 8080, DNS |
| `qdrant-clients` | Qdrant | ingress from shaide-server (workspace code search) and KC on 6333/6334 |
| `rustfs-clients` | RustFS | ingress from KC, and from `rustfsClientNamespaces` (default `monitoring`, for Loki) on 9000 |
| `kc-mcp` | KC-MCP | ingress from `app-shaide/shaide-server` on 8080; egress to KC 8080, DNS |

Kubernetes allow rules add up. The existing namespace-wide `allow-shaide-ingress` in
`app-mcp` still admits every `app-shaide` pod to KC-MCP; KC-MCP's gateway-token check is what
authorizes callers. The RustFS policy also admits all pods in each configured client
namespace, rather than only Loki. If other workloads must reach Qdrant or RustFS, add their namespaces or
policies before enabling KC. Enforcement needs a policy-capable CNI.

## Disabling

Redeploy app-shaide with `knowledgeCenterEnabled=false` to remove KC, its Secrets and
KC policies. Documents and vectors remain in RustFS and Qdrant; the storage Secret stays.
Do not destroy app-shaide to disable KC.

For app-mcp, the handoff depends on what remains selected:

| Remaining selection | Required app-mcp action |
| --- | --- |
| Generic datasources remain | Resolve and deploy with `knowledgeCenterEnabled=false` and the retained generic datasource list; this removes KC-MCP from the desired resources. |
| No generic datasources, previously managed KC-only stack | Use the installer's existing targeted `Deployer.DestroyOnly` path for that app-mcp stack, without normal Resolve or a subsequent Up. |
| Nothing selected and no previously managed app-mcp stack | No app-mcp operation is needed. |

Do not skip a previously managed KC-only app-mcp stack: skipping leaves its resources.
Normal false-plus-empty Resolve and direct Load deliberately reject missing generic
configuration; an empty desired state is not a teardown shortcut.

Before targeted teardown, confirm the installer's ownership record and retained Pulumi state
identify the exact app-mcp project, stack and backend previously managed for this installation.
Use that retained state and secrets-provider access, and the explicitly selected cluster;
the existing `DestroyOnly` cluster-target/UID guard must pass. Review the stack inventory to
confirm no generic or unrelated workloads remain owned by it, and no unrelated workloads
rely on its managed namespace or RBAC. A live KC-MCP pod or a false selection alone does not
prove ownership. If state or ownership is missing or ambiguous, stop for operator review.
Destroy only app-mcp, never app-shaide or shared storage. The separate installer owner must
wire this transition; the platform does not implement that selection or teardown workflow.

## Migrating from `mcp-gateway`

KC-MCP runs only in `app-mcp`. If an existing installation pinned `mcp-gateway`, enabling
KC fails with an explicit error and nothing is moved. See
[MCP servers](mcp-servers.md#enabling-mcp-support).
