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

## 6. Storage — EBS CSI driver

EKS does not install a dynamic storage provisioner by default, and shaide requires a
default StorageClass. Attach the driver and give it an IAM role:

```bash
eksctl utils associate-iam-oidc-provider \
  --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" --approve

eksctl create iamserviceaccount \
  --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" \
  --namespace kube-system --name ebs-csi-controller-sa \
  --role-name AmazonEKS_EBS_CSI_DriverRole \
  --attach-policy-arn arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy \
  --role-only --approve

eksctl create addon --name aws-ebs-csi-driver \
  --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" \
  --service-account-role-arn "arn:aws:iam::${AWS_ACCOUNT_ID}:role/AmazonEKS_EBS_CSI_DriverRole" \
  --force
```

Create a `gp3` class and make it the default:

```bash
kubectl apply -f - <<'EOF'
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: gp3
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: ebs.csi.aws.com
volumeBindingMode: WaitForFirstConsumer
parameters:
  type: gp3
EOF
```

If `gp2` is currently marked default, clear it so only one default remains:

```bash
kubectl patch storageclass gp2 \
  -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"false"}}}'
```

## 7. Load balancing — AWS Load Balancer Controller

Required so `Service` type `LoadBalancer` and Gateway resources are provisioned. Without
it, external addresses stay `<pending>`.

Tag the subnets so the controller can discover them — public subnets carry
`kubernetes.io/role/elb=1`, private subnets `kubernetes.io/role/internal-elb=1`:

```bash
aws ec2 create-tags --region "${AWS_REGION}" \
  --resources <public-subnet-id> ... \
  --tags Key=kubernetes.io/role/elb,Value=1
```

Install the controller with its IAM policy:

```bash
curl -o iam-policy.json \
  https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/main/docs/install/iam_policy.json

aws iam create-policy \
  --policy-name AWSLoadBalancerControllerIAMPolicy \
  --policy-document file://iam-policy.json

eksctl create iamserviceaccount \
  --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}" \
  --namespace kube-system --name aws-load-balancer-controller \
  --attach-policy-arn "arn:aws:iam::${AWS_ACCOUNT_ID}:policy/AWSLoadBalancerControllerIAMPolicy" \
  --approve

helm repo add eks https://aws.github.io/eks-charts && helm repo update
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName="${CLUSTER_NAME}" \
  --set serviceAccount.create=false \
  --set serviceAccount.name=aws-load-balancer-controller
```

Verify:

```bash
kubectl -n kube-system get deploy aws-load-balancer-controller
```

## 8. Verify the cluster is conformant

Run the checks in [Verification](../cluster-requirements/verification.md) — in particular
the LoadBalancer test, which must return an `EXTERNAL-IP`.

## Cleanup additions

```bash
helm uninstall aws-load-balancer-controller -n kube-system
eksctl delete addon --name aws-ebs-csi-driver --cluster "${CLUSTER_NAME}" --region "${AWS_REGION}"
```

## Next steps

This guide covers the EKS cluster and the cloud-side resources shaide depends on. The
in-cluster gateway (Istio, Gateway API CRDs, the shared Gateway resource) is deployed by
the platform itself.

With a conformant cluster and `kubectl` configured, continue with the
[installer](../installation/installer-guide.md).
