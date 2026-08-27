---
title: "Stack configuration"
description: "Pulumi stack configuration keys used during installation."
weight: 40
---

# Stack configuration

This document lists every Pulumi stack file used in an on-prem air-gapped installation,
the config keys each file exposes, and what the operator must set before running
`pulumi up`.

**Legend**

| Mark | Meaning |
|---|---|
| **Required** | Must be set; the stack will error if absent |
| Optional | Has a documented fallback or default; override only when the default does not fit |
| **Secret** | Must be set via `pulumi config set --secret`; never commit in plain text |

---

## 1. Cluster infrastructure

Stack: `infra/on-prem/pulumi/infra/Pulumi.<stack-name>.yaml`

Deploys the `hostpath` StorageClass that all subsequent stacks depend on.
Run this once before any other stack.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `k8s-onprem-airgap-infra:components` | **Required** | `[storageclass]` | List of components to deploy. Only valid value for on-prem is `storageclass`. |
| `k8s-onprem-airgap-infra:kubeconfig` | Optional | `~/.kube/rke2-cluster.yaml` | Path to the kubeconfig on the provisioner laptop. Falls back to `KUBECONFIG` env var or `~/.kube/config` when omitted. |

---

## 2. Platform services

Stack: `infra/on-prem/pulumi/services/Pulumi.<stack-name>.yaml`

Deploys Harbor, MetalLB, and the GPU Operator in two phases.

**Phase 1 — Harbor only** (set `components: [harbor]`, then `pulumi up`)
**Phase 2 — add MetalLB and GPU Operator** (set `components: [harbor, metallb, gpu-operator]`, then `pulumi up`)

Do not enable `metallb` or `gpu-operator` until Harbor is running and images have been uploaded to it.

### Base keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `k8s-onprem-airgap-services:components` | **Required** | `[harbor]` → `[harbor, metallb, gpu-operator]` | List of components to enable. Start with `[harbor]` alone; add the remaining two after Harbor is running and images are uploaded to it. |
| `k8s-onprem-airgap-services:kubeconfig` | Optional | `~/.kube/rke2-cluster.yaml` | Path to the kubeconfig on the provisioner laptop. Falls back to `KUBECONFIG` env var or `~/.kube/config` when omitted. |

### Harbor keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `harbor:hostname` | **Required** | `harbor.harbor.svc.cluster.local` | Internal hostname for Harbor. Must match `harbor_hostname` in `ansible/group_vars/all/harbor.yml` and the `/etc/hosts` entry written by `harbor_setup.yml`. |
| `harbor:projectName` | **Required** | `images-infra` | Harbor project where infrastructure images (MetalLB, GPU Operator) are stored. Must match project names in `ansible/group_vars/all/images.yaml`. |
| `harbor:nodeHostname` | **Required** | `<node-hostname>` | Kubernetes node name where Harbor pods are scheduled. Used to pin Harbor PersistentVolumes via `nodeAffinity`. Must match `harbor_node_hostname` in `harbor.yml`. |
| `harbor:chartPath` | Optional | `./charts/harbor-1.18.2.tgz` | Path to the Harbor Helm chart tarball relative to `pulumi/services/`. Override only when using a different chart version. |
| `harbor:adminPassword` | **Secret** | — | Harbor administrator password. Set before the first `pulumi up`: `pulumi config set --secret harbor:adminPassword <password>`. Must match `vault_harbor_admin_password` in `ansible/group_vars/all/vault.yml`. |
| `harbor:robotPassword` | **Secret** | — | Harbor robot account password. Set after `ansible-playbook harbor_setup.yml` runs: `pulumi config set --secret harbor:robotPassword $(cat ../ansible/artifacts/harbor-robot-secret)`. When absent, Pulumi skips the `imagePullSecret` for MetalLB and GPU Operator — re-run `pulumi up` after registering it. |

### MetalLB keys

