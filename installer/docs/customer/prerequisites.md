# Prerequisites

Complete this checklist before running the shaide AI Platform.

**Scope: this checklist covers on-prem (RKE2) deployments only.** Every command below
targets an on-prem RKE2 cluster (hardcoded `~/.kube/rke2-cluster.yaml` kubeconfig paths,
the `hostpath` StorageClass, and unconditional GPU/NVIDIA driver requirements). Cloud
deployments (GKE, EKS, AKS) use managed Kubernetes, cloud-native dynamic storage
provisioners instead of `hostpath`, and cloud-managed GPU node pools — none of the
node-readiness, HostPath, or GPU-driver steps below apply to a cloud install. If you are
installing on a cloud cluster, confirm the equivalent managed-cluster requirements with
your provisioner instead of following this checklist verbatim.

## Required before installation

| Area                    | Check |
|-------------------------|--------------------------------------------------------------------------------------------|
| **Provisioner machine** | The provisioner is ready to run the installer container.                                   |
| **Network and DNS**     | The provisioner can reach the cluster.                                                     |
| **Kubernetes access**   | The kubeconfig can access and manage the target cluster.                                   |
| **Node readiness**      | All target nodes are `Ready` and labeled for the expected workload placement.              |
| **GPU readiness**       | GPU nodes expose NVIDIA GPU resources to Kubernetes.                                       |
| **HostPath**            | The required StorageClasses are available.                                                 |
| **Installer bundle**    | `bundle.tar.gz` is present and readable.                                                   |
| **Credentials**         | All required tokens, passwords, and installer state passphrase are available.              |

## 1. Provisioner Machine
The provisioner machine is the Linux host where the installer is executed.

Check that Docker is installed and usable by the current user:

```bash
docker info
```

Check that the provisioner has enough local disk space for installer artifacts and extracted bundle contents.
Create the persistent installer state directory if it does not already exist:

```bash
mkdir -p /var/lib/shaide-installer
```

Choose any path with at least 100 GB free; this guide uses `/var/lib/shaide-installer`.

This directory must be preserved across installer re-runs and upgrades.

## 2. Network
The provisioner must be able to reach the target cluster before installation starts.

At minimum, the provisioner must be able to reach the Kubernetes API server:

```bash
nc -zv <control-plane-ip> 6443
```

Required network access from the provisioner:

| Port        | Protocol | Target        | Purpose              |
|-------------|----------|---------------|----------------------|
| ICMP        | —        | all nodes     | Connectivity testing |
| 22          | TCP      | all nodes     | SSH                  |
| 6443        | TCP      | control plane | Kubernetes API       |
| 443         | TCP      | all nodes     | Ingress (shaide)     |
| 30000–32767 | TCP/UDP  | all nodes     | NodePort range       |




## 3. Kubernetes access

The provisioner must have a `kubeconfig` for the target RKE2 cluster and it must have `cluster-admin` permissions.

Check that the `kubeconfig` can reach the cluster:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml version
```

Check whether the kubeconfig has cluster-admin permissions, if this returns `yes`, all permissions are satisfied:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml auth can-i '*' '*'
```

Check that all nodes run the same RKE2/Kubernetes version, all nodes should report the same `kubelet` version.

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes \
  -o custom-columns="NAME:.metadata.name,VERSION:.status.nodeInfo.kubeletVersion"
```

## 4. Node readiness

All target nodes must be registered with Kubernetes, report `Ready`, and have the expected taints applied.

Check node status, all target nodes must show ` STATUS = Ready`:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes
```

Check node taints:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml describe nodes | grep -A5 "Taints:"
```

Check labels:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes --show-labels
```


## 5. GPU readiness

NVIDIA drivers must already be installed on every GPU node before the installer runs. The installer does not install GPU drivers.

Check that GPU resources are advertised to Kubernetes:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml describe node <gpu-node> | grep nvidia.com/gpu
```

Check that NVIDIA GPU labels are present:

```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l nvidia.com/gpu.present=true
```

Reference: [NVIDIA GPU Operator platform support](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/platform-support.html#supported-operating-systems-and-kubernetes-platforms)

See [GPU Operator deployment scenarios](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html#common-deployment-scenarios) for details.


## 6. HostPath directories

The target cluster must provide HostPath storage for persistent data used by the shaide AI Platform.

Check the required `hostpath` StorageClass exists in the cluster:


```bash
kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get storageclass hostpath
```

## 7. Installer bundle

The required installation files must be present on the provisioner machine before the installer is started.

| File                      | Purpose                                               |
|---------------------------|-------------------------------------------------------|
| `installer.tar.gz` | Installer Docker image.                               |
| `bundle.tar.gz`           | Installation payload used by the installer.           |


## 8. Credentials

All credentials required by the installer must be available before installation starts.

Prepare the following values:
 
| Credential                | Purpose                                                                                 |
|---------------------------|-----------------------------------------------------------------------------------------|
| **Hugging Face token**    | Download model artifacts. Must have read access to all selected models.                 |
| **Installer state passphrase** | Encrypt installer deployment state. Use the same passphrase for future runs.       |
| **SSH private key**       | Required if the installer connects to cluster nodes for image preload.                  |
| **Harbor admin password** | Required to manage Harbor.                                                              |
| **Harbor robot password** | Image push and pull operations. Store it for future updates.                            |
| **shaide admin password** | Create the initial shaide administrator account.                                        |
