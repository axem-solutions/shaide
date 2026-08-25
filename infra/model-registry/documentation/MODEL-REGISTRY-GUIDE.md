# Model Pull: HuggingFace vs Harbor

Focus: Kubernetes workloads only.

---

## Case 1: Model Registry = HuggingFace

```
┌─────────────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                                  │
│                                                                     │
│  Deployment                                                         │
│  ┌──────────────────────────────────────────┐                       │
│  │ modelArtifacts:                          │                       │
│  │   uri: "hf://org/model-name"            │                       │
│  └──────────────────────────────────────────┘                       │
│                                                                     │
│  ┌──────────────────────┐                                           │
│  │ Secret               │                                           │
│  │   HF_TOKEN = ***     │                                           │
│  └──────────┬───────────┘                                           │
│             │ env var                                               │
│             │                                                       │
│  ┌──────────▼───────────────────────────────────────────────────┐  │
│  │ Serving Pod                                                  │  │
│  │                                                              │  │
│  │  Step 1   framework reads hf:// URI from deployment spec    │  │
│  │  Step 2   framework downloads model from huggingface.co ───────────► huggingface.co
│  │           using HF_TOKEN                                    │  │
│  │  Step 3   model files stored in ephemeral storage / PV     │  │
│  │  Step 4   model loaded into memory                         │  │
│  │  Step 5   serve requests                                   │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ⚠  Every replica pod repeats Steps 1–4 independently              │
│  ⚠  Pods require outbound internet access to huggingface.co        │
│  ⚠  HF_TOKEN lives inside the cluster as a Kubernetes Secret       │
└─────────────────────────────────────────────────────────────────────┘
```

**Step by step:**

1. The deployment spec references the model via `hf://org/model-name` URI.
2. `HF_TOKEN` is stored as a Kubernetes Secret and injected into the pod
   as an environment variable (required for gated/private models).
3. At pod startup the serving framework resolves the `hf://` URI and
   downloads the model directly from huggingface.co.
4. Model files are stored in ephemeral pod storage or a PV.
5. Model is loaded into memory.
6. Pod is ready to serve requests.

---

## Case 2: Model Registry = Harbor

```
┌─────────────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster                                                  │
│                                                                     │
│  Deployment                                                         │
│  ┌──────────────────────────────────────────┐                       │
│  │ modelArtifacts:                          │                       │
│  │   uri: "/model-cache/hub/models--org--.. │                       │
│  └──────────────────────────────────────────┘                       │
│                                                                     │
│  ┌──────────────────────────────┐                                   │
│  │ Secret                       │                                   │
│  │   HARBOR_USER = robot$...    │                                   │
│  │   HARBOR_PASSWORD = ***      │                                   │
│  └──────────────┬───────────────┘                                   │
│                 │ env vars                                          │
│                 │                                                   │
│  ┌───────────────────────────────┐                                  │
│  │ Harbor  (ClusterIP)           │                                  │
│  │ harbor.harbor:80              │                                  │
│  └───────────────┬───────────────┘                                  │
│                  │                                                  │
│  ┌───────────────▼───────────────┐                                  │
│  │ ORAS Pull Job  (runs once,    │                                  │
│  │ before serving deployment)    │                                  │
│  │                               │                                  │
│  │  Step 1   read Harbor robot  │                                  │
│  │           credentials from   │                                  │
│  │           Secret             │                                  │
│  │  Step 2   oras login Harbor  │                                  │
│  │  Step 3   oras pull OCI      │                                  │
│  │           artifact           │                                  │
│  │  Step 4   write model files  │                                  │
│  │           to PV/PVC          │                                  │
│  └───────────────┬───────────────┘                                  │
│                  │                                                  │
│            PV/PVC (pre-filled, shared)                              │
│                  │                                                  │
│  ┌───────────────▼───────────────┐                                  │
│  │ Serving Pod                   │                                  │
│  │                               │                                  │
│  │  Step 5   mount PV/PVC       │                                  │
│  │  Step 6   framework reads    │                                  │
│  │           local path URI     │                                  │
│  │  Step 7   model loaded into  │                                  │
│  │           memory             │                                  │
│  │  Step 8   serve requests     │                                  │
│  └───────────────────────────────┘                                  │
│                                                                     │
│  ✓  All replica pods share one PV/PVC — no re-download             │
│  ✓  No internet egress required from the cluster                   │
│  ✓  HF_TOKEN never enters the cluster                              │
└─────────────────────────────────────────────────────────────────────┘
```

**Step by step:**

1. The deployment spec references the model via a local path URI pointing
   to the PV/PVC mount (e.g. `/model-cache/hub/models--org--name/...`).
2. Harbor robot account credentials (`HARBOR_USER`, `HARBOR_PASSWORD`) are
   stored as a Kubernetes Secret and injected into the ORAS Pull Job.
3. Before the serving deployment starts, the ORAS Pull Job runs once.
   It reads the credentials from the Secret and authenticates with Harbor.
4. The Job runs `oras pull` to fetch the model OCI artifact from Harbor
   (ClusterIP — no internet required).
