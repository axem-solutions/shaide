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
docker build -f installer/build/Dockerfile -t installer:local .
```

Rebuild the image after changing installer code, Pulumi projects, charts, CRDs, the
image manifest, the supported models, or deployment values: all of them ship inside it.

## How it works

Everything the installer deploys ships in the installer container image, built from
`installer/build/Dockerfile`.

At runtime the installer:

1. verifies it is running in an interactive terminal;
2. verifies and prepares persistent installer storage;
3. copies the packaged Pulumi projects from `/opt/shaide-installer/projects` into
   `/var/shaide-installer/projects` and validates them;
4. reads the packaged image manifest and the supported models from the app-serving
   project's `deployments/models`;
5. reads the mounted kubeconfig and lets the user select a context;
6. lets the user choose which models to install, keep or uninstall;
7. discovers whether Harbor already exists in the selected cluster;
8. deploys or configures Harbor when needed;
9. downloads the selected Hugging Face models that Harbor lacks and uploads model/image artifacts to
   Harbor through ORAS;
10. runs the Pulumi projects through the Pulumi Automation API;
11. writes runtime-generated stack config, secrets, cache files, upload state,
    Pulumi state, and logs under `/var/shaide-installer`.

The installation payload ships inside the installer image, so the image version pins the
platform version, including which models it can serve.

## Workflow Stages

The default workflow is defined in `installer/internal/workflow/workflow.go`.

| Stage | Purpose |
| --- | --- |
| `bootstrap` | Check terminal/storage, prepare the Pulumi projects, load manifests, require `HF_TOKEN`, and read `GHCR_TOKEN`. |
| `initK8s` | Load kubeconfig, prompt for a Kubernetes context, and build the Kubernetes client. |
| `select models` | Detect installed models from their pods' `axem.dev/model-slug` label and ask for an action per supported model. |
| `discovery` | Find or deploy Harbor, create Harbor projects and robot credentials, and open a local port-forward. |
| `populate Harbor` | Check model artifacts, download and upload the models to serve that Harbor lacks, upload service images, and delete uninstalled models. |
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
| `installer/cmd/installer` | Installer entrypoint. |
| `installer/internal/config` | Runtime defaults, project preparation, manifest parsing, and storage paths. |
| `installer/internal/workflow` | Stage runner, recovery behavior, and workflow state. |
| `installer/internal/workflow/stages` | Bootstrap, Kubernetes, model selection, discovery, artifact, and Pulumi stages. |
| `installer/internal/ui` | Bubble Tea terminal UI. |
| `installer/internal/harbor` | Harbor auth and API helpers. |
| `installer/internal/huggingface` | Hugging Face download integration. |
| `installer/internal/oras` | OCI artifact and image upload logic. |
| `installer/internal/preloader` | SSH/containerd preload support for Harbor bootstrap images. |
| `installer/internal/iac` | Pulumi Automation API wrapper. |
| `installer/build/Dockerfile` | Container image build. |
| `installer/documentation` | Supporting developer notes and older troubleshooting references. |
| `shaide-installer-data` | Typical host-side persistent runtime storage when using the local run command above. |

## Runtime Storage

The host storage mount is the installer scratch and state root. With the local
run command above, container paths under `/var/shaide-installer` appear on the
host under `shaide-installer-data`.

| Container path | Purpose |
| --- | --- |
| `/.kube/config` | Mounted kubeconfig. |
| `/var/shaide-installer` | Persistent installer storage root. |
| `/var/shaide-installer/projects` | Pulumi projects, re-seeded from the image on every run. |
| `/var/shaide-installer/model-cache` | Hugging Face model cache. |
| `/var/shaide-installer/upload-state` | ORAS upload state for resumable uploads. |
| `/var/shaide-installer/artifact-cache` | OCI artifact cache used by model uploads. |
| `/var/shaide-installer/pulumi-state` | Local Pulumi state. |
| `/var/shaide-installer/logs` | TUI log exports. |
| `/var/shaide-installer/tmp` | Process-wide temporary files, including ORAS upload spools. |

The projects directory is removed and re-copied from the image on every run, so project
files always match the installer version. Pulumi state and stack config live outside it
and survive.

Press `ctrl+y` in the TUI to save visible installer logs to
`/var/shaide-installer/logs/`, which is `logs/` in the host storage directory. The installer
names the saved file relative to that directory, since the container cannot reliably tell the
host path of its bind mount.

## Common Developer Tasks

### Change Installer Code

1. Update Go code under `installer/cmd` or `installer/internal`.
2. Run `cd installer && go test ./...`.
3. Rebuild the installer image.
4. Rebuild the image; the projects are re-seeded from it on the next run.

### Add A Model

1. Add `app_serving/deployments/models/<generative|embedder>/<Name>/` with
   `ms-<slug>/values.yaml` and `gaie-<slug>/values.yaml`.
2. Pin the Hugging Face commit in the `ms-*` values file:

   ```yaml
   shaide:
     revision: "<full commit sha>"
   ```

3. Rebuild the installer image. The model is offered in the `select models` stage and
   published as `ai-models/<slug>:<first 12 characters of the revision>`.

Bumping `shaide.revision` changes the Harbor tag, so the next run uploads and serves the
new weights. Directories matching the variant patterns in
`installer/build/Dockerfile.dockerignore` never reach the image.

### Add A Service Image

1. Add an entry under `harbor_upload_images` in `installer/build/manifests/images.yaml`.
2. Set `source` to the registry it is fetched from, plus `project`, `name`, and `tag`.
3. Use a Harbor project the installer provisions: `ai-models`, `shaide`, `services`.
4. Rebuild the installer image.

The installer pulls the image from the named registry at install time.

### Add A Harbor Bootstrap Image

On-prem only.

1. Add an entry under `goharbor_images` with `source: archive`.
2. Stage the archive under `/opt/shaide-installer/images` in the image build, named
   `<name-with-slashes-as-dashes>-<tag>.tar`.
3. Confirm the preloader can reach the target Harbor node over SSH.
4. Rebuild the installer image.

The current preloader options in `discovery.preloadHarbor` include
environment-specific host, user, SSH key, node, containerd socket, and `ctr`
path values. Treat those as developer-local wiring until they are moved into
runtime config.

### Refresh Pulumi Deployment Assets

1. Update the Pulumi project files, charts, CRDs, or values in the repository.
2. Keep secrets out of the project files.
3. Rebuild the installer image.
4. Run a local development install/update against a disposable cluster when the
   change affects stack behavior.
