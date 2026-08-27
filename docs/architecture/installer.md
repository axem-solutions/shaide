---
title: "Installer"
description: "How the installer is built, what it does at runtime, and how to work on it."
weight: 95
---

# Installer

Internals of the interactive installer: its runtime workflow, storage layout, source
layout, and the tasks involved in changing it.

For installing shaide, see the [Installer guide](../installation/installer-guide.md).

## Building the installer image


Run the Docker build from the repository root:

```bash
docker build -f installer/build/Dockerfile -t onprem-installer:latest .
```

Use this when changing installer code. It does not refresh the bundle;
changing images, manifests, Pulumi stack files, charts, CRDs, or deployment
values requires rebuilding the bundle archive separately.

## How it works

The installer has two separate moving parts:

- the installer container image, built from `installer/build/Dockerfile`;
- the bundle archive, mounted at runtime as `/.bundle/bundle.tar.gz`.

At runtime the installer:

1. verifies it is running in an interactive terminal;
2. verifies and prepares persistent installer storage;
3. extracts the mounted bundle into `/var/shaide-installer/bundle`;
4. reads `manifests/images.yaml` and `manifests/models.yaml`;
5. reads the mounted kubeconfig and lets the user select a context;
6. discovers whether Harbor already exists in the selected cluster;
7. deploys or configures Harbor when needed;
8. downloads selected Hugging Face models and uploads model/image artifacts to
   Harbor through ORAS;
9. runs the bundled Pulumi workdirs through Pulumi Automation API;
10. writes runtime-generated stack config, secrets, cache files, upload state,
    Pulumi state, and logs under `/var/shaide-installer`.

The installation payload is not baked into the installer image. Rebuilding the
installer image changes the TUI and workflow code only. Changing images, model
manifests, charts, CRDs, deployment values, or Pulumi stack files requires
refreshing the bundle.

## Workflow Stages

The default workflow is defined in `installer/internal/workflow/workflow.go`.

| Stage | Purpose |
| --- | --- |
| `bootstrap` | Check terminal/storage, extract the bundle, load manifests, require `HF_TOKEN`, and read `GHCR_TOKEN`. |
| `initK8s` | Load kubeconfig, prompt for a Kubernetes context, and build the Kubernetes client. |
| `discovery` | Find or deploy Harbor, create Harbor projects and robot credentials, and open a local port-forward. |
| `populate Harbor` | Check model artifacts, download selected models, upload models, upload service images, and optionally delete models. |
| `deploy AI platform` | Deploy `app-serving`, `gateway-provider`, and `app-shaide` Pulumi stacks. |

Harbor discovery uses these defaults from `installer/internal/config/config.go`:

| Setting | Default |
| --- | --- |
| Namespace | `harbor` |
| Service | `harbor` |
| Pull secret | `harbor-pull-secret` |
| Local port-forward port | `5000` |
| Model project | `ai-models` |
| Robot account | `robot$k8s-harbor-sa` |

If the default Harbor namespace, service, or pull secret is missing or invalid,
the recovery flow prompts the user to install Harbor, retry with another
resource name, continue an update flow, or abort.

## Repository Layout

| Path | Purpose |
| --- | --- |
| `installer/cmd/onprem-installer` | Installer entrypoint. |
| `installer/internal/config` | Runtime defaults, bundle extraction, manifest parsing, and storage paths. |
| `installer/internal/workflow` | Stage runner, recovery behavior, and workflow state. |
| `installer/internal/workflow/stages` | Bootstrap, Kubernetes, discovery, artifact, and Pulumi stages. |
| `installer/internal/ui` | Bubble Tea terminal UI. |
| `installer/internal/harbor` | Harbor auth and API helpers. |
| `installer/internal/huggingface` | Hugging Face download integration. |
| `installer/internal/oras` | OCI artifact and image upload logic. |
| `installer/internal/preloader` | SSH/containerd preload support for Harbor bootstrap images. |
| `installer/internal/iac` | Pulumi Automation API wrapper. |
| `installer/build/Dockerfile` | Container image build. |
| `installer/installer-bundle` | Bundle staging trees, manifests, Pulumi workdirs, charts, CRDs, and values. |
| `installer/documentation` | Supporting developer notes and older troubleshooting references. |
| `shaide-installer-data` | Typical host-side persistent runtime storage when using the local run command above. |

