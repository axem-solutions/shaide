# app_shaide Pulumi Stack

Pulumi Go stack that deploys the Shaide application into Kubernetes namespace `app-shaide`.
This stack owns the application layer (Shaide server, control panel UI, RustFS object storage,
and Qdrant vector database) and wires everything together with shared configuration and
secrets managed through Pulumi.

## Repository Layout

`app_shaide/` itself only contains `main.go` (a thin wrapper that calls
`shaide.DeployAppShaide(ctx)`) and `deployments/`. The actual implementation lives in the
shared `pkg` module, at `pkg/iac/shaide/`:

```
pkg/iac/shaide/
├── shaide.go                              # DeployAppShaide — orchestration entry point
└── internal/
    ├── config/config.go                   # Config loading; typed view of Pulumi stack config
    ├── runtime/context.go                  # DeploymentContext — shared labels + dependency options
    ├── platform/
    │   ├── k8s-serviceaccount.go           # shaide-server ServiceAccount (+ workload-identity annotations)
    │   ├── k8s-rbac.go                     # ClusterRole/ClusterRoleBinding — cluster-wide pod watch
    │   ├── configmap.go                    # shaide-config ConfigMap + shaide-secrets Secret
    │   └── secret.go                       # ghcr-creds pull secret (ghcr.io or Harbor)
    ├── components/
    │   ├── shaide/deploy.go                # shaide-server StatefulSet + Service (+ HTTPRoute)
    │   ├── controlpanel/deploy.go          # control-panel Deployment + Service
    │   ├── webapp/deploy.go                # webapp Deployment + Service
    │   ├── rustfs/deploy.go                # rustfs StatefulSet + Service
    │   └── qdrant/deploy.go                # qdrant StatefulSet + Service
    └── cloudprovider/                      # Provider interface + per-cloud implementations
        ├── provider.go, gcp.go, aws.go, azure.go, on-prem.go, generic.go
```

## What Each Component Does

### `main.go`

Compiles the stack binary and delegates to `shaide.DeployAppShaide(ctx)`.

### `pkg/iac/shaide/shaide.go`

The stack orchestrator and dependency coordinator:
- Loads Pulumi config via `appconfig.Load(ctx)`.
- Creates the Kubernetes provider used by all resources in this stack.
- Creates the namespace first, so all subsequent resources are scoped correctly.
- Creates shared prerequisites: GHCR/Harbor pull secret, `shaide-config` ConfigMap,
  `shaide-secrets` Secret, ServiceAccount, and cluster-wide RBAC.
- Selects a `cloudprovider.Provider` (see below) and calls `ProvisionStorage` before any
  StatefulSet is created.
- Calls per-component `Deploy` functions in a stable order: `shaide`, `controlpanel`,
  `webapp`, `rustfs`, `qdrant`.
- Threads shared dependencies through a `runtime.DeploymentContext` so resources only
  create after their inputs are ready.

### `pkg/iac/shaide/internal/platform/`

- `k8s-serviceaccount.go`: Creates the ServiceAccount used by `shaide-server`
  (`shaideServiceAccountName`). Attaches whatever annotations are set in
  `serviceAccountAnnotations` — a generic map, so the same code handles GKE Workload
  Identity, AKS Workload Identity, EKS IRSA, or nothing at all (on-prem/generic).
- `k8s-rbac.go`: Creates a cluster-scoped `ClusterRole`/`ClusterRoleBinding` granting
  `shaide-server`'s ServiceAccount `get`/`list`/`watch` on `pods` **cluster-wide** — this
  is what lets Shaide observe pod state across every namespace it needs to (its own,
  `app_serving`'s per-model namespaces, `app_mcp`'s namespace, etc.), not just its own namespace.
- `configmap.go`: Creates the shared `shaide-config` ConfigMap (non-secret runtime
  settings, service discovery) and `shaide-secrets` Secret (`adminAuthKey`, `s3Password`).
- `secret.go`: Creates `ghcr-creds` (`kubernetes.io/dockerconfigjson`). Authenticates
  against `ghcr.io` using `ghcrUser`/`ghcrToken` — or, when `harborHostname` is set,
  against that internal Harbor registry instead, using the same two config keys.

