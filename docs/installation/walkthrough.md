---
title: "Installation walkthrough"
description: "Step-by-step guided installation."
weight: 20
---

# Installation walkthrough

This guide explains how to run the shaide AI Platform installer on a prepared Kubernetes cluster.

The installer is a containerized terminal UI that deploys the shaide platform, uploads required artifacts, configures model-serving workloads, and stores installation state for future updates.

Use this document after completing the [Prerequisites](../getting-started/prerequisites.md).

## Overview

The installation has two parts:

- Installer setup performed by the operator on the provisioner machine.
- Installer stages executed by the shaide installer TUI.

### Installer Setup

| Step                          | Purpose                                                                                            |
|-------------------------------|----------------------------------------------------------------------------------------------------|
| **Load the installer image**  | Import the installer Docker image on the provisioner machine.                                      |
| **Prepare installer storage** | Create persistent local storage for bundle extraction, model cache, deployment state, and logs.    |
| **Configure credentials**     | Export the tokens and passphrases required by the installer.                                       |
| **Define run paths**          | Set the kubeconfig, bundle archive, and installer storage paths used by the run command.           |
| **Start the installer**       | Run the installer container with the required mounts, environment variables, and cluster access.   |

### Installer Workflow Stages

| Installer stage       | Action                                                      |
|-----------------------|-------------------------------------------------------------|
| `Bootstrap`           | Validate storage, installer state, and bundle archive.      |
| `Kubernetes`          | Load kubeconfig and connect to the target cluster.          |
| `Discovery`           | Discover Harbor and determine the installation type.         |
| `Populate Harbor`     | Upload images and selected model artifacts.                 |
| `Deploy platform`     | Deploy Gateway-Provider, App-Serving, and App-Shaide.       |
| `Verify installation` | Check pods, namespaces, gateway resources, and UI access.   |
| `Preserve state`      | Keep storage, logs, deployment state, and passphrase for reruns. |


## Installer Setup
### 1. Load Installer Image

Load the installer Docker image from the provided image archive.

```bash
docker load < onprem-installer.tar.gz
```

Verify that the image is available locally:

```bash
docker images onprem-installer:latest
```

### 2. Prepare Installer Storage

Create the persistent storage directory used by the installer.

```bash
mkdir -p /var/lib/shaide-installer
```

This directory is mounted into the installer container and stores installer state, logs, bundle extraction output, deployment state, and model cache.

Use the same directory for future installer runs.

### 3. Set Credentials

Export the Hugging Face token used for model downloads.

```bash
export HF_TOKEN='hf_...'
```

Export the installer state passphrase.

```bash
export PULUMI_CONFIG_PASSPHRASE='<choose-and-store-securely>'
```

Use the same value for future runs against the same cluster.

If your installer release enables Harbor image preloading over SSH, also export the private key path inside the installer container:

```bash
export PRIVATE_KEY_PATH='/root/.ssh/id_ed25519'
```

Only set `PRIVATE_KEY_PATH` when the release notes or installer package require SSH-based Harbor preload.

### 4. Set Run Paths

Set shell variables for the local files and directories used by the installer command.

```bash
HOST_KUBECONFIG="$HOME/.kube/config"
BUNDLE_ARCHIVE="$PWD/bundle.tar.gz"
STORAGE_PATH="/var/lib/shaide-installer"
```

If SSH-based Harbor preload is required, also set:

```bash
HOST_SSH_DIR="$HOME/.ssh"
```

Verify the required paths exist:

```bash
ls -lh "$HOST_KUBECONFIG"
ls -lh "$BUNDLE_ARCHIVE"
ls -ld "$STORAGE_PATH"
```

### 5. Run Installer Container

```bash
docker run --rm -it \
  --network host \
  -e HF_TOKEN \
  -e PULUMI_CONFIG_PASSPHRASE \
  -e PRIVATE_KEY_PATH \
  -v "${HOST_KUBECONFIG}:/.kube/config:ro" \
  -v "${BUNDLE_ARCHIVE}:/.bundle/bundle.tar.gz:ro" \
  --mount "type=bind,src=${STORAGE_PATH},dst=/var/shaide-installer" \
  onprem-installer:latest
```