Required in Phase 2 when `metallb` is in the components list.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `metallb:ipPool` | **Required** | `<range-start>-<range-end>` (e.g. `10.0.10.200-10.0.10.220`) | IP address range allocated to MetalLB for LoadBalancer services. Must not overlap existing subnet assignments in the customer network. |
| `metallb:controllerNodeHostname` | **Required** | `<node-hostname>` | Kubernetes node name where the MetalLB controller pod is pinned. |
| `metallb:chartPath` | Optional | `./charts/metallb-0.15.3.tgz` | Path to the MetalLB Helm chart tarball relative to `pulumi/services/`. |

### GPU Operator keys

Required in Phase 2 when `gpu-operator` is in the components list.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `gpu-operator:gpuNodeHostname` | **Required** | `<node-hostname>` | Kubernetes node name of the GPU worker. Used to pin the GPU Operator controller pod. |
| `gpu-operator:chartPath` | Optional | `./charts/gpu-operator-v25.10.1.tgz` | Path to the GPU Operator Helm chart tarball relative to `pulumi/services/`. |

---

## 3. Gateway provider

Stack: `infra/gateway-provider/Pulumi.<stack-name>.yaml`

Deploys the Istio control plane, Gateway API CRDs, and the cluster-level Gateway resource.
Run after MetalLB is operational (MetalLB assigns the external IP to the Istio ingress gateway).

Populate CRD files before running `pulumi up` — see `infra/gateway-provider/crds/README.md`.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `gateway-provider:cloudProvider` | **Required** | `on-prem` | Identifies the target platform. Must be `on-prem` for this installation. |
| `gateway-provider:kubeconfig` | Optional | `~/.kube/rke2-cluster.yaml` | Path to the kubeconfig on the provisioner laptop. Falls back to `KUBECONFIG` env var or `~/.kube/config` when omitted. |
| `gateway-provider:gatewayClassName` | **Required** | `istio` | Gateway API class name. Use `istio` for RKE2 on-prem; MetalLB provides the LoadBalancer IP. Cloud deployments use `gke-l7-regional-external-managed`. |
| `gateway-provider:istioHub` | **Required** | `harbor.harbor.svc.cluster.local/images-shaide` | Image registry prefix for Istio component images (pilot, proxyv2). Must point to the Harbor project where Istio images were uploaded via `harbor_upload.yml`. |
| `gateway-provider:gatewayApiCrdsPath` | **Required** | `./crds/gateway-api/standard` | Local path to Gateway API CRD manifests relative to the Pulumi project directory. |
| `gateway-provider:gieCrdsPath` | **Required** | `./crds/gie` | Local path to GIE CRD manifests relative to the Pulumi project directory. |
| `gateway-provider:gatewayHostname` | **Required** | `shaide.example.com` | Public hostname for the shared Gateway. Used in the Gateway resource's listener and referenced by HTTPRoutes in the application stacks. |
| `gateway-provider:tlsCert` | **Secret** | — | PEM-encoded TLS certificate. Set via: `pulumi config set --secret gateway-provider:tlsCert`. |
| `gateway-provider:tlsKey` | **Secret** | — | PEM-encoded TLS private key. Set via: `pulumi config set --secret gateway-provider:tlsKey`. |
| `gateway-provider:tlsCertAnnotation` | Optional | _(not set)_ | Cloud-managed TLS certificate annotation. Omitted for on-prem; only required on GCP/GKE. |

---

## 4. Shaide application

Stack: `app_shaide/deployments/Pulumi.<stack-name>.yaml`

Deploys the Shaide server, control panel UI, RustFS object storage, and Qdrant vector DB.

