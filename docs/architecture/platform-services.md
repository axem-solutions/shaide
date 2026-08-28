---
title: "Platform services"
description: "The internal OCI registry that stores every container image and model weight."
weight: 20
---

# Platform services

Pulumi project that deploys Harbor as an internal OCI model registry onto a Kubernetes cluster.

Works on any cloud provider (GCP, AWS, Azure) or any cluster reachable via kubeconfig.
On-prem Harbor (which uses hostPath PVs and pre-loaded images) lives separately in
`infra/on-prem/pulumi/services/`.

---

## Design

Harbor is deployed as ClusterIP-only (no ingress, no public IP, HTTP on port 80).
It is reachable inside the cluster at `harbor.harbor.svc.cluster.local`.

- **Model upload** (provisioner → Harbor): via `kubectl port-forward`
- **Model pull** (ORAS Job inside cluster → Harbor): K8s DNS, `--plain-http`
- **Node image pulls** (`kubelet`/containerd → Harbor): a DaemonSet, not DNS/TLS — see
  `documentation/NODE_REGISTRY_CONFIG.md`
- **Keeping Harbor populated**: an optional, declarative image mirror — see
  `documentation/IMAGE_MIRROR.md`
- **TLS**: disabled — not needed for an internal-only registry
- **Storage**: dynamic PVCs from the cluster default storage class (e.g. `pd-ssd` on GKE)
- **Trivy**: disabled — requires external connectivity for vulnerability DB updates

---

## Directory Structure

```
cloud-harbor/
├── main.go                      # Entry point; delegates to pkg/iac/harbor
├── deployment/
│   ├── Pulumi.yaml               # Project descriptor
│   └── Pulumi.<stack>.yaml       # Per-cluster stack configs (run pulumi from here)
├── documentation/
│   ├── NODE_REGISTRY_CONFIG.md   # How nodes reach Harbor's ClusterIP to pull images
│   └── IMAGE_MIRROR.md           # How the public/private image mirror works
├── scripts/
│   ├── harbor-setup.sh           # Post-deploy: creates projects + robot account
│   ├── harbor-validate.sh        # Validates admin and robot credentials
│   ├── harbor-reset-robot-secret.sh  # Resets robot password without recreating account
│   └── harbor-image-upload.sh    # One-off manual image upload (see also IMAGE_MIRROR.md)
└── charts/
    └── harbor-1.18.2.tgz         # Harbor Helm chart

pkg/iac/harbor/                   # Actual Pulumi program logic, shared via Go module
├── harbor.go                     # Namespace + Helm release + pull secret + HTTPS port fix
├── setup.go                      # Configures Harbor itself: projects (public) + robot account,
│                                  # via the pulumiverse/pulumi-harbor provider
├── node_trust.go                 # Node-trust DaemonSet (see NODE_REGISTRY_CONFIG.md)
└── mirror.go                     # Image mirror Jobs/CronJob (see IMAGE_MIRROR.md)

pkg/kube/                         # Port-forward helpers setup.go uses to reach Harbor's
                                   # ClusterIP-only Service from wherever pulumi up runs
```

The actual Harbor deployment logic (namespace, Helm release, pull secret) lives in the
shared `pkg` module, at
[`pkg/iac/harbor/harbor.go`](../../pkg/iac/harbor/harbor.go).

---

## Deployment

### Step 0 — Download the Harbor Helm chart

The chart tarball is not committed to the repository. Create the `charts/` directory
and download it before running `pulumi up`.

```bash
mkdir -p infra/cloud-harbor/charts
```

```bash
HARBOR_CHART_VERSION=1.18.2

curl -Lo infra/cloud-harbor/charts/harbor-${HARBOR_CHART_VERSION}.tgz \
  https://github.com/goharbor/harbor-helm/releases/download/v${HARBOR_CHART_VERSION}/harbor-${HARBOR_CHART_VERSION}.tgz
```

Or using Helm:

```bash
HARBOR_CHART_VERSION=1.18.2

helm repo add harbor https://helm.goharbor.io
helm repo update
helm pull harbor/harbor --version ${HARBOR_CHART_VERSION} --destination infra/cloud-harbor/charts/
```

### Step 1 — Deploy and configure Harbor

One `pulumi up`. Set the admin password, the robot password, and which projects
to create — then run.

```bash
cd infra/cloud-harbor/deployment

pulumi config set --secret harbor:adminPassword <password> --stack <stack-name>
pulumi config set --secret harbor:robotPassword <robot-password> --stack <stack-name>
pulumi config set harbor:projects "$(cat <<'EOF'
ai-models
images-infra
images-shaide
EOF
)" --stack <stack-name>

pulumi up --stack <stack-name>
```

