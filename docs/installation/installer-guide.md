---
title: "Installer guide"
description: "Installing shaide with the interactive installer."
weight: 10
---

# Installer guide

The AI Platform Installer is a containerized terminal UI for installing or updating the AI Platform
on a Kubernetes cluster.

It is designed for on-prem and cloud deployments. The installer image contains the application code, and the installation payload is supplied separately as a mounted bundle archive.

> For the installer's runtime workflow, storage layout and source layout, see
> [Installer](../architecture/installer.md) in the architecture chapter.


## What the installer needs

The installer is driven by a **bundle** — a gzip-compressed tar archive holding the
Pulumi deployments, manifests, charts and CRDs it installs from, plus Harbor's bootstrap
image archives on on-prem targets.

Preparing one, and the full manifest schema, is covered in
[Installer bundle](bundle.md).

## Running the installer

The installer container expects:

| Input | Required | Container path or env | Purpose |
| --- | --- | --- | --- |
| Kubeconfig | Yes | `/.kube/config` by default | Kubernetes config used for context selection and cluster access. |
| Bundle archive | Yes | `/.bundle/bundle.tar.gz` by default | Installer payload extracted during bootstrap. |
| Persistent storage | Yes | `/var/shaide-installer` | Bundle extraction, model cache, upload state, temporary files, Pulumi state, and logs. |
| Hugging Face token | Yes | `HF_TOKEN` | Downloads selected model snapshots from Hugging Face. |
| GHCR token | Optional | `GHCR_TOKEN` | Used for optional private image access only. |
| SSH private key path | Yes for Harbor install/preload | `PRIVATE_KEY_PATH` | Path inside the container to the SSH private key used for Harbor image preloading. |

The installer also supports these path override environment variables:

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `KUBECONFIG` | `/.kube/config` | Kubeconfig path inside the container. |
| `BUNDLE_ARCHIVE_PATH` | `/.bundle/bundle.tar.gz` | Bundle archive path inside the container. |
| `PRIVATE_KEY_PATH` | none | SSH private key path inside the container for Harbor image preloading. |


Run the installer as an interactive container:

```bash
mkdir -p "${STORAGE_PATH}"

docker run --rm -it \
  --network host \
  -e HF_TOKEN="${HF_TOKEN}" \
  -e GHCR_TOKEN="${GHCR_TOKEN}" \
  -e PRIVATE_KEY_PATH="${PRIVATE_KEY_PATH}" \
  -v "${HOST_KUBECONFIG}:/.kube/config:ro" \
  -v "${HOST_SSH_DIR}:/root/.ssh:ro" \
  -v "${BUNDLE_ARCHIVE}:/.bundle/bundle.tar.gz:ro" \
  --mount "type=bind,src=${STORAGE_PATH},dst=${DST_BOUND_MOUNT}" \
  onprem-installer:latest
```

The `-it` flags are required because the installer is a TUI. `--network host`
lets the container use local Kubernetes and Harbor port-forwarding behavior
without extra Docker port mapping. 

During a fresh Harbor install, the TUI prompts for the Harbor node IP or
hostname, SSH user, remote `ctr` path, and remote containerd socket used by the
image preloader.

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `/.bundle/bundle.tar.gz does not exist` | The bundle archive was not mounted, or `BUNDLE_ARCHIVE_PATH` points to the wrong path. |
| `/var/shaide-installer is not a mount point` | The host storage bind mount is missing. The TUI may let you continue, but cache/state/logs will not persist. |
| `Hugging Face token was not set` | `HF_TOKEN` is required during bootstrap. |
| `stat archive for image ...` | A `source: archive` entry has no matching file under `images/`. Check the derived filename: `name` with `/` replaced by `-`, then `-<tag>.tar`. |
| Image pull failures during the artifact stage | The provisioning machine cannot reach the registry named by an entry's `source`. |
| `models must be non-empty` | `app-serving:models` has no enabled generative or embedder entries. |
| `no gaie-* subdirectory found` or `no ms-* subdirectory found` | The app-serving model folder does not satisfy the values directory contract. |
| `slug mismatch` | The `gaie-*` and `ms-*` directory suffixes do not match. |
| GKE auth errors | Mount the host gcloud config and authenticate on the host before running the container. |
| Pulumi stack lock errors | A previous run left a lock under `/var/shaide-installer/pulumi-state`. Inspect the state before cancelling or removing locks. |

For exported TUI logs, press `ctrl+y` and inspect:

```text
shaide-installer-data/logs/
```

For model artifact cache state, inspect:

```text
shaide-installer-data/artifact-cache/
```
