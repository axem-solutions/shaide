# AI Platform Installer

The AI Platform Installer is a containerized terminal UI for installing or updating the AI Platform 
on a Kubernetes cluster. 

It is designed for on-prem and cloud deployments. The installer image contains the application code, and the installation payload is supplied separately as a mounted bundle archive.


## Bundle Architecture

The offline bundle is a gzip-compressed tar archive with three root-level
payload areas:

| Bundle root path | Purpose                                                                      |
| -----------------| ---------------------------------------------------------------------------- |
| `images/`        | OCI image archives used for Harbor and the  AI Platform                      |
| `manifests/`     | Declarative inventories for images and model artifacts.                      |
| `deployments/`   | Pulumi workdirs, Helm charts, CRDs, model values, and stack config defaults. |


The bundle is self-contained. The installer should be able to extract the bundle, read the manifests and deploy the Pulumi projects without reaching for external resources.

### Bundle Contract
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
|   |-- shaide_server-0.5.0.tar
|   |-- ...
|-- manifests/
|   |-- images.yml
|   |-- models.yml
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
| `images/`               | Image and Harbor bootstrap image manifests. |
| `deployments/`          | Pulumi workdir root.                        |
| `manifests/images.yml`  | Image upload manifest.                      |
| `manifests/models.yml`  | Model download/upload manifest.             |      


### Images
`images/` contains the image archive files referenced by `manifests/images.yml`.
Every active `file` value in either `harbor_upload_images` or `goharbor_images`
must have a matching file under `images/`.
### Image Manifest

It supports two top-level lists:

| Key                    | Used for                                                                          |
| -----------------------| --------------------------------------------------------------------------------- | 
| `harbor_upload_images` | Service and application images uploaded into Harbor by the artifact stage.        |
| `goharbor_images`      | Harbor bootstrap images preloaded onto the Harbor node before Harbor is deployed. |
Example:

```yaml
harbor_upload_images:
  - file: "shaide_server-0.5.0.tar" project: "shaide" name: "shaide_server" tag: "0.5.0"
  - file: "control_panel-0.1.0.tar" project: "shaide" name: "control_panel" tag: "0.1.0"

goharbor_images:
  - file: "harbor-core-v2.14.0.tar"     project: "goharbor" name: "harbor-core"     tag: "v2.14.0"
  - file: "harbor-registry-v2.14.0.tar" project: "goharbor" name: "registry-photon" tag: "v2.14.0"
```

Each entry has the same schema:
The `file` field binds the manifest entry to the physical archive in the bundle.
The `project`, `name`, and `tag` fields define the destination reference in
Harbor.

| Field     | Meaning                                                                                      |
| ----------| -------------------------------------------------------------------------------------------- |
| `file`    | Filename relative to the bundle `images/` directory. The file must exist at `images/<file>`. |
| `project` | Harbor project name.                                                                         |
| `name`    | Harbor repository name inside the project.                                                   |
| `tag`     | Harbor image tag.                                                                            |

For example, this entry:

```yaml
- file: "shaide_server-0.5.0.tar" project: "shaide" name: "shaide_server" tag: "0.5.0"
```

maps this local archive:

```text
images/shaide_server-0.5.0.tar
```

to this Harbor repository reference:

```text
<harbor-host>/shaide/shaide_server:0.5.0
```

### Model Manifest

`manifests/models.yml` is the model artifact inventory. It must contain a non-empty top-level `models` list.

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


## Bundle Assembly 

### Prerequisites

You need these tools  on the bundle builder machine:

| Requirement            | Used for                                    |
| ---------------------- | ------------------------------------------- |
| `tar`                  | Creating and verifying `bundle.tar.gz`.     |
| `skopeo` or equivalent | Creating OCI archive files under `images/`. |

### Step 1: Create The Staging Directory Structure
Start from a clean staging tree when rebuilding the bundle. This prevents stale
resources.

After success the staged bundle must contain these directories:

```text
installer/installer-bundle/bundle/
|-- images/
|-- manifests/
`-- deployments/
    |-- cloud-harbor/
    |   `-- charts/
    |-- app-serving/
    |   `-- deployments/
    |       |-- llm-d-infra/
    |       `-- models/
    |           |-- embedder/
    |           `-- generative/
    |-- gateway-provider/
    |   `-- crds/
    `-- app-shaide/
```

```bash
# Start from a clean state
rm -rf installer/installer-bundle/bundle

# Create folder for images and manifests.
mkdir -p installer/installer-bundle/bundle/images
mkdir -p installer/installer-bundle/bundle/manifests

