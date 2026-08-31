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

The installer image is self-contained: the Pulumi projects, Helm charts, CRDs and the
image list all ship inside it. Container images are copied into the internal registry
from their origin registries at install time.

The one input you supply is the **model manifest**, which lists the models to download
from Hugging Face and publish into the registry. See
[Model manifest](#model-manifest) below.

The container expects:

| Input | Required | Container path or env | Purpose |
| --- | --- | --- | --- |
| Kubeconfig | Yes | `/.kube/config` by default | Context selection and cluster access |
| Model manifest | Yes | `<STORAGE_PATH>/manifests/models.yaml`, or `MODEL_MANIFEST_PATH` | Models to publish into the registry |
| Persistent storage | Yes | `/var/shaide-installer` | Model cache, upload state, Pulumi state, logs |
| Hugging Face token | Yes | `HF_TOKEN` | Downloads selected model snapshots |
| Registry credentials | Optional | `GHCR_TOKEN`, `DOCKERHUB_PASSWORD` | Private images and rate limits |
| SSH private key | On-prem Harbor install | `PRIVATE_KEY_PATH` | Path inside the container, for Harbor image preload |

## Model manifest

> [!IMPORTANT]
> Supplying `models.yaml` by hand is a temporary step. Model selection moves into the
> installer in the next release, and this file will no longer be required.

The manifest is a `models` list. Each entry names a Hugging Face repository, a pinned
revision, and where the artifact lands in the internal registry:

```yaml
models:
  - id: "openai/gpt-oss-20b"
    revision: "6cee5e81ee83917806bbde320786a8fb61efebee"
    harbor_project: "ai-models"
    harbor_name: "gpt-oss-20b"
    harbor_tag: "1.0.0"
```

| Field | Meaning |
| --- | --- |
| `id` | Hugging Face model repository ID |
| `revision` | Commit to download. Pin it for reproducibility |
| `harbor_project` | Registry project the artifact is published to |
| `harbor_name` | Repository name inside that project |
| `harbor_tag` | Tag used to detect and publish the artifact |
| `dependencies` | Optional extra Hugging Face repos fetched alongside the model |

The entry above is published as
`<registry-host>/ai-models/gpt-oss-20b:1.0.0`, as an OCI artifact of type
`application/vnd.cnai.model`.

### Supplying it

Either drop it into the storage mount, where the installer looks by default:

```bash
mkdir -p "${STORAGE_PATH}/manifests"
cp models.yaml "${STORAGE_PATH}/manifests/models.yaml"
```

Or keep it elsewhere and mount it, pointing `MODEL_MANIFEST_PATH` at the container path:

```bash
-e MODEL_MANIFEST_PATH=/manifests/models.yaml \
-v /tmp/manifests/models.yaml:/manifests/models.yaml:ro \
```

The installer fails at bootstrap with a message naming the expected path if the manifest
is missing.

## Overview

The installation has two parts:

- Installer setup performed by the operator on the provisioner machine.
- Installer stages executed by the shaide installer TUI.

### Installer Setup

| Step                          | Purpose                                                                                            |
|-------------------------------|----------------------------------------------------------------------------------------------------|
| **Prepare installer storage** | Create persistent local storage for the model cache, deployment state, and logs.                   |
| **Configure credentials**     | Export the tokens and passphrases required by the installer.                                       |
| **Write the model manifest**  | List the models to publish into the internal registry.                                             |
| **Define run paths**          | Set the kubeconfig, model manifest, and installer storage paths used by the run command.           |
| **Start the installer**       | Run the installer container with the required mounts, environment variables, and cluster access.   |

### Installer Workflow Stages

| Installer stage       | Action                                                      |
|-----------------------|-------------------------------------------------------------|
| `Bootstrap`           | Validate storage, installer state, and the manifests.       |
| `Kubernetes`          | Load kubeconfig and connect to the target cluster.          |
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
| `MODEL_MANIFEST_PATH` | Model manifest path inside the container. Default `<STORAGE_PATH>/manifests/models.yaml` |
| `PRIVATE_KEY_PATH` | SSH key inside the container, for Harbor image preload on on-prem |

### 3. Set Run Paths

Set shell variables for the local files and directories used by the installer command.

```bash
HOST_KUBECONFIG="$HOME/.kube/config"
MODEL_MANIFEST="$PWD/models.yaml"
STORAGE_PATH="/var/lib/shaide-installer"
```

If SSH-based Harbor preload is required, also set:

```bash
HOST_SSH_DIR="$HOME/.ssh"
```

Verify the required paths exist:

```bash
ls -lh "$HOST_KUBECONFIG"
ls -lh "$MODEL_MANIFEST"
ls -ld "$STORAGE_PATH"
```

### 4. Run Installer Container

```bash
docker run --rm -it \
  --network host \
  -e HF_TOKEN \
  -e PULUMI_CONFIG_PASSPHRASE \
  -e PRIVATE_KEY_PATH \
  -e MODEL_MANIFEST_PATH=/manifests/models.yaml \
  -v "${HOST_KUBECONFIG}:/.kube/config:ro" \
  -v "${MODEL_MANIFEST}:/manifests/models.yaml:ro" \
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
- validates the model and image manifests
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

- checks which model artifacts already exist in Harbor
- asks which missing models should be downloaded
- checks available installer storage
- downloads selected models from Hugging Face
- uploads model artifacts to Harbor
- copies container images from their origin registries into Harbor
- optionally deletes selected model repositories from Harbor


| Prompt                                                   | Options / Input                                                                     |
|---------------------------------------------------------|--------------------------------------------------------------------------------------|
| `Select models to download from manifest`               | Models listed in the model manifest                                                  |
| `No model selected. Are you sure you want to continue?` | `No`, `Yes`                                                                          |
| `Do you want to delete models from Harbor?`             | `No`, `Yes`                                                                          |
| `Select models to delete from Harbor`                   | Existing model repositories in Harbor                                                |
| `Harbor model check failed. <reason>`                   | `Enter new credentials`, `Retry`, `Abort`                                            |
| `Harbor username`                                       | Harbor username                                                                      |
| `Harbor password`                                       | Harbor password                                                                      |
| `Download failed for <model>. <reason>`                 | `Enter new token`, `Retry`, `Abort`                                                  |
| `Hugging Face token`                                    | Valid Hugging Face token                                                             |
| `Hugging Face model was not found.`                     | `Add models manually`, `Abort`                                                       |
| `Upload failed for <target>. <reason>`                  | `Enter new credentials`, `Retry`, `Abort`, `Clear upload state and retry`            |

#### Recommended Actions

- Select all models required for the installation.
- Select `No` if asked to continue without selecting models, unless model download is intentionally skipped.
- Select `No` when asked whether to delete models from Harbor during a normal installation.
- Use `Retry` only after fixing a temporary issue, such as network access, Harbor access, or storage availability.
- Use `Enter new credentials` only when the Harbor credentials are incorrect.
- Use `Enter new token` only when the Hugging Face token is invalid or does not have model access.
- Use `Abort` when the cause is unclear or the model manifest needs correction.
- Use `Clear upload state and retry` only if advised by support or if normal retry does not resolve an inconsistent upload state.

#### Expected Behavior

| Step                    | Action                                                       |
|-------------------------|--------------------------------------------------------------|
| Check model artifacts   | Existing models in Harbor are detected and skipped.          |
| Select models           | Missing models from the model manifest are shown.            |
| Check storage           | Installer storage is checked before downloading models.      |
| Download models         | Selected models are downloaded into the installer cache.     |
| Upload models           | Downloaded model artifacts are uploaded to Harbor.           |
| Upload images           | Container images are copied from their origin registries into Harbor. |
| Delete models           | Optional cleanup for intentionally removed model artifacts.  |

Normal installations should select the required models, avoid deleting existing models, and continue only after storage checks pass.

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
| `Gateway class name` | Selected from the GatewayClasses present in the cluster |
| `Gateway hostname (e.g. shaide.example.com)` | Public hostname for the shared Gateway |

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
| `StorageClass for model PVCs` | Leave empty to use the cluster default |
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
| Installer logs are needed             | Press `Ctrl+Y` in the TUI to save logs under `<STORAGE_PATH>/logs/`.                    |
| `model manifest ... is not readable` | The manifest was not placed under `<STORAGE_PATH>/manifests/`, or `MODEL_MANIFEST_PATH` points elsewhere. |
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