This single run deploys Harbor, creates the configured projects as public,
creates (or updates) the `k8s-harbor-sa` robot account with the password above,
and creates the `harbor-pull-secret` Kubernetes secret. No separate script, no
second `pulumi up`. `harbor:robotPassword` is desired state, not a value to
capture from somewhere else — pick anything that meets Harbor's requirement
and set it once.

> The robot password must contain at least one uppercase letter, one lowercase
> letter, one number, and one special character (Harbor registry token service
> requirement).

### Migrating an existing deployment (one-time)

If Harbor is already running on this cluster with projects and a robot account
created the old way (`harbor-setup.sh`, outside Pulumi), import them into
Pulumi's state before the first `pulumi up` after this change — otherwise it
tries to *create* them and Harbor's API rejects it (409, already exists). Not
needed on a genuinely fresh cluster.

Look up the numeric IDs Harbor assigned (`pulumi import` needs these, not the
names):

```bash
kubectl -n harbor port-forward svc/harbor 8080:80 &
PF_PID=$!

curl -s -u admin:<admin-password> http://localhost:8080/api/v2.0/projects | jq -r '.[] | "\(.name) \(.project_id)"'
curl -s -u admin:<admin-password> http://localhost:8080/api/v2.0/robots | jq -r '.[] | "\(.name) \(.id)"'

kill "${PF_PID}"
```

Then, for each project and the robot account:

```bash
cd infra/cloud-harbor/deployment

pulumi import harbor:index/project:Project harbor-project-<name> /projects/<project_id> --stack <stack-name>
pulumi import harbor:index/robotAccount:RobotAccount harbor-robot-k8s-harbor-sa /robots/<robot_id> --stack <stack-name>
```

