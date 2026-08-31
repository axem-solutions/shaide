---
title: "MCP datasources"
description: "Model Context Protocol servers deployed into the shared gateway namespace."
weight: 80
---

# MCP datasources

> For adding datasources and day-to-day management, see
> [MCP servers](../operations/mcp-servers.md).


Pulumi stack that deploys all Kubernetes infrastructure for MCP Server instances in the `mcp-gateway` namespace. One stack deployment covers all MCP datasources for a single customer cluster.

## What it does

MCP Servers are per-datasource bridges (e.g. `example-public`, `example-internal` — see `deployments/Pulumi.TEMPLATE.yaml`) that the Shaide Server calls to answer user queries. This stack creates:

- The `mcp-gateway` namespace
- RBAC so the Shaide Server can watch MCP pod state via the Kubernetes API
- NetworkPolicies that isolate the `mcp-gateway` namespace and control egress per datasource
- A Deployment + Service per datasource

## How it works

### Probes

Three probes are configured on every MCP Server container.

```
Container starts
    └── StartupProbe polling every startupProbePeriod seconds
        (liveness + readiness suspended until startup probe passes)
            └── StartupProbe passes
                    ├── ReadinessProbe — controls traffic routing
                    └── LivenessProbe  — controls container restart
```

**StartupProbe** — buys the container time to initialise before liveness and readiness kick in. Until it passes, Kubernetes disables both other probes entirely, so a slow-starting datasource connection cannot trigger a premature liveness restart. The startup window is `startupProbeFailureThreshold × startupProbePeriod` seconds. This threshold is the one probe parameter overridable per datasource — different datasources have different connectivity startup times.

**ReadinessProbe** — controls whether the pod receives traffic from the Service. Three consecutive failures remove the pod from the Service endpoints; passing adds it back. This is what feeds the `Running` state in Shaide's Kubernetes API Watch: a pod is `Running` when its readiness condition is `True`. The endpoint must validate actual datasource connectivity — not just process liveness. If the datasource is unreachable, the probe must return non-2xx. Otherwise Shaide reports the pod as `Running` while it cannot serve requests.

**LivenessProbe** — checks the MCP container's own health endpoint and restarts the container if the process is unhealthy.

### State observation (Shaide → MCP)

The Shaide Server opens a Kubernetes API Watch on the `mcp-gateway` namespace filtered by label `app.kubernetes.io/component: mcp-server`. This is event-driven — no polling lag — and lets Shaide distinguish `Starting`, `Running`, and `Restarting` states in real time.

This stack provides the prerequisite: every MCP pod carries the required label and the Shaide Server ServiceAccount has `watch` + `list` permission on pods in the `mcp-gateway` namespace.

### CA certificate trust

When a datasource uses a certificate signed by an internal CA, the cert is delivered as a Kubernetes ConfigMap and mounted into the container. The runtime is pointed at the cert via an env var (`NODE_EXTRA_CA_CERTS`, `REQUESTS_CA_BUNDLE`, or `SSL_CERT_FILE` depending on the language).

Two levels of cert configuration exist:

1. **Company-wide** (`companyCACert`) — one ConfigMap `company-internal-ca` shared across all datasources
2. **Per-datasource** (`caCert`) — individual ConfigMap `mcp-<name>-ca`, takes precedence over the company cert

If neither is set, no cert is mounted (use when the datasource has a publicly trusted certificate).

---

## Components

### `main.go`

Stack entrypoint and orchestrator. Creates resources in dependency order:

```
Namespace
  ├── RBAC (Role + RoleBinding)
  ├── NetworkPolicy: allow-shaide-ingress
  ├── ConfigMap: company-internal-ca          (optional)
  └── for each datasource:
        ├── ConfigMap: mcp-<name>-ca          (optional, per-datasource cert)
        ├── Deployment: mcp-<name>
        ├── Service: mcp-<name>
        └── NetworkPolicy: mcp-<name>-egress  (optional, when egressCIDR is set)
```

### `internal/config/config.go`

Loads and validates all Pulumi stack config. Key behaviour:

