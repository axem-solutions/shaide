# CHECKLIST — Installer (RKE2)

Pre-flight checklist for the on-prem installer (`onprem-installer`).
The installer is a containerized TUI that handles Harbor deployment, image upload, service deployment, and model download — it requires only **Go** and **Docker** on the provisioner machine.
The installer does not SSH into nodes — it communicates with the cluster exclusively through `kubectl` (Kubernetes API server on port 6443).

The items below cover only what have to be prepared before running the installer.

## Node roles

| Node | Role | GPU | Purpose |
|---|---|---|---|
| server1 | Control plane | — | Kubernetes API server, etcd, scheduler |
| server2 | Worker | — | Application workloads (Shaide, Harbor, RustFS, Qdrant) |
| server3 | Worker | Standard GPU | Embedding model inference |
| server4 | Worker | High-end GPU | LLM inference |

---

## 1 — Provisioner environment

1. [ ] Docker is installed and accessible without `sudo`:
   ```bash
   docker info
   ```
2. [ ] Go is installed:
   ```bash
   go version
   ```
3. [ ] A local directory for the Pulumi state file exists and is writable on the provisioner machine. The installer uses the Pulumi SDK internally and persists state locally — the directory must be mounted into the container on every run so state survives across executions:
   ```bash
   mkdir -p ~/.onprem-installer/state
   ```

---

## 2 — Installer bundle integrity

The installer mounts the bundle at `/.bundle/bundle.tar.gz` inside the container.
Run all checks below on the provisioner before starting the installer.

4. [ ] The bundle archive exists and is readable:
   ```bash
   ls -lh <path-to>/bundle.tar.gz
   ```
5. [ ] Checksum matches the expected value distributed alongside the bundle:
   ```bash
   sha256sum <path-to>/bundle.tar.gz
   # compare against the expected value in bundle.tar.gz.sha256
   ```
6. [ ] The archive contains `models.yaml` and all required image archives:
   ```bash
   tar -tzf <path-to>/bundle.tar.gz | sort
   ```
   Expected contents:

   - `models.yaml` — model manifest; must be the only manifest YAML file in the archive
   - All image tarballs listed below (OCI archives):

   **Images pushed to Harbor by the installer:**

   | File glob | Category | Description |
   |---|---|---|
   | `metallb-controller-*.tar` | MetalLB | MetalLB controller |
   | `metallb-speaker-*.tar` | MetalLB | MetalLB speaker |
   | `metallb-frr-*.tar` | MetalLB | FRRouting (MetalLB BGP dependency) |
   | `gpu-operator-*.tar` | GPU Operator | NVIDIA GPU Operator |
   | `cuda-*.tar` | GPU Operator | NVIDIA CUDA base image |
   | `driver-*.tar` | GPU Operator | NVIDIA driver image |
   | `k8s-driver-manager-*.tar` | GPU Operator | NVIDIA K8s driver manager |
   | `container-toolkit-*.tar` | GPU Operator | NVIDIA container toolkit |
   | `k8s-device-plugin-*.tar` | GPU Operator | NVIDIA K8s device plugin |
   | `dcgm-exporter-*.tar` | GPU Operator | DCGM exporter |
   | `dcgm-*.tar` | GPU Operator | DCGM |
   | `k8s-mig-manager-*.tar` | GPU Operator | NVIDIA MIG manager |
   | `nfd-*.tar` | GPU Operator | Node Feature Discovery |
   | `shaide_server-*.tar` | Shaide app | Shaide server |
   | `control_panel-*.tar` | Shaide app | Control panel |
   | `rustfs-*.tar` | Shaide app | RustFS object storage |
   | `qdrant-*.tar` | Shaide app | Qdrant vector database |
   | `busybox-*.tar` | Shaide app | Utility image |
   | `istio-pilot-*.tar` | Istio | Istio control plane |
   | `istio-proxyv2-*.tar` | Istio | Istio sidecar proxy |
   | `llm-d-inference-sim-*.tar` | llm-d | LLM-D inference simulator |
   | `epp-*.tar` | llm-d | Gateway API inference EPP |
   | `llm-d-inference-scheduler-*.tar` | llm-d | LLM-D inference scheduler |
   | `llm-d-cuda-*.tar` | llm-d | LLM-D CUDA serving image |
   | `infinity-*.tar` | llm-d | Infinity embedding server (CPU) |

   **Harbor registry images — loaded into containerd, not pushed to Harbor:**

   | File glob | Description |
   |---|---|
   | `harbor-*.tar` | Harbor component images — imported into containerd on the Harbor node before Harbor is deployed via Helm; NOT pushed to Harbor |

7. [ ] `models.yaml` is present and contains a non-empty `models` list:
   ```bash
   tar -xOzf <path-to>/images.tar.gz ./models.yaml | grep -c "id:"
   # must return > 0
   ```