# Create the Pulumi  deployment subfolders expected by the bundle.
mkdir -p installer/installer-bundle/bundle/deployments/cloud-harbor/charts
mkdir -p installer/installer-bundle/bundle/deployments/app-serving/deployments/llm-d-infra
mkdir -p installer/installer-bundle/bundle/deployments/app-serving/deployments/models/embedder
mkdir -p installer/installer-bundle/bundle/deployments/app-serving/deployments/models/generative
mkdir -p installer/installer-bundle/bundle/deployments/gateway-provider/crds
mkdir -p installer/installer-bundle/bundle/deployments/app-shaide
```

### Step 2: Write The Image Manifest
The staged image manifest depends on the deployment type. Do not always copy the
full on-prem inventory into the bundle. For cloud deployment, keep the manifest
minimal and include only the images required by that path.

Destination:

```text
installer/installer-bundle/bundle/manifests/images.yml
```

| Scenario           | Image source                                                                                              |
| ------------------ | --------------------------------------------------------------------------------------------------------  | 
| Cloud deployment   | [ Cloud deployment image set](#Cloud-deployment-image-set)                                                |
| On-prem deployment | [`infra/on-prem/ansible/group_vars/all/images.yml`](../infra/on-prem/ansible/group_vars/all/images.yml)   | 

### Cloud deployment image set

| Image                                  | Staged archive filename                    | Harbor project | Harbor repository                | Harbor tag |
| ---------------------------------------| -------------------------------------------| --------------| ---------------------------------| --- |
| `llm-d-inference-sim:v0.7.1`           | `llm-d-inference-sim-v0.7.1.tar`           | `image-shaide`| `llm-d/llm-d-inference-sim`      | `v0.7.1` |
| `llm-d-inference-scheduler:v0.4.0-rc.1`| `llm-d-inference-scheduler-v0.4.0-rc.1.tar`| `image-shaide`| `llm-d/llm-d-inference-scheduler`| `v0.4.0-rc.1` |


Content:

```yaml
harbor_upload_images:
  - file: "llm-d-inference-sim-v0.7.1.tar"            project: "image-shaide" name: "llm-d/llm-d-inference-sim"       tag: "v0.7.1"
  - file: "llm-d-inference-scheduler-v0.4.0-rc.1.tar" project: "image-shaide" name: "llm-d/llm-d-inference-scheduler" tag: "v0.4.0-rc.1"

goharbor_images: []
```
 

### Step 3: Copy Image Archives

For the cloud example, create only the two required OCI archives.

Destination:

```text
installer/installer-bundle/bundle/images/
```

Commands:

```bash
skopeo copy docker://ghcr.io/llm-d/llm-d-inference-sim:v0.7.1 \
  oci-archive:installer/installer-bundle/bundle/images/llm-d-inference-sim-v0.7.1.tar:ghcr.io/llm-d/llm-d-inference-sim:v0.7.1

skopeo copy docker://ghcr.io/llm-d/llm-d-inference-scheduler:v0.4.0-rc.1 \
  oci-archive:installer/installer-bundle/bundle/images/llm-d-inference-scheduler-v0.4.0-rc.1.tar:ghcr.io/llm-d/llm-d-inference-scheduler:v0.4.0-rc.1
```

Reusable command template:

```bash
skopeo copy docker://<source-registry>/<source-repository>:<source-tag> \
  oci-archive:installer/installer-bundle/bundle/images/<archive-file>.tar:<source-registry>/<source-repository>:<source-tag>
```

The `<archive-file>.tar` value must match the corresponding `file` value in `manifests/images.yml` exactly.

### Step 4: Write The Model Manifest
Create or update the staged model manifest.

Destination:

```text
installer/installer-bundle/bundle/manifests/models.yml
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

## Step 6: Build The Bundle Archive

After staging `images/`, `manifests/`, and `deployments/`, build the final
bundle archive from inside the staging directory. Do not include the `bundle/`
directory itself in the archive.

```bash
tar -C installer/installer-bundle/model-onboarding \
  -czf installer/installer-bundle/model.tar.gz \
  images manifests deployments
```
Verify the archive shape:

```bash
tar -tzf installer/installer-bundle/bundle.tar.gz
```

The archive should list these root entries:
```text
images/
manifests/
deployments/
```
## Build And Run

Run the Docker build from the repository root:

```bash
docker build -f installer/build/Dockerfile -t onprem-installer:latest .
```

Use this when changing installer code. It does not refresh the offline bundle;
changing images, manifests, Pulumi stack files, charts, CRDs, or deployment
values requires rebuilding the bundle archive separately.

The installer container expects:

| Input | Required | Container path or env | Purpose |
| --- | --- | --- | --- |
| Kubeconfig | Yes | `/.kube/config` by default | Kubernetes config used for context selection and cluster access. |
| Bundle archive | Yes | `/.bundle/bundle.tar.gz` by default | Offline installer payload extracted during bootstrap. |
| Persistent storage | Strongly recommended | `/var/shaide-installer` | Bundle extraction, model cache, upload state, temporary files, Pulumi state, and logs. |
| Hugging Face token | Yes | `HF_TOKEN` | Downloads selected model snapshots from Hugging Face. |
| GHCR token | Optional | `GHCR_TOKEN` | Used by the `app-shaide` stack; when empty, the TUI prompts for it. |
| SSH private key path | Yes for Harbor install/preload | `PRIVATE_KEY_PATH` | Path inside the container to the SSH private key used for Harbor image preloading. |
| Gcloud config | Only for GKE auth | `/root/.config/gcloud` | Lets kubeconfigs that use the GKE auth plugin work inside the container. |