- `namespace`, `shaideNamespace`, `shaideServiceAccountName`, and `datasources` are `cfg.Require` — a missing value fails at deploy time
- Datasources are loaded as a typed slice via `cfg.RequireObject` — an empty or missing list fails at deploy time
- Per-datasource fields fall back to global defaults using `resolveStr` / `resolveInt` in `mcpdeployment`

### `internal/components/rbac/deploy.go`

Creates namespace-scoped Role and RoleBinding in the `mcp-gateway` namespace:

- `Role` `mcp-pod-reader` — `watch` + `list` on `pods`
- `RoleBinding` `shaide-mcp-pod-reader` — binds the Shaide Server ServiceAccount (`shaide-server` in `app-shaide`) to the role

The Shaide ServiceAccount is **not created here** — it is owned by the `app_shaide` stack.

### `internal/components/networkpolicy/ingress.go`

Creates `allow-shaide-ingress` in the `mcp-gateway` namespace. Namespace-level policy (empty `podSelector`) that allows ingress only from the `app-shaide` namespace. All other ingress to any pod in `mcp-gateway` is implicitly denied.

### `internal/components/networkpolicy/egress.go`

**`DeployEgress`** — per-datasource policy, skipped when `ds.EgressCIDR` is empty. Allows the specific MCP pod (selected by `app.kubernetes.io/name: mcp-<name>`) to reach its datasource IP on its port, plus DNS.

#### How MCP pods reach external datasources

Datasources such as `example-internal` run outside the Kubernetes cluster on the corporate network. Reaching them from an MCP pod requires every layer of the following chain to be correctly configured:

**1. DNS resolution**

MCP pods use CoreDNS for DNS. CoreDNS only resolves Kubernetes-internal names by default — it has no knowledge of `datasource.internal.example.com` or any other internal corporate hostname. A stub zone must be added to the CoreDNS ConfigMap in `kube-system` to forward queries for the internal domain to the corporate DNS server. Without this, the pod's DNS query fails before any TCP connection is attempted.

The datasource hostname is passed to the MCP container via the `env` map in the datasource config (e.g. `DATASOURCE_URL: https://datasource.internal.example.com`). The MCP Server uses this value to resolve and connect to the datasource.

**2. Node-level routing**

Pod traffic leaving the cluster is NAT-ed to the node's IP address before it enters the corporate network. The corporate network must have IP routes in place so that traffic from node IPs can reach the subnets where the datasources reside. This is an infrastructure concern, not a Kubernetes concern.

**3. Corporate firewall**

Even with routing in place, a corporate firewall typically enforces access control between network segments. Firewall rules must explicitly permit traffic from the worker node IP range to each datasource IP and port. Without this, connections time out silently at the application layer.

**4. Egress NetworkPolicy**

If the cluster enforces a default-deny egress baseline, all outbound pod traffic is blocked regardless of routing and firewall. The `mcp-<name>-egress` NetworkPolicy allows the specific MCP pod to send traffic to the datasource IP and port. Without it, traffic is dropped at the node before it reaches the corporate network.

Kubernetes NetworkPolicy only accepts IP/CIDR — hostnames cannot be used as selectors. Resolve the datasource hostname to an IP before deploying and set it as `egressCIDR`. The `egressHost` field is stored as annotation `mcp.shaide/datasource-host` on the NetworkPolicy for operator reference — it has no effect at the network level but makes `kubectl describe networkpolicy mcp-example-internal-egress` readable instead of showing a bare IP.

Layers 1–3 are prerequisites owned by external teams (Cluster Admin, Network, Security) and must be confirmed before any MCP Server is deployed and tested. A failure at any single layer silently breaks datasource connectivity.

### `internal/components/caconfigmap/deploy.go`

Creates the `company-internal-ca` ConfigMap in the `mcp-gateway` namespace. Skipped entirely when `companyCACert` is empty. Returns the created resource so `main.go` can add it to `DependsOn` before datasource deployments reference it.

### `internal/components/mcpdeployment/deploy.go`

Creates the ConfigMap (per-datasource cert), Deployment, and Service for one datasource. Key details:

**Env vars** — arbitrary environment variables from `ds.Env` are injected into the main container. Use this to pass the datasource URL or any other image-specific config the MCP Server needs at runtime (e.g. `EXAMPLE_API_URL`, `DATASOURCE_URL`). Set via the `env` map in the datasource config.

