---
title: "Installer bundle"
description: "Preparing and configuring the installer bundle."
weight: 30
---

# Installer bundle

The installer bundle is a gzip-compressed tar archive containing all deployment
assets needed by the installer. Stage its contents under
`installer/installer-bundle/bundle/`, then build it with
`installer/installer-bundle/scripts/build-bundle.sh`.

## Bundle contents

The staging directory must have this root layout:

```text
bundle/
|-- checksum.json
|-- deployments/
|-- images/
`-- manifests/
    |-- images.yaml
    `-- models.yaml
```

The runtime directory is named `manifests/` (plural), not `manifest/`.

| Path | What to put there |
| --- | --- |
| `deployments/` | Pulumi projects and stack files, plus all local Helm charts, CRDs, model values, and other files referenced by those projects. Stack files must contain non-secret defaults only. |
| `manifests/images.yaml` | The `harbor_upload_images` and `goharbor_images` inventories. Each entry needs `source`, `project`, `name`, and `tag`. |
| `manifests/models.yaml` | The model inventory. Each entry needs `id`, `revision`, `harbor_project`, `harbor_name`, and `harbor_tag`; `dependencies` is optional. |
| `checksum.json` | A generated fingerprint of the other bundle files. Do not maintain it manually. The build script rewrites it and places it first in the archive, as required by the bootstrap refactor. |

For an archived image, the expected filename is derived from its manifest entry:
replace `/` in `name` with `-`, then append `-<tag>.tar`. For example:

```yaml
goharbor_images:
  - source: archive
    project: goharbor
    name: harbor-core
    tag: v2.14.2
```

must have this file:

```text
images/harbor-core-v2.14.2.tar
```

## Archive layout

The bundle is a gzip-compressed tar archive with three root-level
payload areas:

| Bundle root path | Purpose                                                                      |
| -----------------| ---------------------------------------------------------------------------- |
| `images/`        | OCI archives for Harbor's bootstrap images. On-prem installs only.           |
| `manifests/`     | Declarative inventories for images and model artifacts.                      |
| `deployments/`   | Pulumi workdirs, Helm charts, CRDs, model values, and stack config defaults. |

Application and service images are copied into Harbor from their origin registries.

Harbor's own bootstrap images are handled differently per target:

| Target | Harbor bootstrap images |
| ------ | ------------------------ |
| On-prem | Shipped as archives under `images/` and preloaded onto the Harbor node over SSH, before Harbor exists to serve them. |
| Cloud | Pulled from their origin registries by Pulumi. Nothing is staged in the bundle. |

### Mount and extraction paths
The mounted archive must be available inside the container at:
```text
/.bundle/bundle.tar.gz
```

After extraction, the archive will resolve to:

```text
/var/shaide-installer/bundle
```

Expected archive shape:

```text
.
|-- images/
|   |-- harbor-core-v2.14.0.tar
|   |-- ...
|-- manifests/
|   |-- images.yaml
|   |-- models.yaml
`-- deployments/
    |-- app-shaide/
    |   |-- Pulumi.yaml
    |   `-- Pulumi.shaide.yaml
    |-- app-serving/
    |   |-- Pulumi.yaml
    |   |-- Pulumi.serving.yaml
    |   `-- deployments/
    |       |-- llm-d-infra/
    |       `-- models/
    |           |-- embedder/
    |           |-- generative/
    |-- cloud-harbor/
    |   |-- Pulumi.yaml
    |   |-- Pulumi.harbor.yaml
    |   |-- charts/
    |       |-- harbor-1.18.2.tgz
    `-- gateway-provider/
        |-- Pulumi.yaml
        |-- Pulumi.provider.yaml
        |-- crds/
