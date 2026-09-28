---
title: "Installer guide"
description: "Installing shaide with the interactive installer."
weight: 10
---

# Installer guide

The installer is a containerized terminal UI that deploys the shaide platform onto a
prepared Kubernetes cluster: it uploads the required artifacts, configures model-serving
workloads, and stores installation state for future updates. It is designed for both
on-prem and cloud targets.

Use this page after completing the [Prerequisites](../getting-started/provisioning-prerequisites.md).

> For the installer's runtime workflow, storage layout and source layout, see
> [Installer](../architecture/installer.md) in the architecture chapter.

## What the installer needs

The installer image is self-contained: the Pulumi projects, Helm charts, CRDs, the
image list and the supported models all ship inside it. Container images are copied into
the internal registry from their origin registries at install time, and the models you
select are downloaded from Hugging Face. See [Supported models](#supported-models) below.

The container expects:

| Input | Required | Container path or env | Purpose |
| --- | --- | --- | --- |
| Kubeconfig | Yes | `/.kube/config` by default | Context selection and cluster access |
| Persistent storage | Yes | `/var/shaide-installer` | Model cache, upload state, Pulumi state, logs |
| Hugging Face token | Yes | `HF_TOKEN` | Downloads selected model snapshots |
| Registry credentials | Optional | `GHCR_TOKEN`, `DOCKERHUB_PASSWORD` | Private images and rate limits |
| SSH private key | On-prem Harbor install | `PRIVATE_KEY_PATH` | Path inside the container, for Harbor image preload |

## Supported models

The installer offers every model packaged under `app_serving/deployments/models/` in the
installer image. You choose which ones to serve in the `Select models` stage; there is
nothing to prepare beforehand.

A model directory is `<category>/<Name>/`, where the category is `generative` or
`embedder`, and it holds a `ms-<slug>/values.yaml` (the llm-d-modelservice chart values)
and a `gaie-<slug>/values.yaml`. The installer reads what it needs from the `ms-*` file:

| Field | Meaning |
| --- | --- |
| `shaide.revision` | Hugging Face commit to download. Required; a full commit sha |
| `shaide.dependencies` | Optional extra Hugging Face repos (`id`, `revision`) fetched alongside the model |
| `modelArtifacts.name` | Hugging Face model repository ID |
| `modelArtifacts.size` | Size of the volume holding the weights |
| `decode.containers[0].resources.limits["nvidia.com/gpu"]` | GPUs per pod, shown in the selector |

The `shaide` block is ignored by the chart. A directory without a pinned revision is not
offered, and the reason is logged at the start of the `Select models` stage.

Each model is published to the internal registry as
`<registry-host>/ai-models/<slug>:<first 12 characters of the revision>`, as an OCI
artifact of type `application/vnd.cnai.model`. Because the tag follows the revision,
pinning a new revision publishes and serves the new weights on the next run.

### Transfer resources

A model transfer is the heaviest thing the installer does, and by default it
takes half of what the machine offers. That keeps a laptop usable while it runs,
and lets a larger machine go faster without any tuning.

"Available" means what the process may actually use. Inside a container that is
the cgroup limit, not the host: `--cpus` sets a CPU-time quota while every host
CPU stays visible, so sizing the work from the visible count oversubscribes the
quota and spends the difference being throttled.

```bash
export RESOURCE_CPU_PERCENT=25      # gentler on a busy workstation
export RESOURCE_MEMORY_PERCENT=25
```

```bash
export TRANSFER_HIGH_PERFORMANCE=1  # dedicated machine, use everything
```

The budget is logged at the start of the download stage, as
`transfer budget: 4 of 8 CPUs, 7.7 GB of 15.4 GB memory`.

Bounding the container as well is still worthwhile on a shared machine, since
the installer cannot limit its own cgroup:

```bash
docker run --rm -it --cpus=4 --memory=6g ...
```

## Overview

The installation has two parts:

- Installer setup performed by the operator on the provisioner machine.
- Installer stages executed by the shaide installer TUI.

### Installer Setup

| Step                          | Purpose                                                                                            |
|-------------------------------|----------------------------------------------------------------------------------------------------|
| **Prepare installer storage** | Create persistent local storage for the model cache, deployment state, and logs.                   |
| **Configure credentials**     | Export the tokens and passphrases required by the installer.                                       |
| **Define run paths**          | Set the kubeconfig and installer storage paths used by the run command.                            |
| **Start the installer**       | Run the installer container with the required mounts, environment variables, and cluster access.   |

### Installer Workflow Stages

| Installer stage       | Action                                                      |
|-----------------------|-------------------------------------------------------------|
| `Bootstrap`           | Validate storage, installer state, and the manifests.       |
| `Kubernetes`          | Load kubeconfig and connect to the target cluster.          |
| `Select models`       | Choose which supported models to install, keep or uninstall. |
| `Assign nodes`        | Assign every node to a model's pool, to CPU work, or to nothing, and label it. |
| `Discovery`           | Discover Harbor and determine the installation type.         |
| `Populate Harbor`     | Upload images and selected model artifacts.                 |
| `Deploy platform`     | Deploy Gateway Provider, App-Serving, App-Shaide, and Monitoring. |
| `Verify installation` | Check pods, namespaces, gateway resources, and UI access.   |
| `Preserve state`      | Keep storage, logs, deployment state, and passphrase for reruns. |


## Installer Setup
### 1. Prepare Installer Storage

Create the persistent storage directory used by the installer.

```bash
mkdir -p /var/lib/shaide-installer
```

This directory is mounted into the installer container and stores installer state, logs, deployment state, and the model cache.

Use the same directory for future installer runs.

### 2. Set Credentials

Export the Hugging Face token used for model downloads.

```bash
export HF_TOKEN='hf_...'
```

Export the installer state passphrase.

```bash
export PULUMI_CONFIG_PASSPHRASE='<choose-and-store-securely>'
```

Use the same value for future runs against the same cluster.

For on-prem installations export the private key path inside the installer container:

```bash
export PRIVATE_KEY_PATH='/root/.ssh/id_ed25519'
```

#### All environment variables

The installer reads these at startup. Anything omitted that is still needed is prompted
for during the run.

| Variable | Purpose |
| --- | --- |
| `PULUMI_CONFIG_PASSPHRASE` | Encrypts installer state. Reuse the same value on every run |
| `HF_TOKEN` | Hugging Face token for model downloads |
| `GHCR_USERNAME` / `GHCR_TOKEN` | Credentials for private GitHub Container Registry images |
| `DOCKERHUB_USERNAME` / `DOCKERHUB_PASSWORD` | Credentials for Docker Hub, and to avoid anonymous rate limits |
| `KUBECONFIG` | Kubeconfig path inside the container. Default `/.kube/config` |
| `PRIVATE_KEY_PATH` | SSH key inside the container, for Harbor image preload on on-prem |
| `RESOURCE_CPU_PERCENT` | Share of available CPUs a model transfer may use. Default 50 |
| `RESOURCE_MEMORY_PERCENT` | Share of available memory a model transfer may use. Default 50 |
| `TRANSFER_HIGH_PERFORMANCE` | Set to `1` to let transfers scale to the whole machine, ignoring the shares above |

### 3. Set Run Paths

Set shell variables for the local files and directories used by the installer command.

```bash
HOST_KUBECONFIG="$HOME/.kube/config"
STORAGE_PATH="/var/lib/shaide-installer"
```

If SSH-based Harbor preload is required, also set:

```bash
HOST_SSH_DIR="$HOME/.ssh"
```

Verify the required paths exist:

```bash
ls -lh "$HOST_KUBECONFIG"
ls -ld "$STORAGE_PATH"
```

### 4. Run Installer Container

```bash
docker run --rm -it \
  --network host \
  -e HF_TOKEN \
  -e PULUMI_CONFIG_PASSPHRASE \
  -e PRIVATE_KEY_PATH \
  -v "${HOST_KUBECONFIG}:/.kube/config:ro" \
  --mount "type=bind,src=${STORAGE_PATH},dst=/var/shaide-installer" \
  ghcr.io/axem-solutions/shaide/installer:oss
```

The installer requires an interactive terminal, so `-it` is required.

The `--network host` option allows the installer container to reach the Kubernetes API server through the same network path as the provisioner machine.

The persistent storage directory must be mounted at:

```text
/var/shaide-installer
```


## Installer Workflow Stages

After the container starts, the installer runs a terminal UI workflow. The workflow is split into stages. Some stages only validate state, while others ask for operator input.

Duration depends on model size, network speed, storage speed, and cluster performance. The estimates below are typical planning ranges, not hard limits.

### `Bootstrap` stage

This stage prepares the installer runtime.

During this stage, it:

- verifies that the installer is running in an interactive terminal
- checks that persistent storage is mounted
- prepares the installer storage directories
- loads the image manifest and the supported models
- loads the required runtime configuration

| Prompt                                                                      | Options     | Recommended |
|-----------------------------------------------------------------------------|-------------|-------------|
| `No persistent storage under installer. Are you sure you want to continue?` | `No`, `Yes` | `No`        |

- `No`: Stop the installer and fix the `/var/shaide-installer` mount before continuing.
- `Yes`: Continue without persistent installer storage. Installer state and logs will not be saved, so future re-runs may not be able to resume from this run.

### `Kubernetes` stage

The Kubernetes stage connects the installer to the target Kubernetes cluster.

During this stage, the installer:

- loads the mounted kubeconfig
- asks you to select a Kubernetes context
- creates the Kubernetes client used by the later installer stages


| Prompt                      | Options                          |
|-----------------------------|----------------------------------|
| `Select Kubernetes Context` | Contexts found in the kubeconfig |

- Select the context that points to the target shaide cluster.
- Do not continue with a context for a different cluster.

### `Select models` stage

This stage decides which models the platform serves after this run.

During this stage, the installer:

- detects which supported models already run on the cluster
- shows every supported model in a table with its type, GPUs per pod, volume size and status
- asks for an action per model

| Status | Actions |
|---|---|
| `Available` | `-` (leave it out), `install` |
| `Installed` | `keep`, `uninstall` |

Every row starts at the action that changes nothing, so an update run that keeps the same
models is a single confirm. Changed rows are marked with `●`, and the focused row's Hugging
Face repository and revision are shown below the table.

| Key | Action |
|---|---|
| `j` / `k`, arrow keys | Move between rows |
| `g` / `G` | Jump to the first row / to `Continue` |
| `ctrl+d` / `ctrl+u` | Move half a page |
| `h` / `l` | Switch the row's action without opening the dropdown |
| `enter` / `space` | Open the row's dropdown, or confirm on `Continue` |
| `u` | Reset the row to its starting action |
| `?` | Show all keys |
| `esc` | Close the dropdown, or cancel |

Uninstalling a model removes it from the serving stack, which deletes its volume and the
weights on it, and deletes its artifact from Harbor. Uninstalling the last served model
destroys the serving stack.

### `Assign nodes` stage

This stage decides which nodes each workload runs on. Every model to serve gets its own
pool of nodes; the platform itself (app-shaide, app-mcp) runs on the CPU only nodes.

During this stage, the installer:

- lists the cluster's nodes with their GPU, CPU and memory, as the NVIDIA GPU Operator and
  the node report them
- leaves out nodes reserved by a taint shaide does not tolerate, such as a control plane
  or a system pool; GPU nodes tainted `nvidia.com/gpu` are listed
- shows the nodes grouped by pool: one group per selected model, `CPU only` and
  `Unassigned`
- labels the nodes once you confirm the changes

A node starts in the group its labels already put it in, so a rerun that changes nothing
is a single confirm. On a first run, nodes without a GPU start in `CPU only` and GPU nodes
in `Unassigned`.

| Rule | Why |
|---|---|
| Every model's pool has at least one node | The model has nowhere to run otherwise |
| A pool has one GPU type (`nvidia.com/gpu.product`) | A model is tuned for one GPU |
| Every node in a pool has at least the GPUs one pod of the model needs | The pod would never schedule |
| At least one node is `CPU only` | The platform itself runs there |

Each group header shows whether its pool is valid, and `Continue` is blocked until every
pool is. A node without a GPU cannot join a model's pool.

| Key | Action |
|---|---|
| `j` / `k`, arrow keys | Move between nodes |
| `1`-`9` | Move the node, or the selected nodes, to that group |
| `enter` / `space` | Open the "move to" dropdown, or confirm on `Continue` |
| `V` | Start or end selecting several nodes |
| `x` | Move the node to `Unassigned` |
| `u` | Put the node back where it started |
| `?` | Show all keys |

The labels the installer sets are booleans set to `"true"`:

| Group | Labels |
|---|---|
| A model's pool | `axem.dev/model-<slug>`, and `axem.dev/workload-generative` or `axem.dev/workload-embedding` |
| `CPU only` | `axem.dev/workload-cpu` |
| `Unassigned` | none |

Any other `axem.dev/model-*` or `axem.dev/workload-*` label on a listed node is removed,
which also clears the pools of uninstalled models. Before applying, the installer logs
every change and asks to confirm; `Back to assignment` returns to the view with the edits
kept.

> [!NOTE]
> On a cloud cluster, labels set here are lost when the node pool replaces a node. Set the
> same labels on the node pool as well.

### `Discovery` stage

The `discovery` stage determines whether this is a fresh installation or an update.

During this stage, the installer:

- detects existing Harbor resources
- decides whether this is a fresh install or update
- deploys Harbor when needed
- creates or validates the Harbor pull secret
- prepares the Harbor connection used by later artifact upload stages

| Prompt                                                                 | Options / Input                                                           |
|------------------------------------------------------------------------|---------------------------------------------------------------------------|
| `Harbor namespace "<name>" was not found`                              | `Fresh install`, `Choose another namespace`, `Abort`                      |
| `Harbor namespace`                                                     | Existing namespace name                                                   |
| `Harbor service "<name>" is not usable in namespace "<namespace>"`     | `Enter service name`, `Choose another namespace`, `Fresh install`, `Abort`|
| `Harbor service`                                                       | Existing service name, usually `harbor`                                   |
| `Harbor secret "<name>" is not usable in namespace "<namespace>"`      | `Enter secret name`, `Choose another namespace`, `Fresh install`, `Abort` |
| `Harbor pull secret`                                                   | Existing pull secret name, usually `harbor-pull-secret`                   |
| `Harbor admin password`                                                | Harbor administrator password                                             |
| `Harbor robot password`                                                | Harbor robot account password                                             |
| `Could not <operation> from Harbor ...`                                | `Retry`, `Continue without Harbor models`, `Fresh install`, `Abort`                    |
| `Harbor image preload failed`                                          | `Retry`, `Abort`                                                          |

#### Recommended Actions

- Choose `Fresh install` only when Harbor does not already exist or this is a new environment.
- Choose `Choose another namespace` when Harbor already exists in a different namespace.
- Use `Enter service name` when Harbor exists but the installer is pointed at the wrong Service.
- Use `Enter secret name` when Harbor exists but the installer is pointed at the wrong pull secret.
- Use `Retry` after fixing temporary Harbor, SSH, port-forward, network, or disk-space issues.
- Use `Abort` when the cluster state is unexpected and needs investigation.
- Store the Harbor admin and robot passwords securely.

| Condition                                                    | Installer behavior                                  |
|--------------------------------------------------------------|-----------------------------------------------------|
| Harbor namespace, service, pull secret, and access are valid | Reuse existing Harbor and continue in update mode.  |
| Harbor resources are missing in a new environment            | Deploy Harbor as part of the fresh-install path.    |
| Existing Harbor resources are incomplete or invalid          | Ask for corrected namespace, service, or secret.    |

#### Harbor image preload

On-prem only, when Harbor is being installed. Harbor's own images are copied onto the
Harbor node over SSH, because Harbor does not yet exist to serve them.

| Prompt | Default |
|---|---|
| `Harbor node IP or hostname` | — |
| `Harbor node SSH user` | — |
| `Remote ctr path` | `/var/lib/rancher/rke2/bin/ctr` |
| `Remote containerd socket` | `/run/k3s/containerd/containerd.sock` |

The SSH key comes from `PRIVATE_KEY_PATH` and must be readable inside the container.


### `Populate Harbor` stage

This stage uploads the artifacts required by the shaide AI Platform.

During this stage, the installer:

- checks which models to serve are already in Harbor
- checks available installer storage for the missing ones
- downloads them from Hugging Face
- uploads model artifacts to Harbor
- copies container images from their origin registries into Harbor
- deletes the artifacts of uninstalled models from Harbor


| Prompt                                                   | Options / Input                                                                     |
|---------------------------------------------------------|--------------------------------------------------------------------------------------|
| `Harbor model check failed. <reason>`                   | `Enter new credentials`, `Retry`, `Abort`                                            |
| `Harbor username`                                       | Harbor username                                                                      |
| `Harbor password`                                       | Harbor password                                                                      |
| `Download failed for <model>. <reason>`                 | `Enter new token`, `Retry`, `Abort`                                                  |
| `Hugging Face token`                                    | Valid Hugging Face token                                                             |
| `Hugging Face model was not found.`                     | `Add models manually`, `Abort`                                                       |
| `Upload failed for <target>. <reason>`                  | `Enter new credentials`, `Retry`, `Abort`, `Clear upload state and retry`            |

#### Recommended Actions

- Use `Retry` only after fixing a temporary issue, such as network access, Harbor access, or storage availability.
- Use `Enter new credentials` only when the Harbor credentials are incorrect.
- Use `Enter new token` only when the Hugging Face token is invalid or does not have model access.
- Use `Abort` when the cause is unclear.
- Use `Clear upload state and retry` only if advised by support or if normal retry does not resolve an inconsistent upload state.

#### Expected Behavior

| Step                    | Action                                                       |
|-------------------------|--------------------------------------------------------------|
| Check model artifacts   | Models to serve that are already in Harbor are skipped.      |
| Check storage           | Installer storage is checked before downloading models.      |
| Download models         | Missing models are downloaded into the installer cache.      |
| Upload models           | Downloaded model artifacts are uploaded to Harbor.           |
| Upload images           | Container images are copied from their origin registries into Harbor. |
| Delete uninstalled models | Artifacts of uninstalled models are deleted from Harbor. A missing artifact counts as deleted. |

### `Deploy Platform` stage

The `deploy platform` stage deploys the shaide platform stacks through the installer deployment engine.

The installer runs the stacks in this fixed order:

1. Gateway Provider
2. App-Serving
3. App-Shaide
4. Monitoring

Most deployment values — node selectors, image references, chart and CRD paths — ship
with the installer. The prompts below cover what cannot be known in advance.

#### Platform and gateway

| Prompt | Options / Input |
|---|---|
| `Cloud platform` | `gcp`, `aws`, `azure`, `on-prem`. Detected from the cluster; confirm or override |
| `Gateway class` | Selected from the GatewayClasses the cluster accepts, plus `istio`, which the gateway stack installs. The class of the existing shared Gateway is pre-selected. Not asked when `istio` is the only option |
| `Gateway hostname (e.g. shaide.example.com)` | Public hostname for the shared Gateway |
| `Azure subnet resource ID for Application Gateway for Containers` | Only with the `azure-alb-external` class. Pre-filled with the existing association on an update |
| `cert-manager ClusterIssuer for Gateway TLS` | Selected from the cluster's ClusterIssuers, or `(none, HTTP only)`. Not asked when the cluster has no ClusterIssuer |

#### TLS

Which certificate prompt appears depends on the platform selected above.

| Prompt | Platform |
|---|---|
| `ACM certificate ARN` | AWS |
| `Application Gateway certificate name` | Azure |
| `GKE Certificate Manager certificate name` | GCP |
| `TLS cert annotation key (empty for none)` | All |
| `TLS certificate reference (usually empty on-prem)` | All |

#### Storage and application

| Prompt | Options / Input |
|---|---|
| `StorageClass for model PVCs` | Pick from the cluster's StorageClasses, or `(cluster default)`. Asked once, only for models whose volume does not exist yet (every model on `Recreate`), pre-selecting the class in use. A model's existing PVC keeps its class, and a class the model manifest sets is used as is. Not asked on-prem, where model volumes use `hostpath` |
| `StorageClass for monitoring PVCs (Loki, Prometheus)` | Pick from the cluster's StorageClasses, or `(cluster default)`. Asked once, and only for volumes that do not exist yet: an existing Loki or Prometheus PVC keeps its class, because a PVC's StorageClass cannot change |
| `Shaide admin password` | Creates the initial administrator account |

#### App-serving deployment mode

Asked when app-serving is deployed onto an existing installation.

| Option | Effect |
|---|---|
| `Update — keep model volumes` | Patches running resources in place. **Default** |
| `Recreate — destroy the stack, deleting model volumes` | Model weights are deleted and must be pulled from Harbor again |

Choose `Recreate` only when a change cannot be applied in place — it discards the model
volumes and makes the next start considerably slower.

#### If a stack fails

Each stack has its own recovery prompt offering `Retry` and `Abort`. Retry after fixing
the underlying cause; the installer resumes from the failed stack rather than restarting
the run.

#### Expected Behavior

| Step             | Action                                               |
|------------------|------------------------------------------------------|
| Gateway Provider | Gateway resources are deployed successfully.         |
| App-Serving      | Model-serving resources and workloads are deployed.  |
| App-Shaide       | shaide application services are deployed.            |
| Monitoring       | Log aggregation and dashboards are deployed.         |

The stage is complete when all four deployment steps finish successfully.


## Verification

After the installer finishes, confirm the platform is up:

```bash
# Harbor pods
kubectl -n harbor get pods

# shaide pods
kubectl -n app-shaide get pods

# Model serving pods (one namespace per model)
kubectl get namespaces | grep llm-d-

# Shared gateway is live
kubectl -n gateway-system get gateway shared-gateway
```
If you have configured DNS for your gateway hostname, the shaide UI is reachable at `https://<your-gateway-hostname>/ui`.

## Re-running the Installer

The installer is safe to re-run.

Use the same values from the previous run:

- `STORAGE_PATH`
- `PULUMI_CONFIG_PASSPHRASE`
- target kubeconfig and Kubernetes context

Re-runs use the existing installer deployment state and allow the installer to continue from the previous installation state.

Use re-runs to:

- apply updates from a newer installer image
- upload new image or model versions
- continue after a failed or interrupted installation
- update existing platform resources

Do not delete the persistent installer storage directory unless you intentionally want to discard local installer state.

## Troubleshooting

| Symptom                               | Action                                                                                  |
|---------------------------------------|-----------------------------------------------------------------------------------------|
| Installer cannot reach the API server | Confirm `kubectl get nodes` works from the provisioner machine.                         |
| Harbor preload fails with SSH error   | Confirm SSH access works from the provisioner machine to the target cluster nodes.      |
| Installer state unlock fails          | Confirm `PULUMI_CONFIG_PASSPHRASE` matches the passphrase used on the previous run.     |
| Installer logs are needed             | Press `Ctrl+Y` in the TUI to save logs under `<STORAGE_PATH>/logs/`. The installer names the file relative to the storage directory, e.g. `logs/installer-logs-20260927-165610.log`. |
| `/var/shaide-installer is not a mount point` | The storage bind mount is missing. The TUI may let you continue, but state and logs will not persist. |
| `Hugging Face token was not set`      | `HF_TOKEN` is required during bootstrap.                                               |
| Image pull failures in the artifact stage | The provisioning machine cannot reach the registry named by an entry's `source`.   |
| `models must be non-empty`            | `app-serving:models` has no enabled generative or embedder entries.                    |
| `no gaie-*` / `no ms-* subdirectory found` | The model folder does not satisfy the values directory contract.                  |
| `slug mismatch`                       | The `gaie-*` and `ms-*` directory suffixes do not match.                               |
| Pulumi stack lock errors              | A previous run left a lock under `<STORAGE_PATH>/pulumi-state`. Inspect the state before removing locks. |

If installer state unlock fails, use the original passphrase. Starting with a new passphrase requires discarding the existing deployment state and re-adopting cluster resources.

Model artifact cache state, when an upload needs inspecting:

```text
<STORAGE_PATH>/artifact-cache/
```

## What to Keep Safe

Keep the following values and files after installation:

- `PULUMI_CONFIG_PASSPHRASE`: Required to unlock installer-managed deployment state on future runs. Use the same value for every re-run, update, or model onboarding operation against this cluster.

- `<STORAGE_PATH>/`: Contains deployment state, model cache, upload state, and installer logs. Preserve this directory across installer re-runs.

- `Harbor admin password`: Required for Harbor administration after installation.

- `Harbor robot credentials`: Required for image push and pull operations.

- `shaide admin password`: Required for shaide administrative access.
