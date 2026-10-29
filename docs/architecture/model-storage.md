---
title: "Model storage"
description: "How model weights are stored, pulled and cached."
weight: 60
---

# Model storage

## Overview

Model weights are stored as OCI artifacts in Harbor and pulled into a PersistentVolume by a
one-time Kubernetes Job before the inference pod starts. No HuggingFace token is required
anywhere in the cluster at runtime.

The model server Deployment mounts the PVC at `/model-cache` and sets
`HF_HUB_CACHE=/model-cache/hub` and `HF_HUB_OFFLINE=1`, so vLLM resolves the model by its repo
ID (e.g. `<org>/<model-name>`) from the local cache without any network access.

---

## Architecture

```
                  Harbor Registry
                  (harbor.harbor.svc.cluster.local — K8s DNS inside pods)

                  ai-models/
                    <model-a>:<tag>
                    <model-b>:<tag>
                    ...

                       │ ORAS pull Job (runs once per model at deploy time)
                       ▼
                  PersistentVolume
                  Cloud:   RWO, dynamic provisioner (cluster default, e.g. GKE standard-rwo)
                  On-prem: RWO, hostpath (static PV auto-created by Pulumi)

                  /model-cache/
                    hub/
                      models--<org>--<model-name>/
                        snapshots/<hash>/...

                       │ PVC mount
                       ▼
                  ┌──────────────────────────────────────┐
                  │  Inference Pod                        │
                  │  vllm serve <org>/<model-name>        │
                  │  HF_HUB_CACHE=/model-cache/hub        │
                  │  HF_HUB_OFFLINE=1                     │
                  └──────────────────────────────────────┘
```

---

## Stack Configuration

Each model can optionally declare a `modelSource` block in the stack YAML. When present, Pulumi:

1. Creates a RWO PVC for that model (`<slug>-model`).
2. On on-prem with `hostpathNode` set: auto-creates a static `PersistentVolume` bound to that node
   under `/var/lib/hostpath/models/<slug>` (the directory must already exist on the node).
3. Creates a one-time ORAS pull Job that populates the PVC from Harbor.
4. Mounts the PVC read-only into the model server pods and points vLLM at `<modelUri>` in its
   Hugging Face cache (`hub/<org>/<model-name>` serves `<org>/<model-name>` from `/model-cache/hub`).
5. Makes the model server Deployment depend on the Job, preventing the pod from starting with an empty PVC.

The PVC is `ReadWriteOnce`, so all replicas of a model are scheduled onto the node holding it.

### `modelSource` fields

| Field | Required | Description |
|---|---|---|
| `harborRef` | always | Harbor OCI artifact reference, e.g. `harbor.harbor.svc.cluster.local/ai-models/nomic:1.5.0` |
| `modelUri` | always | Path within PVC to the HF hub cache, e.g. `hub/org/model-name` |
| `storageSize` | always | PVC size. Must be ≥ unpacked model size. E.g. `5Gi`, `40Gi` |
| `storageClass` | cloud, optional | Overrides the cluster default StorageClass, e.g. `hyperdisk-balanced`. Ignored on-prem (always `hostpath`). |
| `hostpathNode` | on-prem | Node hostname for the hostpath PV, e.g. `<node-hostname>`. Required on-prem; omit on cloud. |
| `hostpathDir` | on-prem, optional | Absolute path on the node; default: `/var/lib/hostpath/models/<slug>`. |

### Cloud example

```yaml
app-serving:models:
  generative:
    - name: <modelName>
      enabled: true
      nodeSelector:
        nodegroup: <node-pool-name>
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<model>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 40Gi

app-serving:harborHostname: harbor.harbor.svc.cluster.local
app-serving:harborUser: robot$k8s-puller
app-serving:harborToken:
  secure: <encrypted robot secret>
```

### On-prem example

```yaml
app-serving:models:
  embedder:
    - name: <modelName>
      enabled: true
      nodeSelector:
        workload: cpu
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<model>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 5Gi
        hostpathNode: <node-hostname>

app-serving:kubeconfig: ~/.kube/rke2-cluster.yaml
app-serving:harborHostname: harbor.harbor.svc.cluster.local
app-serving:harborUser: robot$k8s-puller
app-serving:harborToken:
  secure: <encrypted robot secret>
```

---

## Uploading Models to Harbor