```

The installer validates these paths directly:

| Path                    | Required by                                 |
| ----------------------- | ------------------------------------------- |
| `images/`               | Harbor bootstrap image archives. On-prem installs only. |
| `deployments/`          | Pulumi workdir root.                        |
| `manifests/images.yaml`  | Image upload manifest.                      |
| `manifests/models.yaml`  | Model download/upload manifest.             |


### Images

`images/` holds the OCI archives for entries whose `source` is `archive` — in practice,
Harbor's own bootstrap images. Archive filenames are derived from the entry, not declared:
`name` with `/` replaced by `-`, then `-<tag>.tar`. An entry with
`name: harbor-core, tag: v2.14.0` resolves to `images/harbor-core-v2.14.0.tar`.

### Image Manifest

`manifests/images.yaml` supports two top-level lists:

| Key                    | Used for                                                                          |
| -----------------------| --------------------------------------------------------------------------------- |
| `harbor_upload_images` | Service and application images copied into Harbor by the artifact stage.          |
| `goharbor_images`      | Harbor bootstrap images preloaded onto the Harbor node before Harbor is deployed. |

Example:

```yaml
harbor_upload_images:
  - { source: "ghcr",      project: "shaide",   name: "axem-solutions/shaide_server", tag: "v0.11.0" }
  - { source: "dockerhub", project: "shaide",   name: "qdrant/qdrant",                tag: "v1.17" }
  - { source: "ghcr",      project: "services", name: "llm-d/llm-d-cuda",             tag: "v0.7.0" }

goharbor_images:
  - { source: "archive", project: "goharbor", name: "harbor-core",     tag: "v2.14.0" }
  - { source: "archive", project: "goharbor", name: "registry-photon", tag: "v2.14.0" }
```

Every entry uses the same schema:

| Field     | Meaning                                                          |
| ----------| ------------------------------------------------------------------ |
| `source`  | Where the image is fetched from. See the table below.             |
| `project` | Harbor project name.                                              |
| `name`    | Repository path, used both at the source and inside the project.  |
| `tag`     | Image tag.                                                        |

Accepted `source` values:

| Value          | Fetched from                            |
| -------------- | --------------------------------------- |
| `ghcr`         | GitHub Container Registry               |
| `dockerhub`    | Docker Hub                              |
| `quay`         | Quay.io                                 |
| `nvcr`         | NVIDIA NGC                              |
| `registry_k8s` | registry.k8s.io                         |
| `archive`      | An OCI archive in the bundle `images/` directory |

Only `archive` entries require a file in the bundle. All other sources are pulled from the
named registry at install time, so the provisioning machine needs network access to them.

For example, this entry:

```yaml
- { source: "ghcr", project: "shaide", name: "axem-solutions/shaide_server", tag: "v0.11.0" }
```

is pulled from GHCR and published to Harbor as:

```text
<harbor-host>/shaide/axem-solutions/shaide_server:v0.11.0
```

Harbor projects must be ones the installer provisions: `ai-models`, `shaide`, `services`.

### Model Manifest

`manifests/models.yaml` is the model artifact inventory. It must contain a non-empty top-level `models` list.

```yaml
models:
  - id: "nomic-ai/nomic-embed-text-v1.5"
    harbor_project: ai-models
    harbor_name: nomic-embed-text-v1.5
    harbor_tag: "1.5.0"
    revision: "e5cf08aadaa33385f5990def41f7a23405aec398"
    dependencies:
      - id: "nomic-ai/nomic-bert-2048"
        revision: "7710840340a098cfb869c4f65e87cf2b1b70caca"
```

Fields:

| Field             | Meaning                                                                    |
| ----------------- | ---------------------------------------------------------------------------|
| `id`              | Hugging Face model repository ID.                                          |
| `harbor_project`  | Harbor project for the model artifact.                                     |
| `harbor_name`     | Harbor repository name for the model artifact.                             |
| `harbor_tag`      | Harbor tag used to detect and publish the artifact.                        |
| `revision`        | Hugging Face revision or commit to download. Pin this for reproducibility. |
| `dependencies`    | Optional extra Hugging Face repositories downloaded with the model.        |

Model artifacts are pushed as OCI artifacts with artifact type
`application/vnd.cnai.model`.

For example, this model entry:

```yaml
- id: "nomic-ai/nomic-embed-text-v1.5"
  harbor_project: "ai-models"
  harbor_name: "nomic-embed-text-v1.5"
  harbor_tag: "1.5.0"
  revision: "e5cf08aadaa33385f5990def41f7a23405aec398"