**CA cert validation** — after resolving the effective cert and trust env var, `Deploy()` returns an error immediately if a cert is configured but no trust env var is set. This prevents a silent misconfiguration where the cert exists in the pod but the application never finds it.

**Per-datasource overrides** — `resolveStr` / `resolveInt` apply the precedence rule: datasource value wins, global config is the fallback. Covers `imagePullPolicy`, `healthPath`, resources, `replicas`, and `startupProbeFailureThreshold`.

**Probes** — all three probes (startup, readiness, liveness) use the same `healthPath`. The startup probe's `failureThreshold` is per-datasource overridable because different datasources have different startup times. Readiness and liveness probes use global timing only.

**Service** — `ClusterIP`, named `mcp-<name>`, reachable at `mcp-<name>.mcp-gateway.svc.cluster.local`. Created after the Deployment via `DependsOn`.

---

## CA certificate setup

Required only when datasources use TLS certificates signed by the company's internal CA. Skip entirely if all datasources use publicly trusted certificates.

### Step 1 — Obtain the CA certificate

Request the company root CA certificate in PEM format from the PKI / Security team. If the company uses an intermediate CA, request the full chain (root + intermediate) concatenated in a single PEM file.

### Step 2 — Set the cert in the stack config

```bash
cd app_mcp/deployments
pulumi stack select <stack-name>
pulumi config set companyCACert "$(cat /path/to/company-ca.crt)"
```

The cert is stored in Pulumi stack state (encrypted at rest). On the next `pulumi up`, Pulumi creates the `company-internal-ca` ConfigMap in the `mcp-gateway` namespace.

### Step 3 — Set the trust env var

The env var that points the runtime at the mounted cert depends on the MCP Server language:

```bash
pulumi config set companyCATrustEnvVar NODE_EXTRA_CA_CERTS   # Node.js
pulumi config set companyCATrustEnvVar REQUESTS_CA_BUNDLE    # Python
pulumi config set companyCATrustEnvVar SSL_CERT_FILE         # Go
```

If datasources use different runtimes, override per datasource with `datasources[N].caTrustEnvVar` instead.

### Per-datasource cert override

If one datasource uses a different CA than the rest, set the cert directly on that entry — it takes precedence over `companyCACert` for that datasource only:

```bash
pulumi config set --path 'datasources[0].caCert' "$(cat /path/to/ds-ca.crt)"
pulumi config set --path 'datasources[0].caTrustEnvVar' NODE_EXTRA_CA_CERTS
```

### CA certificate rotation

```bash
pulumi config set companyCACert "$(cat /path/to/new-company-ca.crt)"
pulumi up
```

Kubernetes propagates the updated ConfigMap to running pods within ~60 seconds (kubelet sync period). No pod restart is required unless the MCP Server reads the cert once at startup rather than on each connection — confirm with the application team if unsure.

---

## Stack config

See [`deployments/Pulumi.TEMPLATE.yaml`](deployments/Pulumi.TEMPLATE.yaml) for full documentation of every config key with examples.

Required keys: `namespace`, `shaideNamespace`, `shaideServiceAccountName`, `datasources`.

## Deploying

```bash
cd app_mcp/deployments

# First time
pulumi stack init <stack-name>
pulumi config set app_mcp:namespace mcp-gateway
# ... set remaining required keys and datasources

# Deploy
pulumi up

# Tear down
pulumi destroy
```

---

## Not Yet Implemented

### Pod-level `securityContext`

`runAsNonRoot` prevents the container from running as root. `seccompProfile: RuntimeDefault` applies the container runtime's default syscall filter. Both are required under the `restricted` PodSecurity policy — without them, deployments are rejected by the admission controller in hardened clusters.

### Container-level `securityContext`

`allowPrivilegeEscalation: false` and `capabilities: drop: [ALL]` are standard hardening. `readOnlyRootFilesystem: true` is recommended but may require an `emptyDir` volume if the MCP Server writes temp files at runtime (pip cache, uvicorn sockets, etc.) — confirm with the application team before enabling.