## Runtime Storage

The host storage mount is the installer scratch and state root. With the local
run command above, container paths under `/var/shaide-installer` appear on the
host under `shaide-installer-data`.

| Container path | Purpose |
| --- | --- |
| `/.kube/config` | Mounted kubeconfig. |
| `/.bundle/bundle.tar.gz` | Mounted bundle archive. |
| `/var/shaide-installer` | Persistent installer storage root. |
| `/var/shaide-installer/bundle` | Extracted bundle. |
| `/var/shaide-installer/model-cache` | Hugging Face model cache. |
| `/var/shaide-installer/upload-state` | ORAS upload state for resumable uploads. |
| `/var/shaide-installer/artifact-cache` | OCI artifact cache used by model uploads. |
| `/var/shaide-installer/pulumi-state` | Local Pulumi state. |
| `/var/shaide-installer/logs` | TUI log exports. |
| `/var/shaide-installer/tmp` | Process-wide temporary files, including ORAS upload spools. |

The bundle extractor writes `.bundle-extract.json` under the extracted bundle
directory and reuses an extraction when the mounted archive path, size, and
mtime still match.

Press `ctrl+y` in the TUI to save visible installer logs to
`/var/shaide-installer/logs/`.

## Common Developer Tasks

### Change Installer Code

1. Update Go code under `installer/cmd` or `installer/internal`.
2. Run `cd installer && go test ./...`.
3. Rebuild the installer image.
4. Reuse the existing bundle unless the payload contract changed.

### Add A Service Image

1. Add an entry under `harbor_upload_images` in `manifests/images.yaml`.
2. Set `source` to the registry it is fetched from, plus `project`, `name`, and `tag`.
3. Use a Harbor project the installer provisions: `ai-models`, `shaide`, `services`.
4. Rebuild `installer/installer-bundle/bundle.tar.gz`.

No archive is staged — the installer pulls the image from the named registry.

### Add A Harbor Bootstrap Image

On-prem only.

1. Add an entry under `goharbor_images` with `source: archive`.
2. Stage the archive under `images/`, named `<name-with-slashes-as-dashes>-<tag>.tar`.
3. Confirm the preloader can reach the target Harbor node over SSH.
4. Rebuild and verify the bundle.

The current preloader options in `discovery.preloadHarbor` include
environment-specific host, user, SSH key, node, containerd socket, and `ctr`
path values. Treat those as developer-local wiring until they are moved into
runtime config.

### Add A Model Artifact

1. Add the model to `manifests/models.yaml`.
2. Set `id`, `harbor_project`, `harbor_name`, `harbor_tag`, and a pinned
   `revision`.
3. Add `dependencies` when the model requires additional Hugging Face repos.
4. Rebuild the bundle.
5. Run the installer and select the model when prompted.

### Add Or Modify An App-Serving Deployment

1. Add or update a folder under
   `deployments/app-serving/deployments/models/<category>/<model-name>/`.
2. Ensure the folder has one `gaie-*` directory and one `ms-*` directory.
3. Ensure both contain `values.yaml`.
4. Ensure the `gaie-*` and `ms-*` slugs match.
5. Add or update the matching entry in `Pulumi.serving.yaml`.
6. If the deployment uses a Harbor model artifact, point `modelSource.harborRef`
   at the manifest artifact destination.

### Refresh Pulumi Deployment Assets

1. Update stack files, charts, CRDs, or values under the bundle staging tree.
2. Keep secrets out of the stack files.
3. Rebuild `bundle.tar.gz`.
4. Run a local development install/update against a disposable cluster when the
   change affects stack behavior.
