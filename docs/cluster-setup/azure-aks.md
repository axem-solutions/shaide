---
title: "Azure AKS"
description: "Creating an AKS cluster ready to run shaide."
weight: 40
---

# Azure AKS

Simple, command-based guide to stand up an AKS cluster with a GPU node pool, ready
for LLM inference workloads.

## Prerequisites

- An Azure subscription with permission to create resource groups and AKS clusters
- [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) installed
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) installed
- GPU quota for the target VM family in the target region (`NCadsA100v4`,
  `NVadsA10v5`, etc. — request an increase under **Subscriptions → Usage + quotas**
  if node pool creation fails with a quota error)

Log in and set your subscription:

```bash
az login
az account set --subscription <subscription-id>
```

## 1. Set variables

```bash
export RESOURCE_GROUP=<resource-group-name>
export CLUSTER_NAME=<cluster-name>
export LOCATION=<location>            # e.g. westeurope
```

## 2. Create the resource group

```bash
az group create --name "${RESOURCE_GROUP}" --location "${LOCATION}"
```

## 3. Create the cluster with a default (CPU) node pool

```bash
az aks create \
  --resource-group "${RESOURCE_GROUP}" \
  --name "${CLUSTER_NAME}" \
  --location "${LOCATION}" \
  --node-count 2 \
  --node-vm-size Standard_D4s_v5 \
  --generate-ssh-keys \
  --enable-workload-identity \
  --enable-oidc-issuer
```

`--enable-workload-identity`/`--enable-oidc-issuer` set up AKS Workload Identity,
needed if any workload authenticates to Azure services without static credentials.

This takes 5–10 minutes.

## 4. Get credentials and verify

```bash
az aks get-credentials --resource-group "${RESOURCE_GROUP}" --name "${CLUSTER_NAME}"

kubectl get nodes
kubectl get pods -A
```

## 5. Add a GPU node pool

```bash
az aks nodepool add \
  --resource-group "${RESOURCE_GROUP}" \
  --cluster-name "${CLUSTER_NAME}" \
  --name gpupool \
  --node-vm-size Standard_NC8as_T4_v3 \
  --node-count 0 \
  --min-count 0 \
  --max-count 3 \
  --enable-cluster-autoscaler \
  --labels nodegroup=gpu-nodepool \
  --node-taints sku=gpu:NoSchedule
```

AKS auto-installs the NVIDIA driver on GPU-series nodes — no separate DaemonSet
install needed. Adjust `--node-vm-size` for the workload (`Standard_NC8as_T4_v3` for
a single T4, `Standard_NC24ads_A100_v4` for A100-class GPUs).

Scale it up when you need capacity:

```bash
az aks nodepool scale \
  --resource-group "${RESOURCE_GROUP}" --cluster-name "${CLUSTER_NAME}" \
  --name gpupool --node-count 1
```

## 6. Verify the GPU is scheduled

```bash
kubectl get nodes -l nodegroup=gpu-nodepool
kubectl describe node -l nodegroup=gpu-nodepool | grep nvidia.com/gpu
```

Since the GPU node pool is tainted, workloads need a matching toleration
(`sku=gpu:NoSchedule`) to be scheduled onto it.

## Cleanup

```bash
az aks nodepool delete --resource-group "${RESOURCE_GROUP}" --cluster-name "${CLUSTER_NAME}" --name gpupool
az aks delete --resource-group "${RESOURCE_GROUP}" --name "${CLUSTER_NAME}"
az group delete --name "${RESOURCE_GROUP}"
```

## 7. Enable workload identity

The ALB Controller authenticates to Azure through workload identity, so the cluster needs
an OIDC issuer:

```bash
az aks update --resource-group "${RESOURCE_GROUP}" --name "${CLUSTER_NAME}" \
  --enable-oidc-issuer --enable-workload-identity

az aks show --resource-group "${RESOURCE_GROUP}" --name "${CLUSTER_NAME}" \
  --query oidcIssuerProfile.issuerUrl -o tsv
```

## 8. Load balancing — Application Gateway for Containers

AGC is the supported gateway on AKS. It runs outside the cluster and is programmed by an
in-cluster controller, so it needs a delegated subnet plus an identity.

Create a subnet delegated to the AGC service:

```bash
az network vnet subnet create \
  --resource-group "${RESOURCE_GROUP}" \
  --vnet-name "${VNET_NAME}" \
  --name subnet-alb \
  --address-prefixes <cidr> \
  --delegations Microsoft.ServiceNetworking/trafficControllers
```

Create a managed identity for the controller and grant it the roles AGC requires — at
minimum **AppGw for Containers Configuration Manager** on the resource group, and
**Network Contributor** on the delegated subnet:

```bash
az identity create --resource-group "${RESOURCE_GROUP}" --name azure-alb-identity
```

Federate that identity with the cluster's OIDC issuer for the
`azure-alb-system/alb-controller-sa` service account, then install the controller:

```bash
helm install alb-controller \
  oci://mcr.microsoft.com/application-lb/charts/alb-controller \
  --namespace azure-alb-system --create-namespace \
  --set albController.namespace=azure-alb-system \
  --set albController.podIdentity.clientID=<identity-client-id>
```

Verify the GatewayClass appears — shaide uses `azure-alb-external`:

```bash
kubectl get gatewayclass
kubectl -n azure-alb-system get pods
```

> AGC requires `Microsoft.ServiceNetworking/trafficControllers`, which is not offered in
> every Azure region. Confirm your region supports it before creating the cluster.

## 9. Storage

AKS ships `managed-csi` as the default StorageClass. Confirm it is present and marked
default:

```bash
kubectl get storageclass
```

## 10. Verify the cluster is conformant

Run the checks in [Verification](../cluster-requirements/verification.md) — in particular
the LoadBalancer test, which must return an `EXTERNAL-IP`.

## Cleanup additions

```bash
helm uninstall alb-controller -n azure-alb-system
az identity delete --resource-group "${RESOURCE_GROUP}" --name azure-alb-identity
```

## Next steps

This guide covers the AKS cluster and the cloud-side resources shaide depends on. The
in-cluster gateway (Istio, Gateway API CRDs, the shared Gateway resource) is deployed by
the platform itself.

With a conformant cluster and `kubectl` configured, continue with the
[installer](../installation/installer-guide.md).