### Infrastructure binding keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app_shaide:cloudProvider` | **Required** | `on-prem` | Target platform identifier. Must be `on-prem`. |
| `app_shaide:kubeconfig` | Optional | `~/.kube/rke2-cluster.yaml` | Path to the kubeconfig on the provisioner laptop. Falls back to `KUBECONFIG` env var or `~/.kube/config` when omitted. |
| `app_shaide:namespace` | Optional | `app-shaide` | Kubernetes namespace where all Shaide resources are created. |
| `app_shaide:shaideServiceAccountName` | **Required** | `shaide-server` | Kubernetes ServiceAccount name for the shaide-server pod. |
| `app_shaide:gatewayHostname` | **Required** | `shaide.example.com` | Hostname used in the HTTPRoute to the shared Gateway. Switches shaide-server from `LoadBalancer` to `ClusterIP` mode. Must match `gateway-provider:gatewayHostname`. |
| `app_shaide:lbAnnotations` | Optional | `metallb.universe.tf/address-pool: default-pool` | MetalLB address pool annotation on the LoadBalancer Service. Must match the `IPAddressPool` name created by the services stack. Ignored when `gatewayHostname` is set. |

### Node selector keys

Controls which Kubernetes node each component is scheduled on.
Values must match the node labels applied during RKE2 provisioning.
Omit all node selector keys to let Kubernetes schedule freely.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app_shaide:nodeSelectorKey` | Optional | `app` | Label key used for all node selector matches. |
| `app_shaide:nodeSelectorShaide` | Optional | `shaide` | Label value for the `shaide-server` pod. Falls back to `nodeSelector` global value when omitted. |
| `app_shaide:nodeSelectorControlPanel` | Optional | `shaide` | Label value for the `control-panel` UI pod. |
| `app_shaide:nodeSelectorWebapp` | Optional | `shaide` | Label value for the `webapp` pod. |
| `app_shaide:nodeSelectorRustfs` | Optional | `shaide` | Label value for the RustFS pod. |
| `app_shaide:nodeSelectorQdrant` | Optional | `shaide` | Label value for the Qdrant pod. |
| `app_shaide:nodeSelector` | Optional | _(not set)_ | Global fallback label value applied to any component without a per-component key. Omit to let Kubernetes schedule freely. |

### Storage keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app_shaide:storageClassName` | **Required** | `hostpath` | StorageClass name. Must match the class created by the `k8s-onprem-airgap-infra` stack. |
| `app_shaide:pvNodeHostname` | **Required** | `<node-hostname>` | Kubernetes node name where hostPath PV directories exist. PVs are pinned to this node via `nodeAffinity`. Must match `gpu-operator:gpuNodeHostname` in the services stack. |
| `app_shaide:shaidePVSize` | Optional | `5Gi` | Storage capacity for the shaide-server SQLite database PV. |
| `app_shaide:rustfsPVSize` | Optional | `5Gi` | Storage capacity for the RustFS object storage PV. |
| `app_shaide:qdrantPVSize` | Optional | `5Gi` | Storage capacity for the Qdrant vector storage PV. |

### Harbor registry keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app_shaide:harborHostname` | **Required** | `harbor.harbor.svc.cluster.local` | Internal Harbor hostname used to construct the `imagePullSecret`. Must match `harbor:hostname` in the services stack. |
| `app_shaide:ghcrUser` | **Required** | `robot$k8s-harbor-sa` | Harbor robot account username for pulling images. |
| `app_shaide:ghcrToken` | **Secret** | — | Harbor robot account password. Set after `harbor_setup.yml` runs: `pulumi config set --secret app_shaide:ghcrToken $(cat infra/on-prem/ansible/artifacts/harbor-robot-secret)`. |

### Container image keys

