---
title: "GCP GKE"
description: "Creating a GKE cluster ready to run shaide."
weight: 30
---

# GCP GKE

Simple, command-based guide to stand up a GKE cluster with a GPU node pool and
Gateway API enabled, ready for LLM inference workloads.

## Prerequisites

- A GCP project with billing enabled
- [`gcloud` CLI](https://cloud.google.com/sdk/docs/install) installed
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) installed
- GPU quota in the target region/zone (`NVIDIA_L4_GPUS`, `NVIDIA_A100_GPUS`, etc. —
  request an increase under **IAM & Admin → Quotas** if cluster/node-pool creation
  fails with a quota error)

Authenticate and set your project:

```bash
gcloud auth login
gcloud config set project <project-id>
gcloud auth application-default login
```

Enable the required APIs (one-time per project):

```bash
gcloud services enable container.googleapis.com compute.googleapis.com
```

## 1. Set variables

```bash
export CLUSTER_NAME=<cluster-name>
export GCP_REGION=<region>            # e.g. europe-west4
export GCP_ZONE="${GCP_REGION}-a"      # for zonal node pools
```

## 2. Create the cluster with a default (CPU) node pool

```bash
gcloud container clusters create "${CLUSTER_NAME}" \
  --region "${GCP_REGION}" \
  --num-nodes 1 \
  --machine-type e2-standard-4 \
  --gateway-api=standard \
  --workload-pool="$(gcloud config get-value project).svc.id.goog"
```

`--gateway-api=standard` enables GKE's built-in Gateway API support — a cluster-level
feature that can only be turned on at creation time or via `gcloud container clusters
update --gateway-api=standard`, not by anything deployed later. `--workload-pool`
enables Workload Identity, needed if any workload authenticates to GCP services (e.g.
Vertex AI) without static service-account keys.

This takes 10–15 minutes.

## 3. Get credentials and verify

```bash
gcloud container clusters get-credentials "${CLUSTER_NAME}" --region "${GCP_REGION}"

kubectl get nodes
kubectl get pods -A
```

## 4. Add a GPU node pool

```bash
gcloud container node-pools create gpu-pool \
  --cluster "${CLUSTER_NAME}" \
  --region "${GCP_REGION}" \
  --node-locations "${GCP_ZONE}" \
  --machine-type g2-standard-4 \
  --accelerator type=nvidia-l4,count=1,gpu-driver-version=latest \
  --num-nodes 0 \
  --min-nodes 0 \
  --max-nodes 3 \
  --enable-autoscaling \
  --node-labels nodegroup=gpu-nodepool
```

`gpu-driver-version=latest` tells GKE to auto-install the NVIDIA driver on the node —
no separate DaemonSet install needed (unlike self-managed Kubernetes). Adjust
`--machine-type`/`--accelerator` for the workload (`g2-standard-4` + `nvidia-l4` for
inference, `a2-highgpu-1g` + `nvidia-tesla-a100` for larger models).

Scale it up when you need capacity:

```bash
gcloud container clusters resize "${CLUSTER_NAME}" --region "${GCP_REGION}" \
  --node-pool gpu-pool --num-nodes 1
```

## 5. Verify the GPU is scheduled

```bash
kubectl get nodes -l nodegroup=gpu-nodepool
kubectl describe node -l nodegroup=gpu-nodepool | grep nvidia.com/gpu
```

## Cleanup

```bash
gcloud container node-pools delete gpu-pool --cluster "${CLUSTER_NAME}" --region "${GCP_REGION}"
gcloud container clusters delete "${CLUSTER_NAME}" --region "${GCP_REGION}"
```

## 6. Enable the Gateway API

GKE implements Gateway API through its own controller, which must be enabled on the
cluster:

```bash
gcloud container clusters update "${CLUSTER_NAME}" \
  --region "${GCP_REGION}" \
  --gateway-api=standard
```

Confirm the GatewayClasses are available — shaide uses
`gke-l7-regional-external-managed`:

```bash
kubectl get gatewayclass
```

## 7. Load balancing and DNS

GKE provisions Google Cloud load balancers directly from Gateway and Service resources,
so no controller install is needed. Reserve a static address so the ingress IP survives
recreation:

```bash
gcloud compute addresses create shaide-gateway-ip --region "${GCP_REGION}"
gcloud compute addresses describe shaide-gateway-ip --region "${GCP_REGION}" --format='value(address)'
```

Point your gateway hostname at that address with an `A` record.

## 8. TLS certificates

Enable Certificate Manager, which issues the managed certificate the Gateway references:

```bash
gcloud services enable certificatemanager.googleapis.com
```

Managed certificates validate via DNS authorization — add the CNAME record it asks for
before the certificate will issue.

## 9. Storage

GKE ships `standard-rwo` as the default StorageClass. Confirm it is present and marked
default:

```bash
kubectl get storageclass
```

No action is needed unless the default has been removed or changed.

## 10. Verify the cluster is conformant

Run the checks in [Verification](../cluster-requirements/verification.md) — in particular
the LoadBalancer test, which must return an `EXTERNAL-IP`.

## Cleanup additions

```bash
gcloud compute addresses delete shaide-gateway-ip --region "${GCP_REGION}"
```

## Next steps

This guide covers the GKE cluster and the cloud-side resources shaide depends on. The
in-cluster gateway (Istio, Gateway API CRDs, the shared Gateway resource) is deployed by
the platform itself.

With a conformant cluster and `kubectl` configured, continue with the
[installer](../installation/installer-guide.md).