---

## 3 — Hugging Face access

The installer downloads models directly from Hugging Face during the `populate Harbor` stage.
Both the network path and the token must be valid before starting.

Before running the installer, export the token on the provisioner:
```bash
export HF_TOKEN=<your-token>
```
The installer reads `HF_TOKEN` at startup and exits immediately if it is unset.

8. [ ] `huggingface.co` is reachable from the provisioner machine:
   ```bash
   curl -sSf https://huggingface.co > /dev/null && echo "reachable"
   ```
9. [ ] The Hugging Face token is valid and has access to the required model repositories:
   ```bash
   curl -sf -H "Authorization: Bearer $HF_TOKEN" \
     https://huggingface.co/api/whoami | jq .name
   # must return a username, not an auth error
   ```

---

## 4 — Network and firewall

The installer communicates with the cluster through the Kubernetes API server (6443).

10. [ ] **Provisioner → nodes** — all ports open:

   | Port | Proto | Target | Purpose |
   |---|---|---|---|
   | ICMP | — | all nodes | Connectivity testing |
   | 22 | TCP | all nodes | SSH |
   | 6443 | TCP | control plane | Kubernetes API |
   | 443 | TCP | all nodes | Ingress (Shaide) |
   | 30000–32767 | TCP/UDP | all nodes | NodePort range |

   Verify the API server is reachable from the provisioner:
   ```bash
   nc -zv <control-plane-ip> 6443
   ```

11. [ ] **Ingress name resolution** — the Shaide ingress hostname resolves to the correct IP on all machines that need access, via internal DNS or static `/etc/hosts` entries.
    ```bash
    dig +short <shaide-hostname>
    ```
    or if dig isn't available:
    ```bash
    nslookup <shaide-hostname>
    ```

---

## 5 — Kubeconfig preparation (RKE2-specific)