All images must be uploaded to Harbor before `pulumi up`.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app_shaide:shaideServerImage` | **Required** | `harbor.../images-shaide/shaide_server:v0.7.0` | Shaide server image reference (full Harbor path). |
| `app_shaide:controlPanelImage` | **Required** | `harbor.../images-shaide/control_panel:v0.3.0` | Control panel UI image reference. |
| `app_shaide:webappImage` | **Required** | `harbor.../images-shaide/webapp:v0.1.0` | Web app UI image reference. |
| `app_shaide:rustfsImage` | **Required** | `harbor.../images-shaide/rustfs/rustfs:1.0.0-alpha.92` | RustFS image reference. |
| `app_shaide:qdrantImage` | **Required** | `harbor.../images-shaide/qdrant/qdrant:v1.17` | Qdrant vector DB image reference. |
| `app_shaide:busyboxImage` | **Required** | `harbor.../images-infra/busybox:1.37` | Busybox init-container image reference. |

### Application settings keys

Injected as environment variables via ConfigMap into the shaide-server pod.

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app_shaide:shaideServerUiFqdn` | **Required** | `control-panel` | In-cluster DNS name of the control-panel service. |
| `app_shaide:shaideServerUiPort` | **Required** | `3000` | Port for the control-panel service. |
| `app_shaide:controlPanelBasePath` | **Required** | `/ui` | Base path for the control-panel UI. |
| `app_shaide:shaideServerS3Fqdn` | **Required** | `rustfs` | In-cluster DNS name of the RustFS service. |
| `app_shaide:shaideServerS3Port` | **Required** | `9000` | Port for the RustFS S3 API. |
| `app_shaide:rustfsWebhookArn` | **Required** | `arn:rustfs:sqs:eu-central-1:shaide:webhook` | RustFS webhook ARN for object storage events. |
| `app_shaide:databaseUrl` | **Required** | `sqlite:///root/.config/axem/shaide/db/on-prem-db.sqlite` | SQLite database URL for shaide-server. |
| `app_shaide:s3User` | **Required** | `rustfsuser` | S3 access key username. |
| `app_shaide:s3UploadProxyRoutePrefix` | **Required** | `/s3` | Route prefix for S3 upload proxy. |
| `app_shaide:vectorDBUrl` | **Required** | `http://qdrant:6334` | Qdrant gRPC endpoint. |
| `app_shaide:controlPanelService` | **Required** | `control-panel` | Kubernetes service name for control-panel. |
| `app_shaide:webappService` | **Required** | `webapp` | Kubernetes service name for webapp. |
| `app_shaide:rustfsService` | **Required** | `rustfs` | Kubernetes service name for RustFS. |
| `app_shaide:qdrantService` | **Required** | `qdrant` | Kubernetes service name for Qdrant. |
| `app_shaide:rustfsNotifyWebhookEnableShaide` | **Required** | `on` | Enables RustFS → shaide-server webhook notifications. |
| `app_shaide:rustfsNotifyWebhookEndpointShaide` | **Required** | `http://shaide-server:8080/v1/object-storage/event` | Webhook endpoint on shaide-server for object storage events. |
| `app_shaide:rustfsNotifyWebhookQueueDirShaide` | **Required** | `/data/deploy/logs/notify` | RustFS webhook queue directory. |
| `app_shaide:rustLibBacktrace` | Optional | `1` | Rust backtrace verbosity level. |
| `app_shaide:rustSpantrace` | Optional | `0` | Rust span trace verbosity level. |

### Secret keys

| Config key | Required | Description |
|---|---|---|
| `app_shaide:adminAuthKey` | **Secret** | Shaide admin authentication key. Set via: `pulumi config set --secret app_shaide:adminAuthKey <value>`. |
| `app_shaide:s3Password` | **Secret** | RustFS (S3) admin password. Set via: `pulumi config set --secret app_shaide:s3Password <value>`. |

---

## 5. Model serving

Stack: `app_serving/deployments/Pulumi.<stack-name>.yaml`

Deploys the llm-d inference gateway and model workloads.
Add one entry per model under the `models` key.

### Base keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app-serving:cloudProvider` | **Required** | `on-prem` | Target platform identifier. Must be `on-prem`. |
| `app-serving:kubeconfig` | Optional | `~/.kube/rke2-cluster.yaml` | Path to the kubeconfig on the provisioner laptop. Falls back to `KUBECONFIG` env var or `~/.kube/config` when omitted. |