The installer requires an interactive terminal, so `-it` is required.

The `--network host` option allows the installer container to reach the Kubernetes API server through the same network path as the provisioner machine.

The bundle must be mounted at:

```text
/.bundle/bundle.tar.gz
```

The persistent storage directory must be mounted at:

```text
/var/shaide-installer
```


## Installer Workflow Stages

After the container starts, the installer runs a terminal UI workflow. The workflow is split into stages. Some stages only validate state, while others ask for operator input.

Duration depends on bundle size, model size, network speed, storage speed, and cluster performance. The estimates below are typical planning ranges, not hard limits.

### `Bootstrap` stage

This stage prepares the installer runtime.

During this stage, it:

- verifies that the installer is running in an interactive terminal
- checks that persistent storage is mounted
- prepares the installer storage directories
- validates the mounted bundle
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


### `Populate Harbor` stage

This stage uploads the artifacts required by the shaide AI Platform.

During this stage, the installer:

- checks which model artifacts already exist in Harbor
- asks which missing models should be downloaded
- checks available installer storage
- downloads selected models from Hugging Face
- uploads model artifacts to Harbor
- uploads bundled container images to Harbor
- optionally deletes selected model repositories from Harbor


| Prompt                                                   | Options / Input                                                                     |
|---------------------------------------------------------|--------------------------------------------------------------------------------------|
| `Select models to download from manifest`               | Models listed in the bundle manifest                                                 |
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
| Select models           | Missing models from the bundle manifest are shown.           |
| Check storage           | Installer storage is checked before downloading models.      |
| Download models         | Selected models are downloaded into the installer cache.     |
| Upload models           | Downloaded model artifacts are uploaded to Harbor.           |
| Upload images           | Container images from the bundle are uploaded to Harbor.     |
| Delete models           | Optional cleanup for intentionally removed model artifacts.  |

Normal installations should select the required models, avoid deleting existing models, and continue only after storage checks pass.

### `Deploy Platform` stage

The `deploy platform` stage deploys the shaide platform stacks through the installer deployment engine.

The installer runs the stacks in this order:

1. Gateway-Provider
2. App-Serving
3. App-Shaide

During this stage, the installer uses the bundled deployment configuration. Most deployment values, such as gateway settings, node selectors, image references, and storage configuration, come from the bundle.

| Prompt                                                   | Options / Input |
|----------------------------------------------------------|-----------------|
| `Is this a model-onboarding scenario`                    | `no`, `yes`     |
| `shaide admin password`                                  | Password input  |

#### Recommended Actions

- Select `no` for a normal installation or update.
- Select `yes` only when intentionally running a model onboarding flow.
- Enter the customer-approved shaide admin password and store it securely.

#### Expected Behavior

| Step             | Action                                               |
|------------------|------------------------------------------------------|
| Gateway-Provider | Gateway resources are deployed successfully.         |
| App-Serving      | Model-serving resources and workloads are deployed.  |
| App-Shaide       | shaide application services are deployed.            |

The stage is complete when all three deployment steps finish successfully.


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

- apply bundle updates
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

If installer state unlock fails, use the original passphrase. Starting with a new passphrase requires discarding the existing deployment state and re-adopting cluster resources.

## What to Keep Safe

Keep the following values and files after installation:

- `PULUMI_CONFIG_PASSPHRASE`: Required to unlock installer-managed deployment state on future runs. Use the same value for every re-run, update, or model onboarding operation against this cluster.

- `<STORAGE_PATH>/`: Contains deployment state, model cache, extracted bundle data, upload state, and installer logs. Preserve this directory across installer re-runs.

- `Harbor admin password`: Required for Harbor administration after installation.

- `Harbor robot credentials`: Required for image push and pull operations.

- `shaide admin password`: Required for shaide administrative access.
