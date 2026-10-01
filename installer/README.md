# shaide installer

The shaide installer is a containerized terminal UI that installs or updates the shaide AI
Platform on an existing Kubernetes cluster, cloud or on-prem.

Everything it deploys ships inside the installer image: the Pulumi projects, Helm charts,
CRDs, the image manifest and the supported models. Nothing is mounted besides the
kubeconfig and a storage directory.

| Document | Covers |
| --- | --- |
| [Installer guide](../docs/installation/installer-guide.md) | Running the installer: inputs, prompts and stages |
| [Installer architecture](../docs/architecture/installer.md) | Runtime workflow, storage layout, source layout and developer tasks |

## Build

Build from the repository root, since the image also packages the Pulumi projects and
models from outside `installer/`:

```bash
docker build -f installer/build/Dockerfile -t installer:local .
```

Rebuild after changing installer code, Pulumi projects, charts, CRDs, the image manifest,
the supported models or deployment values.

## Run

```bash
STORAGE_PATH="$PWD/shaide-installer-data"
mkdir -p "${STORAGE_PATH}"

docker run --rm -it \
  --network host \
  -e HF_TOKEN \
  -e PULUMI_CONFIG_PASSPHRASE \
  -v "$HOME/.kube/config:/.kube/config:ro" \
  --mount "type=bind,src=${STORAGE_PATH},dst=/var/shaide-installer" \
  installer:local
```

- `-it` is required: the installer is a TUI.
- `--network host` lets it reach the API server and its Harbor port-forward without port
  mapping.
- Reuse the same storage directory and `PULUMI_CONFIG_PASSPHRASE` on every run against the
  same cluster; the Pulumi state lives there.

For the full list of environment variables (registry credentials, `PRIVATE_KEY_PATH` for
the on-prem Harbor preload, transfer limits), see the
[installer guide](../docs/installation/installer-guide.md#all-environment-variables).

## Image contents

| Path in the image | Source in the repository | Purpose |
| --- | --- | --- |
| `/bin/installer` | `installer/cmd/installer` | The installer binary |
| `/opt/shaide-installer/projects/<project>/` | `*/deployments/Pulumi.yaml`, charts, CRDs | Pulumi projects, copied to `/var/shaide-installer/projects` on every run |
| `/opt/shaide-installer/projects/app-serving/deployments/models/` | `app_serving/deployments/models/` | The supported models offered in the model selector |
| `/opt/shaide-installer/manifests/images.yaml` | `installer/build/manifests/images.yaml` | Container images copied into Harbor |

Per-cluster model variants and simulators are kept out of the image by
`installer/build/Dockerfile.dockerignore`.

## Develop

```bash
cd installer
go test ./...
```

| Path | Purpose |
| --- | --- |
| `cmd/installer` | Entrypoint |
| `internal/config` | Runtime defaults, storage paths, project staging, and the image and model catalog |
| `internal/workflow` | Stage runner, recovery behavior and workflow state |
| `internal/workflow/stages` | `bootstrap`, `kubernetes`, `models`, `nodes`, `discovery`, `artifact` and `pulumi` stages |
| `internal/placement` | Node pools and where workloads run |
| `internal/ui` | Bubble Tea terminal UI |
| `internal/kube` | Kubernetes helpers |
| `internal/harbor` | Harbor auth and API helpers |
| `internal/huggingface` | Hugging Face downloads |
| `internal/oras` | OCI artifact and image uploads |
| `internal/preloader` | SSH/containerd preload of Harbor's own images (on-prem) |
| `internal/iac` | Pulumi Automation API wrapper |
| `build/` | Dockerfile, its ignore file, and the image manifest |

How to add a model, a service image or a Harbor bootstrap image is described under
[Common developer tasks](../docs/architecture/installer.md#common-developer-tasks).