### GPU scheduling keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app-serving:gpuToleration.key` | **Required** | `dedicated` | Taint key on the GPU worker node. |
| `app-serving:gpuToleration.operator` | **Required** | `Exists` | Toleration operator. |
| `app-serving:gpuToleration.effect` | **Required** | `NoSchedule` | Must match the taint effect set in Ansible inventory `node_taints`. |

### Harbor registry keys

| Config key | Required | Example / Default | Description |
|---|---|---|---|
| `app-serving:harborHostname` | **Required** | `harbor.harbor.svc.cluster.local` | In-cluster Harbor service address. Used to construct the `imagePullSecret` in each model namespace. Must match `harbor:hostname` in the services stack. |
| `app-serving:harborUser` | **Required** | `robot$k8s-harbor-sa` | Harbor robot account username for pulling images. |
| `app-serving:harborToken` | **Secret** | — | Harbor robot account password. Set after `harbor_setup.yml` runs: `pulumi config set --secret app-serving:harborToken $(cat infra/on-prem/ansible/artifacts/harbor-robot-secret)`. |

### Models key

Defines which LLM and embedder models to deploy. Each entry creates its own namespace
(`llm-d-<name>`), infrastructure release, and inference gateway.

```yaml
app-serving:models:
  generative:
    - name: <model-slug>          # drives namespace and release names
      enabled: true
      nodeSelector:
        nodegroup: gpu-pro        # must match node label applied during provisioning
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<image>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 50Gi         # PV size for model weights
        hostpathNode: <node-name> # node where the model weights PV is created
        hostpathDir: /var/lib/hostpath/llm-models
  embedder:
    - name: <model-slug>
      enabled: true
      nodeSelector:
        nodegroup: gpu
      modelSource:
        harborRef: harbor.harbor.svc.cluster.local/ai-models/<image>:<tag>
        modelUri: hub/<org>/<model-name>
        storageSize: 5Gi
        hostpathNode: <node-name>
        hostpathDir: /var/lib/hostpath/llm-models
```

| Field | Required | Description |
|---|---|---|
| `name` | **Required** | Slug used for namespace (`llm-d-<name>`), Helm release, and gateway names. Must be unique per cluster. |
| `enabled` | **Required** | Set to `true` to deploy the model; `false` to keep the config but skip deployment. |
| `nodeSelector` | **Required** | Node label selector map (e.g. `nodegroup: gpu-pro`); must match labels applied during RKE2 provisioning. |
| `modelSource.harborRef` | **Required** | Full Harbor image reference for the model container. |
| `modelSource.modelUri` | **Required** | Model identifier passed to the inference engine (e.g. `hub/nomic-ai/nomic-embed-text-v1.5`). |
| `modelSource.storageSize` | **Required** | PersistentVolume size for model weights storage. |
| `modelSource.hostpathNode` | **Required** | Kubernetes node name where the model weights hostPath PV is created. Must have sufficient disk space. |
| `modelSource.hostpathDir` | **Required** | Absolute host path on `hostpathNode` where model weights are stored. |

---

## Secrets that must be set via CLI

These are never committed in plain text. Set them with `pulumi config set --secret` in the order shown:

| Stack | Config key | When |
|---|---|---|
| `k8s-onprem-airgap-services` | `harbor:adminPassword` | Before first `pulumi up` (Phase 1) |
| `k8s-onprem-airgap-services` | `harbor:robotPassword` | After `ansible-playbook harbor_setup.yml` (Phase 2) |
| `gateway-provider` | `gateway-provider:tlsCert` | Before `pulumi up` |
| `gateway-provider` | `gateway-provider:tlsKey` | Before `pulumi up` |
| `app_shaide` | `app_shaide:ghcrToken` | After `ansible-playbook harbor_setup.yml` |
| `app_shaide` | `app_shaide:adminAuthKey` | Before `pulumi up` |
| `app_shaide` | `app_shaide:s3Password` | Before `pulumi up` |
| `app-serving` | `app-serving:harborToken` | After `ansible-playbook harbor_setup.yml` |