The installer also supports these path override environment variables:

| Environment variable | Default | Purpose |
| --- | --- | --- |
| `KUBECONFIG` | `/.kube/config` | Kubeconfig path inside the container. |
| `BUNDLE_ARCHIVE_PATH` | `/.bundle/bundle.tar.gz` | Bundle archive path inside the container. |
| `PRIVATE_KEY_PATH` | none | SSH private key path inside the container for Harbor image preloading. |

The shell variables below are host-side helpers for the `docker run` command;
the installer reads the mounted paths and environment variables inside the
container.

```bash
HOST_KUBECONFIG="$HOME/.kube/config"
HOST_GCLOUD_CONFIG="$HOME/.config/gcloud"
HOST_SSH_DIR="$HOME/.ssh"
BUNDLE_ARCHIVE="$PWD/installer/installer-bundle/bundle.tar.gz"
STORAGE_PATH="$PWD/shaide-installer-data"
DST_BOUND_MOUNT="/var/shaide-installer"
HF_TOKEN="<token>"
GHCR_TOKEN="<token>" 
PRIVATE_KEY_PATH="/root/.ssh/google_compute_engine"
```

If the target Kubernetes context depends on GKE auth, log in on the host before
running the container:

```bash
gcloud auth login
gcloud auth application-default login
```

Run the installer as an interactive container:

```bash
mkdir -p "${STORAGE_PATH}"

docker run --rm -it \
  --network host \
  -e CLOUDSDK_CONFIG="/root/.config/gcloud" \
  -e HF_TOKEN="${HF_TOKEN}" \
  -e GHCR_TOKEN="${GHCR_TOKEN}" \
  -e PRIVATE_KEY_PATH="${PRIVATE_KEY_PATH}" \
  -v "${HOST_KUBECONFIG}:/.kube/config:ro" \
  -v "${HOST_GCLOUD_CONFIG}:/root/.config/gcloud" \
  -v "${HOST_SSH_DIR}:/root/.ssh:ro" \
  -v "${BUNDLE_ARCHIVE}:/.bundle/bundle.tar.gz:ro" \
  --mount "type=bind,src=${STORAGE_PATH},dst=${DST_BOUND_MOUNT}" \
  onprem-installer:latest
```

The `-it` flags are required because the installer is a TUI. `--network host`
lets the container use local Kubernetes and Harbor port-forwarding behavior
without extra Docker port mapping. The gcloud mount can be omitted for clusters
that do not use GKE authentication. Keep the gcloud config mount writable for
GKE clusters because `gke-gcloud-auth-plugin` may refresh cached credentials
under `/root/.config/gcloud`.

During a fresh Harbor install, the TUI prompts for the Harbor node IP or
hostname, SSH user, remote `ctr` path, and remote containerd socket used by the
image preloader.

## Architecture

The installer has two separate moving parts:

- the installer container image, built from `installer/build/Dockerfile`;
- the offline bundle archive, mounted at runtime as `/.bundle/bundle.tar.gz`.

At runtime the installer:

1. verifies it is running in an interactive terminal;
2. verifies and prepares persistent installer storage;
3. extracts the mounted bundle into `/var/shaide-installer/bundle`;
4. reads `manifests/images.yml` and `manifests/models.yml`;
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
refreshing the offline bundle.

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
| `/.bundle/bundle.tar.gz` | Mounted offline bundle archive. |
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

1. Copy the image into `installer/installer-bundle/fresh/images/`.
2. Add a matching entry under `harbor_upload_images` in `manifests/images.yml`.
3. Ensure `file` exactly matches the archive filename.
4. Rebuild `installer/installer-bundle/bundle.tar.gz`.
5. Verify with `tar -tzf installer/installer-bundle/bundle.tar.gz`.

### Add A Harbor Bootstrap Image

1. Copy the image into `installer/installer-bundle/fresh/images/`.
2. Add a matching entry under `goharbor_images`.
3. Confirm the preloader can reach the target Harbor node over SSH.
4. Rebuild and verify the bundle.

The current preloader options in `discovery.preloadHarbor` include
environment-specific host, user, SSH key, node, containerd socket, and `ctr`
path values. Treat those as developer-local wiring until they are moved into
runtime config.

### Add A Model Artifact

1. Add the model to `manifests/models.yml`.
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

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `/.bundle/bundle.tar.gz does not exist` | The bundle archive was not mounted, or `BUNDLE_ARCHIVE_PATH` points to the wrong path. |
| `/var/shaide-installer is not a mount point` | The host storage bind mount is missing. The TUI may let you continue, but cache/state/logs will not persist. |
| `Hugging Face token was not set` | `HF_TOKEN` is required during bootstrap. |
| `image <file> listed in manifest but not found` | `manifests/images.yml` references an archive missing from `images/`. |
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