The installer downloads the selected models from Hugging Face on the provisioning host
and uploads them to Harbor. It downloads only the files vLLM loads, so the artifact and the
model volume hold the served weights without the other formats a repository ships. See [`Populate Harbor` stage](../installation/installer-guide.md#populate-harbor-stage).

---

## What Pulumi Creates (per model with `modelSource`)

| Resource | Name | Details |
|---|---|---|
| `PersistentVolume` | `<slug>-model-pv` | On-prem only; hostpath at `/var/lib/hostpath/models/<slug>` on `hostpathNode` |
| `PersistentVolumeClaim` | `<slug>-model` | RWO; `hostpath` StorageClass on-prem, cluster default on cloud |
| `batch/v1 Job` | `<slug>-model-pull` | ORAS pull; writes `/model-cache/.pulled` marker; idempotent; `backoffLimit: 3`, `ttlSecondsAfterFinished: 86400` |

The Job mounts the `harbor-creds` secret at `/root/.docker/config.json` for ORAS
authentication against Harbor. It runs with `workingDir: /model-cache/hub` so the pulled
artifact extracts to the correct layout.

---

## ORAS Image

The ORAS CLI image (`oras-project/oras:v1.3.1`) is pulled from where the installer mirrored it,
`<harbor>/services/oras-project/oras:v1.3.1`, on every platform. The installer passes the
mirrored reference as `app-serving:orasImage`. A direct `pulumi up` without it falls back to
`ghcr.io/oras-project/oras:v1.3.1`.

---

## How the URI is Constructed

When `modelSource` is configured, Pulumi overrides `modelArtifacts.uri` in the Helm chart:

```
pvc+hf://<slug>-model/<modelUri>
```

For example, with `slug = <slug>` and `modelUri = hub/<org>/<model-name>`:

```
pvc+hf://<slug>-model/hub/<org>/<model-name>
```

The llm-d chart interprets this as:
- Mount PVC `<slug>-model` at `/model-cache`
- Set `HF_HUB_CACHE=/model-cache/hub`
- Set `HF_HUB_OFFLINE=1`
- The inference server resolves `<org>/<model-name>` from the local hub cache

---

## Pulumi Resource Graph (per model)

```
Namespace
  ├── HarborPullSecret   (harbor-creds; created when harborToken is set)
  ├── [PersistentVolume] (<slug>-model-pv; on-prem only, when hostpathNode is set)
  ├── ModelPVC           (<slug>-model; created when modelSource is set)
  ├── OrasJob            (<slug>-model-pull; depends on PVC)
  │
  ├── Gateway            (gateway-<slug> + parameters ConfigMap + llmd-gateway-<slug> Service)
  ├── Router (Helm)      (router-<slug>: InferencePool + EPP)
  ├── ModelServer        (model-<slug> Deployment)  ← depends on Namespace, PVC, OrasJob
  │     ├── HTTPRoute        (every model; Gateway → InferencePool)
  │     └── EmbeddingService (embedder models; ClusterIP port 8200 → pods' port 8000)
```

---

## Job Failure Recovery

If the ORAS Job fails (exhausts `backoffLimit: 3`) and no `.pulled` marker was written:

1. Check logs: `kubectl logs -n <namespace> job/<slug>-model-pull`
2. Delete the failed Job and PVC:
   ```bash
   kubectl delete job <slug>-model-pull -n <ns>
   kubectl delete pvc <slug>-model -n <ns>
   # On-prem: also delete the PV (Pulumi will recreate it)
   kubectl delete pv <slug>-model-pv
   ```
3. Re-run `pulumi up` to recreate them.

The marker file `/model-cache/.pulled` makes the Job idempotent — if the Job is replaced
after a partial pull, the next run will skip to completion immediately if the marker exists.

---

## Harbor Authentication

The `harbor-creds` Kubernetes secret (`kubernetes.io/dockerconfigjson`) is created per
model namespace by Pulumi when `harborToken` is set. It serves two purposes:

1. **`imagePullSecrets`** — allows cluster nodes to pull container images from Harbor (for
   on-prem air-gapped deployments where nodes have no internet).
2. **ORAS Job volume mount** — mounted at `/root/.docker/config.json` inside the ORAS pull
   Job container so `oras pull` can authenticate against Harbor to fetch model artifacts.

The credentials come from `harborUser` (e.g. `robot$k8s-puller`) and `harborToken` in the
stack config.
