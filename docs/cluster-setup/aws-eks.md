---
title: "AWS EKS"
description: "Creating an EKS cluster ready to run shaide."
weight: 20
---

# AWS EKS

Simple, command-based guide to stand up an EKS cluster with a GPU node group, ready
for LLM inference workloads.

## Prerequisites

- An AWS account with permissions to create EKS clusters, VPCs, IAM roles, and EC2
  instances (or node groups)
- [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)
  installed and configured: `aws configure` (or SSO: `aws sso login`)
- [`eksctl`](https://eksctl.io/installation/) installed
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) installed
- A GPU quota in the target region (`g5`, `g6`, or `p4d` instance types — request a
  quota increase in the AWS Console under **Service Quotas** if you hit `InsufficientInstanceCapacity`
  or quota errors)

Verify your identity and default region before starting:

```bash
aws sts get-caller-identity
aws configure get region
```

## 1. Set variables

```bash
export CLUSTER_NAME=<cluster-name>
export AWS_REGION=<region>            # e.g. eu-central-1
export K8S_VERSION=1.31
```

## 2. Create the cluster with a default (CPU) node group

```bash
eksctl create cluster \
  --name "${CLUSTER_NAME}" \
  --region "${AWS_REGION}" \
  --version "${K8S_VERSION}" \
  --nodegroup-name system \
  --node-type m6i.large \
  --nodes 2 \
  --nodes-min 1 \
  --nodes-max 3 \
  --managed
```

This takes 15–20 minutes. `eksctl` provisions the VPC, subnets, IAM roles, the EKS
control plane, and the managed node group, and writes a kubeconfig context for you.

## 3. Verify cluster access

```bash
kubectl get nodes
kubectl get pods -A
```

## 4. Add a GPU node group

```bash
eksctl create nodegroup \
  --cluster "${CLUSTER_NAME}" \
  --region "${AWS_REGION}" \
  --name gpu-nodes \
  --node-type g5.xlarge \
  --nodes 0 \
  --nodes-min 0 \
  --nodes-max 3 \
  --node-labels "nodegroup=gpu" \
  --managed
```

Scaling starts at 0 — the cluster autoscaler (or manual `eksctl scale nodegroup`)
brings GPU nodes up on demand. Adjust `--node-type` for the workload (`g5.xlarge`
for a single L4-class GPU, `p4d.24xlarge` for multi-GPU A100 training/inference).

Scale it manually when you need capacity:

```bash
eksctl scale nodegroup --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" \
  --name gpu-nodes --nodes 1
```

## 5. Install the NVIDIA device plugin

GPU nodes need the device plugin DaemonSet so Kubernetes can schedule pods against
`nvidia.com/gpu` resource requests:

```bash
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/main/deployments/static/nvidia-device-plugin.yml
```

Verify the GPU is advertised once a GPU node is up:

```bash
kubectl get nodes -l nodegroup=gpu
kubectl describe node -l nodegroup=gpu | grep nvidia.com/gpu
```

## Cleanup

```bash
eksctl delete nodegroup --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" --name gpu-nodes
eksctl delete cluster --name "${CLUSTER_NAME}" --region "${AWS_REGION}"
```

## Next steps

This guide only covers the EKS cluster itself — load balancing and ingress (AWS Load
Balancer Controller, subnet tagging, Gateway API) are set up by `infra/gateway-provider`,
not here.

With a running cluster and `kubectl` configured, continue with the platform
deployment order: `infra/cloud-harbor` → `infra/gateway-provider` → `app_serving` →
`app_shaide`. See the root [`README.md`](../../README.md) for the full architecture.