### `pkg/iac/shaide/internal/components/`

- `shaide/deploy.go`: Deploys `shaide-server` as a StatefulSet with a PVC mounted at
  `/root/.config` for SQLite persistence, and exposes a Service. The Service is
  `ClusterIP` (paired with an HTTPRoute to a shared Gateway) whenever `infraStackRef` or
  `gatewayHostname` is set; otherwise it's a `LoadBalancer` with annotations from
  `lbAnnotations`. Delegates cloud-specific post-deploy resources to the active
  `cloudprovider.Provider`.
- `controlpanel/deploy.go`: Deploys the control panel as a Deployment (single replica,
  no persistence) with a `ClusterIP` Service on port `3000`.
- `webapp/deploy.go`: Deploys the end-user facing web application as a Deployment
  (single replica, no persistence) with a `ClusterIP` Service on port `8787`. Same shape
  as the control panel — internal-only, points at `shaide-server` via
  `SHAIDE_SERVER_FQDN`/`SHAIDE_SERVER_PORT` env vars.
- `rustfs/deploy.go`: Deploys `rustfs` as a StatefulSet with a PVC for `/data`, an
  `emptyDir` for `/logs`, and a `ClusterIP` Service on port `9000` (plus `9001` when
  `rustfsConsoleEnabled` is set). The container runs as UID/GID `10001` and is
  configured via values from the shared ConfigMap and Secret.
- `qdrant/deploy.go`: Deploys `qdrant` as a StatefulSet with a PVC for
  `/qdrant/storage` and a `ClusterIP` Service exposing REST `6333` and gRPC `6334`.

### `pkg/iac/shaide/internal/cloudprovider/`

A `Provider` interface (`provider.go`) with one implementation per target, selected by
the informational `cloudProvider` config value:

| `cloudProvider` | `ProvisionStorage` | `PostDeployService` |
|---|---|---|
| `gcp` | no-op (GKE `pd.csi.storage.gke.io` dynamic provisioner) | Creates a GKE `HealthCheckPolicy` targeting `/v1/health` |
| `azure` | no-op (AKS `disk.csi.azure.com` dynamic provisioner) | no-op (Workload Identity pod label is applied directly in `shaide/deploy.go`) |
| `aws` | no-op (EBS CSI dynamic provisioner) | no-op |
| `on-prem` | Creates one static, pre-bound hostPath `PersistentVolume` per stateful component (shaide-server, rustfs, qdrant), pinned to `pvNodeHostname` — only when `storageClassName` is `hostpath` | no-op (MetalLB handles LB via the `lbAnnotations` Service annotation) |
| anything else | no-op | no-op |

Any unrecognized `cloudProvider` value falls back to the generic no-op provider — useful
for local/dev clusters that already have a working default StorageClass and don't need a
LoadBalancer at all.

## Component Ports

| Component | Service Name | Ports | Scope |
| --- | --- | --- | --- |
| Shaide server | `shaide-server` | `80` -> `8080` | External (LoadBalancer) or internal (Gateway/HTTPRoute) |
| Control panel | `control-panel` | `3000` | Internal only |
| Web app | `webapp` | `8787` | Internal only |
| RustFS | `rustfs` | `9000` (`9001` when `app_shaide:rustfsConsoleEnabled=true`) | Internal only |
| Qdrant | `qdrant` | `6333`, `6334` | Internal only |

## emptyDir Permissions Note (RustFS)

`emptyDir` does not support direct permission or ownership settings in pod spec. The fix is
an `initContainer` (defined in `pkg/iac/shaide/internal/components/rustfs/deploy.go`) that prepares the filesystem
before the main container starts:
- Runs as root.
- Sets `/data` and `/logs` to mode `0755`.
- Sets ownership to `10001:10001`.
- Starts the main `rustfs` container after permissions are correct.

This matches RustFS expectations (`0o755`) and avoids runtime permission errors.

## Configuration Source of Truth