After every existing project and the robot account are imported, `pulumi up`
behaves like the fresh-install case above — no changes, or it fixes drift
(e.g. a project that's still private) if there is any.

---

## Resetting the Robot Password

The normal path is `pulumi config set --secret harbor:robotPassword <new-password>`
followed by `pulumi up` — `setup.go` PATCHes the robot account's password to
match on every apply.

For an out-of-band rotation without touching Pulumi config first (credentials
compromised, need it rotated immediately):

```bash
cd infra/cloud-harbor

HARBOR_ADMIN_PASSWORD=<password> \
HARBOR_ROBOT_PASSWORD=<new-robot-password> \
  bash scripts/harbor-reset-robot-secret.sh
```

Then reconcile Pulumi's config with what Harbor now actually has:

```bash
cd infra/cloud-harbor/deployment

pulumi config set --secret harbor:robotPassword <new-robot-password> --stack <stack-name>
pulumi up --stack <stack-name>
```

---

## Manual setup (optional fallback)

`pulumi up` (Step 1 above) creates Harbor's projects and robot account on its
own — this is not required on a normal deploy. Kept for cases where scripting
around Harbor's REST API directly is more convenient (debugging, a one-off
cluster that shouldn't be Pulumi-managed, etc).

```bash
cd infra/cloud-harbor

HARBOR_ADMIN_PASSWORD=<password> \
HARBOR_ROBOT_PASSWORD=<robot-password> \
  bash scripts/harbor-setup.sh
```

Creates the same projects and robot account `setup.go` would, via the Harbor
REST API over a `kubectl port-forward`. Validate the result:

```bash
cd infra/cloud-harbor

HARBOR_ADMIN_PASSWORD=<password> \
HARBOR_ROBOT_PASSWORD=<robot-password> \
  bash ../scripts/harbor-validate.sh
```

Runs four checks: admin REST API, admin token service, robot REST API, robot token service.

---

## Stack Config Reference

| Key | Required | Description |
|---|---|---|
| `harbor:adminPassword` | yes | Harbor admin password (secret) |
| `harbor:robotPassword` | no | Desired `k8s-harbor-sa` robot account password (secret) — `pulumi up` creates/updates the account to match. Also gates the pull secret and is required for image mirroring |
| `harbor:projects` | no | Harbor projects to create and keep public, one name per line. No default; unset means `pulumi up` creates none (existing projects, if any, are left alone) |
| `harbor:chartPath` | no | Path to Harbor Helm chart tarball (default: `./charts/harbor-1.18.2.tgz`) |
| `harbor:nodeHostname` | no | Pin all Harbor pods to a specific node (`kubernetes.io/hostname`) |
| `harbor:staticClusterIP` | no | Pin Harbor's Service to a fixed ClusterIP; required for the node-trust DaemonSet |
| `kubeconfig` | no | Path to kubeconfig; omit to use `KUBECONFIG` env / `~/.kube/config` |

**Image mirror** (see `documentation/IMAGE_MIRROR.md` for full detail) — all opt-in, no
defaults baked into the code:

| Key | Required | Description |
|---|---|---|
| `harbor:mirrorEnabled` | no | `true` to deploy the image mirror at all; requires `harbor:robotPassword` too |
| `harbor:publicImages` | no | Static public images, one `src\|dest` pair per line |
| `harbor:ghcrOrg` | no | GitHub org for private image discovery |
| `harbor:ghcrUser` / `harbor:ghcrToken` | no | GHCR credentials; unset skips private-image mirroring entirely (secret for token) |
| `harbor:ghcrSyncMode` | no | `all` (default), `min-version`, or `pinned` |
| `harbor:ghcrMinVersions` | no | `package\|min_version` per line, used by `min-version` mode |
| `harbor:ghcrPinnedImages` | no | Exact `package:tag` pairs, used by `pinned` mode |

---

## Pinning the Harbor ClusterIP (optional)

Setting `harbor:staticClusterIP` makes Harbor's Service address fixed instead of
Kubernetes-assigned, which is what lets the node-trust DaemonSet (see the platform's
node-registry-config docs) bake a stable address into every node's `hosts.toml` at deploy
time. The value must be a free IP inside the cluster's actual Service CIDR — any provider,
any cluster, checked with plain kubectl:

**Existing Harbor deployment** — reuse the ClusterIP it already has, so pinning doesn't
attempt to change an existing (immutable) value:

```bash
kubectl -n harbor get svc harbor -o jsonpath='{.spec.clusterIP}'; echo
```

**Fresh cluster, nothing deployed yet** — the Service CIDR itself isn't exposed as an object
you can `kubectl get`, but the API server enforces it on every Service create, so a
server-side dry run that intentionally requests an out-of-range IP will have it echoed back
in the rejection error:

```bash
kubectl apply --dry-run=server -f - <<EOF
apiVersion: v1
kind: Service
metadata:
  name: cidr-probe
spec:
  clusterIP: 1.1.1.1
  ports:
    - port: 80
EOF
```

Expect something like `provided IP is not in the valid range. The range of valid IPs is
10.96.0.0/16` — nothing is created (`--dry-run=server` validates without persisting), and the
range it reports is authoritative for that specific cluster regardless of cloud provider.
Then pick any free IP inside that range (check `kubectl get svc -A -o
jsonpath='{range .items[*]}{.spec.clusterIP}{"\n"}{end}'` for what's already taken; avoid the
`.1` address, reserved for the `kubernetes` Service itself).

```bash
cd infra/cloud-harbor/deployment

pulumi config set harbor:staticClusterIP <chosen-ip> --stack <stack-name>
pulumi up --stack <stack-name>
```

---

## Uploading Container Images

For ongoing, declarative mirroring driven by stack config, use the image mirror described in
`documentation/IMAGE_MIRROR.md` (`harbor:mirrorEnabled`) instead — it runs as part of
`pulumi up`, no manual script invocation needed. The steps below remain useful for a one-off,
ad hoc upload outside that declared list.

After Harbor is running and its projects exist (`pulumi up`, or the manual fallback
above), mirror all required container images from public registries into Harbor:

```bash
cd infra/cloud-harbor

KUBECONFIG=~/.kube/<stack>.yaml \
HARBOR_ROBOT_PASSWORD=<robot-password> \
  bash ../scripts/harbor-image-upload.sh
```

**Prerequisites:**

- `skopeo` installed on the provisioner
- `kubectl` configured for the target cluster
- `gh` CLI installed and authenticated (`gh auth login`) — required to pull
  private images from `ghcr.io/axem-solutions`

The script opens a `kubectl port-forward` to Harbor, logs in to `ghcr.io` via the
`gh` CLI token, then copies each image directly from the public registry into Harbor
using `skopeo copy`. Images are grouped by use case; MetalLB and GPU Operator images
are commented out (not needed on GKE).

---

## Uploading Models

After Harbor is running, use `infra/model-registry/model-sync.sh` to download
models from HuggingFace and push them as OCI artifacts to Harbor:

```bash
cd infra/model-registry

ENV_FILE=<stack-name> MODELS_YML=<stack-name>.yml bash model-sync.sh
```

The `env-vars/<stack-name>` file must contain:

```bash
HF_TOKEN=hf_...
HARBOR_USER=robot$k8s-harbor-sa
HARBOR_PASSWORD=<robot-password>
```

See `infra/model-registry/env-vars/example` for the full template.

---

## Stack Deployment Order

```
cluster creation (per-cloud, outside this repo)
       ↓
infra/cloud-harbor                       ← this stack
       ↓
infra/gateway-provider                   ← Istio + Gateway API
       ↓
app_serving                              ← model serving
```

`cloud-harbor` does not depend on any cloud-specific stack reference — it only
needs a reachable Kubernetes cluster.