12. [ ] Verify the RKE2 version is supported by the GPU Operator:
    [NVIDIA GPU Operator platform support](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/platform-support.html#supported-operating-systems-and-kubernetes-platforms)
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml version
    ```
13. [ ] All nodes run the same RKE2 version:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes \
      -o custom-columns="NAME:.metadata.name,VERSION:.status.nodeInfo.kubeletVersion"
    ```
    All nodes must report the same version.
14. [ ] Verify the kubeconfig is functional and all nodes are `Ready`:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes
    ```

---

## 6 — Kubernetes cluster

15. [ ] The default RKE2 admin kubeconfig has `cluster-admin` and satisfies all requirements automatically. If a restricted user is used, verify the minimum permissions:

    | Resource | Namespace | Verbs |
    |---|---|---|
    | `namespaces` | cluster-scoped | `get` |
    | `services` | `harbor` | `get` |
    | `secrets` | `harbor` | `get` |
    | `endpointslices` | `harbor` | `list` |
    | `pods/portforward` | `harbor` | `create` |

    Verify with:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml auth can-i '*' '*'
    ```
    If this returns `yes`, all permissions are satisfied.

16. [ ] GPU worker nodes carry the taint `dedicated=gpu:NoSchedule` and the correct labels. Labels are defined in `ansible/inventory/hosts.yml` under each host's `node_labels` list:

    | Node | Taint | Labels |
    |---|---|---|
    | server2 | — | `app=shaide`, `nodegroup=no-gpu` |
    | server3 | `dedicated=gpu:NoSchedule` | `app=em`, `nodegroup=gpu` |
    | server4 | `dedicated=gpu:NoSchedule` | `app=llm`, `nodegroup=gpu-pro` |

    Verify on the cluster:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes --show-labels
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml describe nodes | grep -A5 "Taints:"

    # each must return exactly one node
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l app=shaide
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l app=em
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l app=llm
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l nodegroup=no-gpu
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l nodegroup=gpu
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l nodegroup=gpu-pro
    ```
    Apply any missing label before continuing:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml label node <node-name> app=shaide nodegroup=no-gpu   # server2
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml label node <node-name> app=em    nodegroup=gpu       # server3
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml label node <node-name> app=llm   nodegroup=gpu-pro   # server4
    ```

---

## 7 — Infra layer resources

The installer depends on the resources listed below. `StorageClass` must be provisioned before the installer runs — Harbor resources are created by the installer on a fresh install. 

### Required resources

| Kind | Name | Namespace | Required for | Notes |
|---|---|---|---|---|
| `StorageClass` | `hostpath` | — | both paths (Harbor + app PVs) | `kubernetes.io/no-provisioner`, `WaitForFirstConsumer` |
| `Namespace` | `harbor` | — | update path only | created by installer on fresh install |
| `Service` | `harbor` | `harbor` | update path only (port-forward) | created by installer on fresh install |
| `Secret` | `harbor-pull-secret` | `harbor` | update path only (registry auth) | created by installer on fresh install |

> **Fresh install path:** the installer deploys Harbor — `Namespace`, `Service`, and `Secret` do not need to exist beforehand.
>
> **Update/Upgrade path:** all three must exist and be healthy.

17. [ ] The `hostpath` StorageClass exists:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get storageclass hostpath
    ```

---

## 8 — GPU nodes, GPU Operator

The installer deploys the GPU Operator with `driver.enabled=false` — NVIDIA drivers **must be pre-installed on every GPU node** before the installer runs.

Reference: [NVIDIA GPU Operator platform support](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/platform-support.html#supported-operating-systems-and-kubernetes-platforms)

See [GPU Operator deployment scenarios](https://docs.nvidia.com/datacenter/cloud-native/gpu-operator/latest/getting-started.html#common-deployment-scenarios) for details.

18. [ ] NVIDIA drivers are pre-installed on `server3` and `server4`. This is a manual prerequisite — the installer does not install drivers.

19. [ ] The `nvidia.com/gpu` resource is advertised on GPU nodes:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml describe node <gpu-node> | grep nvidia.com/gpu
    ```
20. [ ] NVIDIA-related labels are present on GPU nodes:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get nodes -l nvidia.com/gpu.present=true
    ```

---

## 9 — HostPath directories

All PVs use the custom `hostpath` StorageClass (`kubernetes.io/no-provisioner`, `WaitForFirstConsumer`).
Static PVs are pinned to a specific node via `kubernetes.io/hostname` nodeAffinity.
PVs use `DirectoryOrCreate` — kubelet creates the directory if missing, but does so as root, which can cause ownership issues. Ensure all required directories exist with correct permissions before running the installer.

- **Fresh install:** all directories are new — create them before running the installer.
- **Update:** most directories already exist and contain live data (Harbor registry, model files). Create any directories required by newly added components; do not wipe existing ones.

Harbor must be deployed and the `busybox:1.37` image uploaded before running these checks. The image is pulled from Harbor (`<harbor-host>/images-infra/busybox:1.37`).

Each command must print directory listings without errors. A `No such file or directory` error means the directory is missing and must be created before the installer runs.

21. [ ] `server2` — Harbor and Shaide directories exist:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml run hostpath-check \
      --image=<harbor-host>/images-infra/busybox:1.37 --restart=Never --rm -it \
      --overrides='{
        "spec": {
          "nodeName": "<server2>",
          "volumes": [{"name":"varlib","hostPath":{"path":"/var/lib"}}],
          "containers": [{
            "name": "c", "image": "<harbor-host>/images-infra/busybox:1.37",
            "command": ["sh","-c",
              "ls -la /mnt/hostpath/harbor/registry \
                       /mnt/hostpath/harbor/jobservice \
                       /mnt/hostpath/harbor/database \
                       /mnt/hostpath/harbor/redis \
                       /mnt/app-shaide/shaide-server-data \
                       /mnt/app-shaide/rustfs-data \
                       /mnt/app-shaide/qdrant-data"],
            "volumeMounts": [{"name":"varlib","mountPath":"/mnt"}]
          }]
        }
      }'
    ```

22. [ ] `server3` — embedding model directory exists:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml run hostpath-check \
      --image=<harbor-host>/images-infra/busybox:1.37 --restart=Never --rm -it \
      --overrides='{
        "spec": {
          "nodeName": "<server3>",
          "tolerations": [{"key":"dedicated","operator":"Equal","value":"gpu","effect":"NoSchedule"}],
          "volumes": [{"name":"varlib","hostPath":{"path":"/var/lib"}}],
          "containers": [{
            "name": "c", "image": "<harbor-host>/images-infra/busybox:1.37",
            "command": ["sh","-c","ls -la /mnt/hostpath/llm-models"],
            "volumeMounts": [{"name":"varlib","mountPath":"/mnt"}]
          }]
        }
      }'
    ```

23. [ ] `server4` — LLM model directory exists:
    ```bash
    kubectl --kubeconfig ~/.kube/rke2-cluster.yaml run hostpath-check \
      --image=<harbor-host>/images-infra/busybox:1.37 --restart=Never --rm -it \
      --overrides='{
        "spec": {
          "nodeName": "<server4>",
          "tolerations": [{"key":"dedicated","operator":"Equal","value":"gpu","effect":"NoSchedule"}],
          "volumes": [{"name":"varlib","hostPath":{"path":"/var/lib"}}],
          "containers": [{
            "name": "c", "image": "<harbor-host>/images-infra/busybox:1.37",
            "command": ["sh","-c","ls -la /mnt/hostpath/llm-models"],
            "volumeMounts": [{"name":"varlib","mountPath":"/mnt"}]
          }]
        }
      }'
    ```