All parameters are defined in the active stack's `deployments/Pulumi.<stack>.yaml`. Each stack config is the authoritative list of settings for that deployment target, including:
- Namespace and platform behavior (`app_shaide:namespace`, `app_shaide:cloudProvider`,
  `app_shaide:infraStackRef`, `app_shaide:nodeSelector`, `app_shaide:shaideServiceAccountName`).
- Container images (`shaideServerImage`, `controlPanelImage`, `webappImage`, `rustfsImage`, `qdrantImage`).
- Runtime config (`shaideServerS3Fqdn`, `shaideServerS3Port`, `databaseUrl`, `vectorDBUrl`).
- Trial deployment flag (`trial`, defaults to `FALSE`; injected into shaide-server as the
  `TRIAL` env var — only the `trial` stack sets it to `TRUE`).
- RustFS console exposure (`rustfsConsoleEnabled`).
- MCP integration (`mcpNamespace`, optional — see [MCP Integration](#mcp-integration-optional)).
- Secrets (`ghcrToken`, `adminAuthKey`, `s3Password`).

The `nodeSelector` value must match a `nodegroup` label on the target node pool (e.g. `shaide-nodepool`).

## Prereqs

- Kubernetes context points to the target cluster.
- Pulumi stack is selected for this project.
- Required secrets are set (`ghcrToken`, `adminAuthKey`, `s3Password`).

## Workload Identity

`app_shaide:serviceAccountAnnotations` is a generic annotation map applied to the
`shaide-server` ServiceAccount — the same mechanism works for any cloud's workload
identity binding:

- GKE Workload Identity: `iam.gke.io/gcp-service-account: <gsa-email>`
- AKS Workload Identity: `azure.workload.identity/client-id: <managed-identity-client-id>`
  (Azure additionally requires the `azure.workload.identity/use: "true"` pod label,
  which is applied automatically whenever `cloudProvider: azure`.)
- EKS IRSA: `eks.amazonaws.com/role-arn: <role-arn>`
- On-prem/generic: omit `serviceAccountAnnotations` entirely.

`app_shaide:shaideServiceAccountName` (defaults to `shaide-server`) names the
ServiceAccount these annotations are applied to.

Required IAM on the mapped cloud identity (for Vertex AI access): `roles/aiplatform.user`
or the equivalent role on the target cloud.

## MCP Integration (optional)

`app_shaide:mcpNamespace` points `shaide-server` at the namespace where the `app_mcp`
stack is deployed (e.g. `mcp-gateway`). It is optional:
- If set, `MCP_NAMESPACE` is injected into `shaide-config` and `shaide-server` can
  discover/watch MCP pods in that namespace.
- If left unset, `MCP_NAMESPACE` is omitted from `shaide-config` entirely and
  `shaide-server` runs without MCP support — no `app_mcp` deployment is required.

## Security Notes

- Sensitive values live in the `shaide-secrets` Kubernetes Secret created by Pulumi.
- The GHCR token must have `read:packages` to pull the private Shaide image.
- Avoid committing plaintext secrets into stack config; use `pulumi config set --secret`.

## Resource Ownership

This stack owns only the `app-shaide` application layer resources. Cluster-level routing,
Gateways, and cloud infra are expected to be managed by the infra stacks referenced via
`infraStackRef`.

## Data Persistence

Persistent storage is provided by PVCs for Shaide SQLite (`/root/.config`), RustFS
(`/data`), and Qdrant (`/qdrant/storage`). Deleting PVCs will permanently remove stored data.

## Gateway Mode

Routing mode is cloud-agnostic — it depends only on whether a Gateway hostname is
available, not on `cloudProvider`. When either `infraStackRef` (a StackReference to an
infra stack that exports `gatewayHostname`) **or** `gatewayHostname` (set directly) is
non-empty, the Shaide Service becomes `ClusterIP` and an HTTPRoute is created to attach
it to the shared Gateway. When neither is set, Shaide is exposed directly via a
`LoadBalancer` Service with annotations from `lbAnnotations`. The two are mutually
exclusive in practice — set one or the other, not both.

## Config Changes

Update the active stack config (`deployments/Pulumi.<stack>.yaml`), then apply changes with
`pulumi up` to reconcile the cluster.

## Mirror Images (on-prem)

On-prem deployments cannot pull from `ghcr.io` directly. Images must be mirrored into
Harbor from the provisioner laptop before running `pulumi up`.

### 1 — Authenticate with GHCR

The Shaide images are in a private GitHub Container Registry package. Authentication
requires a GitHub Personal Access Token (PAT) with **`read:packages`** scope.

**Option A — use the `gh` CLI (recommended):**

```bash
gh auth login          # follow the prompts; select HTTPS + browser
gh auth token          # prints the token — used below
```

Then authenticate skopeo:

```bash
skopeo login ghcr.io \
  --username "$(gh api user --jq .login)" \
  --password "$(gh auth token)"
```

**Option B — use a PAT directly:**

```bash
skopeo login ghcr.io \
  --username <your-github-username> \
  --password <your-PAT>
```

Credentials are cached in `~/.config/containers/auth.json` for subsequent skopeo calls.

### 2 — Download images

```bash
skopeo copy docker://ghcr.io/axem-solutions/shaide_server:v0.7.0 \
  oci-archive:infra/on-prem/ansible/artifacts/images/shaide_server-v0.7.0.tar
skopeo copy docker://ghcr.io/axem-solutions/control_panel:v0.3.0 \
  oci-archive:infra/on-prem/ansible/artifacts/images/control_panel-v0.3.0.tar
```

### 3 — Upload to Harbor

```bash
cd infra/on-prem/ansible
ansible-playbook -i inventory-dev harbor_upload.yml
```

---

## Deploy

```bash
cd app_shaide
pulumi up
```

## Quick Checks

```bash
# Namespace and workloads
kubectl get ns app-shaide
kubectl get pods -n app-shaide -o wide

# Services and endpoints
kubectl get svc -n app-shaide -o wide
kubectl get endpoints -n app-shaide

# ServiceAccount + WI mapping (GKE)
kubectl get pod -n app-shaide shaide-server-0 -o jsonpath='{.spec.serviceAccountName}{"\n"}'
kubectl get sa -n app-shaide shaide-server -o yaml | rg "iam.gke.io/gcp-service-account"
kubectl exec -n app-shaide shaide-server-0 -- sh -lc \
  'curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/email; echo'

# Shaide health (inside cluster)
# Replace service names if you customized them in Pulumi config.
kubectl run -n app-shaide shaide-health --rm -i --restart=Never --image=curlimages/curl:8.5.0 -- \
  curl -sS http://shaide-server/v1/health

# RustFS permissions and logs
kubectl exec -n app-shaide rustfs-0 -- ls -ld /data /logs
kubectl logs -n app-shaide rustfs-0
```

## Troubleshooting

- `ImagePullBackOff` for shaide-server:
  - Confirm `ghcr-creds` exists in `app-shaide` and contains valid `ghcrUser`/`ghcrToken`.
  - Re-set the token with `pulumi config set --secret ghcrToken <token>` and `pulumi up`.

- RustFS fails with permission errors:
  - Ensure the `fix-permissions` initContainer ran and set `/data` and `/logs` to `0755` with `10001:10001`.
  - Check initContainer logs: `kubectl logs -n app-shaide rustfs-0 -c fix-permissions`.

- Shaide server not reachable:
  - Verify `shaide-server` Service type and endpoints: `kubectl get svc,endpoints -n app-shaide`.
  - If on GCP with `infraStackRef`, confirm HTTPRoute exists: `kubectl get httproute -n app-shaide`.

- Vertex returns `PERMISSION_DENIED`:
  - Verify `shaide-server` uses the expected KSA and that KSA is annotated with
    `iam.gke.io/gcp-service-account`.
  - Ensure the mapped GSA has `roles/aiplatform.user`.

- Qdrant not responding:
  - Check pod status and PVC binding: `kubectl get pods,pvc -n app-shaide`.
  - Confirm ports are open in the Service: `kubectl describe svc -n app-shaide qdrant`.