```

is downloaded from Hugging Face and published to Harbor as:

```text
<harbor-host>/ai-models/nomic-embed-text-v1.5:1.5.0
```


### Deployments

`deployments/` contains Pulumi workdirs and deployment assets. The installer runs
these workdirs through Pulumi Automation API. Stack files in the bundle should
contain non-secret defaults only. Runtime credentials are injected by the
installer.

Expected workdirs:

| Bundle path                    | Pulumi project     | Stack      | Purpose                                                               |
| -------------------------------| ------------------ | -----------| --------------------------------------------------------------------- |
| `deployments/cloud-harbor`     | `cloud-harbor`     | `harbor`   | Deploys Harbor and uses the bundled Harbor chart.                     |
| `deployments/app-serving`      | `app-serving`      | `serving`  | Deploys model-serving infrastructure,  services, and inference values |
| `deployments/gateway-provider` | `gateway-provider` | `provider` | Deploys Gateway API and inference extension CRDs/resources.           |
| `deployments/app-shaide`       | `app-shaide`       | `shaide`   | Deploys the Shaide application stack.                                 |


Important deployment asset paths:

| Path                                                      |Meaning                                                                    |
| ----------------------------------------------------------|  ------------------------------------------------------------------------ |
| `/cloud-harbor/charts/harbor-1.18.2.tgz`                  |  Bundled Harbor Helm chart archive.                                       |
| `/app-serving/deployments/llm-d-infra`                    |  Bundled llm-d-infra chart path, relative to the app-serving workdir.     |
| `/app-serving/deployments/models/**/gaie-*/values.yaml`   | GAIE values passed into the relevant chart.                               |
| `/app-serving/deployments/models/**/ms-*/values.yaml`     | llm-d-modelservice values passed into the relevant chart.                 |
| `/gateway-provider/crds`                                  |  Gateway API and inference extension CRDs bundled for offline deployment. |
| `/app-serving/deployments/models/embedder/<model-name>`   | The folder name must match `app-serving:models.embedder[].name`.          |
| `/app-serving/deployments/models/generative/<model-name>` |  The folder name must match `app-serving:models.generative[].name`.       |

Runtime values injected by the installer include:

| Runtime value                                   | Injected into                | Source                                        |
| ----------------------------------------------- | ---------------------------  | ----------------------------------------------|
| Kubeconfig path and selected Kubernetes context | `All stacks  `               | Mounted kubeconfig and TUI context selection. |
| Harbor admin password                           |`cloud-harbor`                | Installer prompt or generated runtime config. |
| Harbor robot password and pull credentials      | `cloud-harbor`               | Harbor discovery/provisioning stage.          |
| App-serving Harbor token                        | `app-serving`                | Artifact/Harbor setup stage.                  |
| Shaide admin auth key and S3 password           | `app-shaide`                 | Installer runtime config.                     |
| GHCR token                                      | `app-shaide`                 | `GHCR_TOKEN` or TUI prompt.                   |

Keep defaults, chart paths, CRD paths, model lists, node selectors, tolerations,
and non-secret deployment values in the bundle stack files. Supply secrets
through environment variables, prompts, or Pulumi secret config generated by the
installer.

#### App-Serving Model Layout

`deployments/app-serving` resolves model values from this path:

```text
deployments/app-serving/deployments/models/<category>/<model-name>/
```

`<category>` must be one of:

| Categor       | Example model folder    | Purpose                       |
| ------------- | ----------------------- | ------------------------------|
| `embedder`    | `nomic-embed-text-v1.5` | Embedding model deployments.  |
| `generative`  | `llama-3.1-8b-instruct` | Generative model deployments. |

Each enabled entry in `Pulumi.serving.yaml` must use a `name` that exactly
matches a model folder under its category. The name is a deployment folder name,
not a Hugging Face model ID.

Example stack config:

```yaml
config:
  app-serving:models:
    embedder:
      - name: nomic-embed-text-v1.5
        enabled: true
        nodeSelector:
          nodegroup: nvidia-l4-nodepool
        modelSource:
          harborRef: harbor.harbor.svc.cluster.local/ai-models/nomic-embed-text-v1.5:1.5.0
          modelUri: hub/nomic-ai/nomic-embed-text-v1.5
          storageSize: 5Gi