5. Model files are written to the shared PV/PVC. The Job completes.
6. The serving pod starts and mounts the pre-filled PV/PVC.
7. The framework reads the local path URI — no download happens.
8. Model is loaded into memory from the mounted volume.
9. Pod is ready to serve requests.

---

## Case 3: Harbor + hostPath (on-prem)

On-prem clusters typically have no cloud storage provider, so a `hostPath`
PersistentVolume is used instead. The model files live directly on the node's
local disk. Because hostPath is node-specific, both the ORAS Pull Job and the
serving pod must be scheduled on the **same node** using a `nodeSelector`.

```
┌─────────────────────────────────────────────────────────────────────┐
│ Kubernetes Cluster (on-prem)                                        │
│                                                                     │
│  Deployment                                                         │
│  ┌──────────────────────────────────────────┐                       │
│  │ modelArtifacts:                          │                       │
│  │   uri: "/model-cache/hub/models--org--.. │                       │
│  └──────────────────────────────────────────┘                       │
│                                                                     │
│  ┌──────────────────────────────┐                                   │
│  │ Secret                       │                                   │
│  │   HARBOR_USER = robot$...    │                                   │
│  │   HARBOR_PASSWORD = ***      │                                   │
│  └──────────────┬───────────────┘                                   │
│                 │ env vars                                          │
│                 │                                                   │
│  ┌───────────────────────────────┐                                  │
│  │ Harbor  (ClusterIP)           │                                  │
│  │ harbor.harbor:80              │                                  │
│  └───────────────┬───────────────┘                                  │
│                  │                                                  │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ Node  (e.g. gpu-node-01)                  nodeSelector: ✓    │  │
│  │                                                               │  │
│  │  ┌──────────────────────────────────────┐                    │  │
│  │  │ PersistentVolume  (hostPath)         │                    │  │
│  │  │   path: /data/model-cache            │                    │  │
│  │  │   storageClass: ""                   │                    │  │
│  │  └──────────────────────────────────────┘                    │  │
│  │                    │                                          │  │
│  │  ┌─────────────────▼────────────────────┐                    │  │
│  │  │ ORAS Pull Job                        │                    │  │
│  │  │   nodeSelector: gpu-node-01          │                    │  │
│  │  │                                      │                    │  │
│  │  │  Step 1  read Harbor credentials    │                    │  │
│  │  │          from Secret                │                    │  │
│  │  │  Step 2  oras login Harbor          │                    │  │
│  │  │  Step 3  oras pull OCI artifact     │                    │  │
│  │  │  Step 4  write model files to       │                    │  │
│  │  │          /data/model-cache  on node │                    │  │
│  │  └──────────────────────────────────────┘                    │  │
│  │                    │                                          │  │
│  │          /data/model-cache  (node local disk)                │  │
│  │                    │                                          │  │
│  │  ┌─────────────────▼────────────────────┐                    │  │
│  │  │ Serving Pod                          │                    │  │
│  │  │   nodeSelector: gpu-node-01          │                    │  │
│  │  │                                      │                    │  │
│  │  │  Step 5  mount hostPath PV          │                    │  │
│  │  │  Step 6  framework reads local      │                    │  │
│  │  │          path URI                   │                    │  │
│  │  │  Step 7  model loaded into memory   │                    │  │
│  │  │  Step 8  serve requests             │                    │  │
│  │  └──────────────────────────────────────┘                    │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  ✓  No internet egress required                                     │
│  ✓  HF_TOKEN never enters the cluster                              │
│  ⚠  ORAS Pull Job and Serving Pod must run on the same node        │
│  ⚠  Serving pod cannot be rescheduled to a different node          │
│     without re-running the ORAS Pull Job on that node              │
└─────────────────────────────────────────────────────────────────────┘
```

**Step by step:**

1. The deployment spec references the model via a local path URI pointing
   to the hostPath mount (e.g. `/model-cache/hub/models--org--name/...`).
2. A `PersistentVolume` is defined with `hostPath: /data/model-cache` and
   bound to the target node.
3. Harbor robot account credentials are stored as a Kubernetes Secret
   and injected into the ORAS Pull Job as environment variables.
4. The ORAS Pull Job is scheduled on the target node via `nodeSelector`.
   It authenticates with Harbor and runs `oras pull`.
5. Model files are written directly to `/data/model-cache` on the node's
   local disk. The Job completes.
6. The serving pod is scheduled on the **same node** via `nodeSelector`
   and mounts the hostPath PV. No download happens.
7. The framework reads the local path URI, loads the model into memory.
8. Pod is ready to serve requests.

**Key constraint:** hostPath binds the workload to a specific node. If the
serving pod is rescheduled to a different node (e.g. after a node failure),
the ORAS Pull Job must be re-run on the new node before the pod can start.

### hostPath and ReadWriteMany (RWX)

hostPath does not support `ReadWriteMany` — it is a node-local directory and
cannot be mounted by pods on different nodes simultaneously. For this use case
that is **not a problem**.

**Why RWX is not needed:**

Writing happens only once. The ORAS Pull Job runs, fills the directory with
model files, and completes. After that the volume is effectively read-only.
All serving pods only read from it — they never write.