```

Each model folder must contain exactly one `gaie-*` values directory and one
`ms-*` values directory:

```text
deployments/app-serving/deployments/models/embedder/nomic-embed-text-v1.5/
|-- gaie-nomic-embed-text-v1-5/
|   `-- values.yaml
`-- ms-nomic-embed-text-v1-5/
    `-- values.yaml
```


The slug after `gaie-` and the slug after `ms-` must match. The slug must use
lowercase letters, digits, and hyphens, start and end with an alphanumeric
character, and stay within the current code limit of 47 characters.

Example stack config:

```yaml
config:
  app-serving:models:
    embedder:
      - name: nomic-embed-text-v1.5
        enabled: true
        nodeSelector:
          nodegroup: nvidia-l4-nodepool
        modelSource:
          harborRef: harbor.harbor.svc.cluster.local/ai-models/nomic-embed-text-v1.5:1.5.0
          modelUri: hub/nomic-ai/nomic-embed-text-v1.5
          storageSize: 5Gi
```


## Staging the bundle

### Prerequisites

You need these tools  on the bundle builder machine:

| Requirement            | Used for                                    |
| ---------------------- | ------------------------------------------- |
| `tar`                  | Creating and verifying `bundle.tar.gz`.     |
| `skopeo` or equivalent | Creating OCI archive files under `images/`. |

### Step 1: Start from the example bundle

A complete example bundle is checked in at `installer/installer-bundle/example/`,
containing the Pulumi projects, stack files, CRDs, the inferencepool chart, and both
manifests. Copy it and adapt, rather than building a tree by hand:

```bash
cp -a installer/installer-bundle/example installer/installer-bundle/bundle
mkdir -p installer/installer-bundle/bundle/images
```

`images/` is created explicitly because the build script requires the directory to exist,
and an empty directory cannot be checked in.

### Step 2: Write The Image Manifest

List every image the deployment needs, each with the `source` it is fetched from. Keep the
manifest minimal — include only the images that path actually requires.

Destination:

```text
installer/installer-bundle/bundle/manifests/images.yaml
```

Example:

```yaml
harbor_upload_images:
  - { source: "ghcr",         project: "shaide",   name: "axem-solutions/shaide_server",       tag: "v0.11.0" }
  - { source: "ghcr",         project: "shaide",   name: "axem-solutions/control_panel",       tag: "v0.4.0" }
  - { source: "dockerhub",    project: "shaide",   name: "qdrant/qdrant",                      tag: "v1.17" }
  - { source: "ghcr",         project: "services", name: "llm-d/llm-d-cuda",                   tag: "v0.7.0" }
  - { source: "ghcr",         project: "services", name: "llm-d/llm-d-routing-sidecar",        tag: "v0.8.0" }
  - { source: "registry_k8s", project: "services", name: "gateway-api-inference-extension/epp", tag: "v1.2.0" }

# On-prem only. Leave empty for cloud — Pulumi pulls Harbor's images from their
# origin registries.
goharbor_images:
  - { source: "archive", project: "goharbor", name: "harbor-core",     tag: "v2.14.0" }
  - { source: "archive", project: "goharbor", name: "registry-photon", tag: "v2.14.0" }
```

Use only Harbor projects the installer provisions: `ai-models`, `shaide`, `services`.

### Step 3: Stage Harbor Bootstrap Archives

**On-prem installs only.** For cloud targets `goharbor_images` is empty, nothing is staged
under `images/`, and this step is skipped entirely.

Only `source: archive` entries need a file staged. Create one OCI archive per
`goharbor_images` entry.

Destination:

```text
installer/installer-bundle/bundle/images/
```

Filenames are derived from the manifest entry — `name` with `/` replaced by `-`, then
`-<tag>.tar` — and must match exactly:

```bash
skopeo copy docker://goharbor/harbor-core:v2.14.0 \
  oci-archive:installer/installer-bundle/bundle/images/harbor-core-v2.14.0.tar:goharbor/harbor-core:v2.14.0
```

Reusable template:

```bash
skopeo copy docker://<source-registry>/<source-repository>:<tag> \
  oci-archive:installer/installer-bundle/bundle/images/<name-with-slashes-as-dashes>-<tag>.tar:<source-registry>/<source-repository>:<tag>
```

Entries in `harbor_upload_images` need no staging — the installer pulls them from the
registry named by their `source`.

### Step 4: Write The Model Manifest
Create or update the staged model manifest.

Destination:

```text
installer/installer-bundle/bundle/manifests/models.yaml
```

Example content with two model artifacts:
```yaml
models:
  - id: "nomic-ai/nomic-embed-text-v1.5"
    harbor_project: ai-models
    harbor_name: nomic-embed-text-v1.5
    harbor_tag: "1.5.0"
    revision: "e5cf08aadaa33385f5990def41f7a23405aec398"        # fill in HF commit hash before first use
    dependencies:
      - id: "nomic-ai/nomic-bert-2048"
        revision: "7710840340a098cfb869c4f65e87cf2b1b70caca"    # fill in HF commit hash to pin

  - id: "openai/gpt-oss-20b"
    harbor_project: ai-models
    harbor_name: gpt-oss-20b
    harbor_tag: "1.0.0"
    revision: "6cee5e81ee83917806bbde320786a8fb61efebee"        # fill in HF commit hash before first use
```

### Step 5: Stage Deployment Workdirs
Each Pulumi stack must be present under `deployments/` with the expected project
file and stack file.

#### Create the stack files
Required workdirs and stack files:

| Workdir                        | Required files                        |
| -------------------------------| ------------------------------------- |
| `deployments/cloud-harbor`     | `Pulumi.yaml`, `Pulumi.harbor.yaml`   |
| `deployments/app-serving`      | `Pulumi.yaml`, `Pulumi.serving.yaml`  |
| `deployments/gateway-provider` | `Pulumi.yaml`, `Pulumi.provider.yaml` |
| `deployments/app-shaide`       | `Pulumi.yaml`, `Pulumi.shaide.yaml`   |

Copy maintained stack files into the staged workdirs.

Start from an existing, known-good stack file for each project, then change these values from the staged stack files if present before packaging:

| Bundle stack file      | Delete value                                                                                                          |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `Pulumi.serving.yaml`  | `app-serving:kubeconfig`, `app-serving:harborToken`, `encryptionsalt`, `app-serving:orasImage`                        |
| `Pulumi.shaide.yaml`   | `app-shaide:kubeconfig`, `app-shaide:ghcrToken`, `app-shaide:adminAuthKey`, `app-shaide:s3Password`, `encryptionsalt` |
| `Pulumi.provider.yaml` | `gateway-provider:kubeconfig`, `gateway-provider:istioHub` `encryptionsalt`                                           |
| `Pulumi.harbor.yaml`   | `cloud-harbor:kubeconfig`, `cloud-harbor:context`, `harbor:adminPassword`, `harbor:robotPassword`, `encryptionsalt`   |


| Bundle stack file      | Change value                                                           |
| ---------------------- | -----------------------------------------------------------------------|
| `Pulumi.serving.yaml`  | `app-serving:llmdChart: deployments/llm-d-infra` , `nodeSelector`     |
| `Pulumi.shaide.yaml`   | `-`                                                                    |
| `Pulumi.provider.yaml` |  `gateway-provider:gatewayApiCrdsPath: ./crds/gateway-api/standard`    |
| `Pulumi.harbor.yaml`   | ``-`                                                                   |

For example `nodeSelector: { nodegroup: nvidia-l4-nodepool }`.
These are cluster-specific and must match the target cluster node labels.

#### Download the external resources to its destinations

Download the Harbor and llm-d-infra  Helm chart into the cloud-harbor and the app-serving workdir, , then copy the Gateway Provider CRDs into the
gateway-provider workdir:

```bash
helm pull harbor \
  --repo https://helm.goharbor.io \
  --version 1.18.2 \
  --destination installer/installer-bundle/bundle/deployments/cloud-harbor/charts

  helm pull llm-d-infra \
  --repo https://llm-d-incubation.github.io/llm-d-infra/ \
  --version v1.3.4 \
  --untar \
  --untardir installer/installer-bundle/bundle/deployments/app-serving/deployments

cp -a infra/gateway-provider/crds \
  installer/installer-bundle/bundle/deployments/gateway-provider/
```

#### Stage Model Deployment Files

Copy the model deployment files for every enabled model into the app-serving bundle workdir:

```text
installer/installer-bundle/bundle/deployments/app-serving/deployments/models/<category>/<model-name>/
```
For the `cloud bundle`, use the Harbor-backed cloud model folders:

| Model type | Model name                | Source folder                                                   |
| ---------- | -------------------------- | ---------------------------------------------------------------- |
| Generative | `GPT-OSS-20B`              | `app_serving/deployments/models/generative/GPT-OSS-20B`          |
| Embedder   | `nomic-embed-text-v1.5`    | `app_serving/deployments/models/embedder/nomic-embed-text-v1.5`  |

```bash
mkdir -p installer/installer-bundle/bundle/deployments/app-serving/deployments/models/generative
mkdir -p installer/installer-bundle/bundle/deployments/app-serving/deployments/models/embedder

cp -a app_serving/deployments/models/generative/GPT-OSS-20B \
  installer/installer-bundle/bundle/deployments/app-serving/deployments/models/generative/

cp -a app_serving/deployments/models/embedder/nomic-embed-text-v1.5 \
  installer/installer-bundle/bundle/deployments/app-serving/deployments/models/embedder/
```

Also make sure `Pulumi.serving.yaml` enables the same model names that were
copied into the bundle:

```yaml
config:
  app-serving:models:
    generative:
      - name: GPT-OSS-20B
        enabled: true
        nodeSelector:
          nodegroup: <cluster specific>
        modelSource:
          harborRef: harbor.harbor.svc.cluster.local/ai-models/gpt-oss-20b:1.0.0
          modelUri: hub/openai/gpt-oss-20b
          storageSize: 50Gi
    embedder:
      - name: nomic-embed-text-v1.5
        enabled: true
        nodeSelector:
          nodegroup: <cluster specific>
        modelSource:
          harborRef: harbor.harbor.svc.cluster.local/ai-models/nomic-embed-text-v1.5:1.5.0
          modelUri: hub/nomic-ai/nomic-embed-text-v1.5
          storageSize: 5Gi
```

## Build

From the repository root:

```bash
installer/installer-bundle/scripts/build-bundle.sh
```

Defaults:

```text
staging directory: installer/installer-bundle/bundle
output archive:    installer/installer-bundle/bundle.tar.gz
```

Both can be overridden:

```bash
installer/installer-bundle/scripts/build-bundle.sh <staging-directory> <output-archive>
```

To build the checked-in example directly:

```bash
installer/installer-bundle/scripts/build-bundle.sh \
  installer/installer-bundle/example \
  installer/installer-bundle/example.tar.gz
```

The script:

1. verifies `deployments/`, `images/` and `manifests/` exist in the staging directory;
2. verifies `manifests/images.yaml` and `manifests/models.yaml` are present;
3. rejects symlinks, which a bundle cannot carry;
4. hashes every payload file, sorted and relative to the bundle root, and writes
   `checksum.json`;
5. creates the archive with `checksum.json` as its first entry;
6. verifies that ordering before replacing the output archive.

It requires Bash, GNU `tar`, `find`, `sort`, `xargs`, `sha256sum`, `awk`, and `mktemp`.

## Verify

```bash
tar -tzf installer/installer-bundle/bundle.tar.gz
tar -xOzf installer/installer-bundle/bundle.tar.gz checksum.json
```

The first command must print `checksum.json` first, followed by the three payload
directories. Rebuilding unchanged content reproduces the same digest; changing any payload
file changes it, which is how the installer decides to re-extract.