`ReadWriteOnce` (RWO) is sufficient because multiple pods **on the same node**
can all mount an RWO volume at the same time. Since hostPath forces all pods
onto the same node anyway (via `nodeSelector`), all serving replicas can read
the model files simultaneously without any conflict.

```
Node: gpu-node-01

  ORAS Pull Job ──write──► /data/model-cache    (runs once, then done)

  Serving Pod 1 ──read───► /data/model-cache  ┐
  Serving Pod 2 ──read───► /data/model-cache  ├── all on same node, RWO is fine
  Serving Pod 3 ──read───► /data/model-cache  ┘
```

RWX would only be needed if serving pods had to run on **multiple nodes** and
all mount the same volume simultaneously. With hostPath that is not possible
regardless of access mode — the volume is physically tied to one node's disk.
If multi-node serving is required, network-attached storage (NFS, Ceph,
Longhorn, etc.) must replace hostPath.

---

## Side-by-side

| | HuggingFace | Harbor (cloud) | Harbor (on-prem hostPath) |
|---|---|---|---|
| URI in deployment | `hf://org/model-name` | `/model-cache/hub/models--org--name/...` | `/model-cache/hub/models--org--name/...` |
| Model source | huggingface.co (internet) | Harbor ClusterIP (internal) | Harbor ClusterIP (internal) |
| Storage | Ephemeral / cloud PV | Cloud PV (any node) | hostPath (node-local disk) |
| Internet egress from pods | Required | Not required | Not required |
| Secret in cluster | `HF_TOKEN` (serving pod) | `HARBOR_USER` + `HARBOR_PASSWORD` (ORAS Job) | `HARBOR_USER` + `HARBOR_PASSWORD` (ORAS Job) |
| Download happens | Every pod, at startup | Once — ORAS Pull Job | Once — ORAS Pull Job per node |
| Pod startup time | Slow (downloads GBs) | Fast (mounts existing volume) | Fast (mounts existing volume) |
| Pod scheduling | Any node | Any node | Fixed — same node as hostPath |
| Node failure | Pod reschedules freely | Pod reschedules freely | ORAS Pull Job must re-run on new node |

---

## HF Cache Layout vs Harbor Pull

### HuggingFace download (provisioner)

```
model-cache/hub/models--<org>--<name>/
  ├─ refs/main              (commit hash)
  ├─ snapshots/<hash>/      (symlinks → blobs)
  │    ├─ config.json       → ../../blobs/<sha256>
  │    ├─ model.safetensors → ../../blobs/<sha256>
  │    └─ tokenizer.json    → ../../blobs/<sha256>
  └─ blobs/<sha256>         (actual file content)
```

Files in `snapshots/` are symlinks. The `blobs/` directory is the
deduplication store — the actual bytes live there only once.

### Harbor pull (Kubernetes Job)

```
/model-cache/hub/models--<org>--<name>/
  ├─ refs/main              (commit hash — preserved)
  ├─ snapshots/<hash>/      (regular files — symlinks resolved during push)
  │    ├─ config.json       (actual file)
  │    ├─ model.safetensors (actual file)
  │    └─ tokenizer.json    (actual file)
  └─ blobs/<sha256>         (actual file — duplicate of snapshots content)
```

ORAS follows symlinks during push, so the blob content is stored twice
after pull: once under `snapshots/` and once under `blobs/`. Disk usage
is approximately 2× compared to the original HuggingFace cache.

### Differences

| | HuggingFace download | Harbor pull |
|---|---|---|
| `snapshots/` files | Symlinks → `blobs/` | Regular files |
| `blobs/` | Deduplication store (1×) | Duplicate of snapshots (~2×) |
| Disk usage | 1× | ~2× |
| Content | ✓ identical | ✓ identical |
| Pinned revision | `revision:` in models YAML | Baked in at upload time |

---

## Will the Existing Deployment Work with Harbor?

**Yes, no changes needed.**

The serving pods mount the PV/PVC and call `from_pretrained(path)` or set
`HF_HUB_CACHE`. Both work identically because:

1. **Same directory layout** — `models--<org>--<name>/snapshots/<hash>/` is
   preserved by ORAS. The `workingDir: /model-cache` in the ORAS pull Job was
   set specifically to match the `HF_HUB_CACHE` layout.

2. **Same file content** — weights, `config.json`, and tokenizer files are
   byte-identical to the original HuggingFace download.

3. **`refs/main` is present** — `from_pretrained` with `HF_HUB_CACHE` uses
   this file to resolve the snapshot directory. ORAS pushes and pulls it
   correctly.

4. **Symlinks vs regular files** — `transformers` / `sentence-transformers`
   reads file content, not the link type. Makes no difference at runtime.

5. **HF_TOKEN already removed** — commits `85c7fd3` and `b553b00` already
   stripped the token from `app_serving`. Serving pods no longer call
   HuggingFace at all.

The only runtime difference the serving pod sees: files in `snapshots/<hash>/`
are regular files instead of symlinks, which `from_pretrained` handles
transparently.
