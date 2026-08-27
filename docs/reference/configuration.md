---
title: "Configuration reference"
description: "Complete reference of platform configuration keys."
weight: 10
---

# Configuration reference

This document is the single authoritative reference for every configurable variable
in the Ansible code. Variables are organised by source file, following the same
structure as the repository. Each entry shows the variable name, whether it is
required or optional, its default value, and a description of its purpose and
accepted values.

**Legend**

| Mark | Meaning |
|---|---|
| **Required** | Has no safe default — the playbook will fail or behave incorrectly if not set |
| **Required\*** | Required only when a specific feature is enabled |
| Optional | Has a working default; change only when the default does not fit the environment |
| Derived | Computed from other variables; do not set manually unless overriding a path |
| Secret | Must be stored in an Ansible Vault–encrypted file |

---

## 1. Inventory — `ansible/inventory/hosts.yml`

These variables are set per host or as inventory-wide connection defaults.

### 1.1 Connection Variables (inventory `vars` block)

Applied to every host in the inventory.

| Variable | Required | Default | Description |
|---|---|---|---|
| `ansible_user` | **Required** | `ansible` | OS user Ansible connects as. Must exist on all nodes with passwordless sudo. |
| `ansible_ssh_private_key_file` | **Required** | `~/.ssh/ansible_ed25519` | Path to the ed25519 private key on the provisioner laptop. |
| `ansible_password` | Optional | _(unset)_ | SSH password. Uncomment and reference `vault_ssh_password` instead of a plaintext value. Not used when key-based auth is active. |
| `ansible_become_password` | Optional | _(unset)_ | Sudo password. Uncomment and reference `vault_ssh_become_password`. Not used when passwordless sudo is configured. |

### 1.2 Per-Host Variables

Set individually for each host entry under `hosts:`.

| Variable | Required | Description |
|---|---|---|
| `ansible_host` | **Required** | IP address or hostname Ansible uses to reach the node. Must be a static IP confirmed before deployment. |
| `node_role` | **Required** | Role of this node in the cluster. Accepted values: `control_plane`, `worker`. Controls which sections of `config.yaml.j2` are rendered. |
| `node_labels` | Optional | List of Kubernetes node labels applied at registration. Format: `"key=value"`. Default is an empty list (defined in `group_vars/all`). Example: `["app=shaide", "nodegroup=gpu"]`. |
| `node_taints` | Optional | List of Kubernetes node taints applied at registration. Format: `"key=value:Effect"`. Default is an empty list. Example: `["dedicated=gpu:NoSchedule"]`. |
| `rke2_node_name` | Optional | Node name RKE2 registers in the cluster. Defaults to `inventory_hostname` so kubectl node names always match inventory names. Override per host only when the inventory name cannot be used. |

### 1.3 Inventory Groups

| Group | Members | Purpose |
|---|---|---|
| `control_plane` | server1 | Hosts running `rke2-server`. Drives `control_plane_ip` derivation. |
| `workers` | server2, server3, server4 | Hosts running `rke2-agent`. |
| `harbor` | server2 | Membership-only overlay group. The node where Harbor pods are scheduled. `harbor_setup.yml` targets this group for the image preload play. Must match the `harbor:nodeHostname` Pulumi config and the `harbor_node_hostname` variable. |
| `gpu_nodes` | server2, server3, server4 | Membership-only overlay group. Gates GPU preflight checks, applies `group_vars/gpu_nodes/main.yml`, and controls kernel-headers installation in `operator_managed` mode. |

---

## 2. Host Variables — `ansible/inventory/host_vars/`

One file per node. Contains only `hostpath_pv_dirs` — the list of subdirectories to
create under `hostpath_base_dir` on that specific node.

| Host | File | `hostpath_pv_dirs` value | Notes |
|---|---|---|---|
| server1 | `host_vars/server1.yml` | `[]` | Control plane; no application workloads. |
| server2 | `host_vars/server2.yml` | `["shaide", "rustfs", "qdrant", "harbor"]` | Shaide application worker. Adjust to match actual PV claims before provisioning. |
| server3 | `host_vars/server3.yml` | `["llm-models"]` | LLM model service worker (GPU, tainted `dedicated=gpu`). |
| server4 | `host_vars/server4.yml` | `["llm-models"]` | LLM model service worker (GPU-PRO, tainted `dedicated=gpu-pro`). |

---

## 3. Global Variables — `ansible/group_vars/all/main.yml`

Applied to every host in every group.

### 3.1 RKE2 Version

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_version` | **Required** | `v1.35.1+rke2r1` | RKE2 release to install. Must match the version of the artifacts staged on the provisioner. Format: `vX.Y.Z+rke2rN`. |

### 3.2 CNI Plugin

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_cni` | **Required** | `canal` | CNI plugin to deploy. Accepted values: `canal`, `calico`, `cilium`, `multus`. The corresponding air-gap image tarball must be present in `rke2_artifact_dir` before the playbook runs. Canal images are bundled inside the core `rke2-images.linux-amd64.tar.zst` — no separate tarball required. |

### 3.3 CNI Artifact Map

| Variable | Required | Default | Description |
|---|---|---|---|
| `cni_artifact_map` | Derived | See below | Maps each supported CNI plugin to the list of extra air-gap tarballs that must be present alongside the core RKE2 images. Used by `artifact_validate` and `rke2_install`. Do not override — edit the map entries if renaming bundles. |

Default map:

| CNI | Extra tarball(s) required |
|---|---|
| `canal` | _(none — bundled in core tarball)_ |
| `calico` | `rke2-images.calico.linux-amd64.tar.zst` |
| `cilium` | `rke2-images.cilium.linux-amd64.tar.zst` |
| `multus` | `rke2-images.multus.linux-amd64.tar.zst` |

### 3.4 RKE2 Installation Paths (on target nodes)

All roles reference these variables — do not hardcode paths in task files or templates.

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_config_dir` | Optional | `/etc/rancher/rke2` | Directory for RKE2 configuration files (`config.yaml`, `registries.yaml`, `rke2.yaml`). |
| `rke2_data_dir` | Optional | `/var/lib/rancher/rke2` | RKE2 data directory — binaries, agent state, and the node token live here. |
| `rke2_bin_dir` | Optional | `/var/lib/rancher/rke2/bin` | Directory containing the RKE2-bundled `kubectl`, `crictl`, and `ctr` binaries. |
| `rke2_images_dir` | Optional | `/var/lib/rancher/rke2/agent/images` | Destination for air-gap image tarballs on the node. RKE2 imports these automatically on startup. |
| `rke2_install_bin_dir` | Optional | `/usr/local/bin` | Directory where the `rke2` binary and uninstall script are installed. Must be on `$PATH`. |

### 3.5 Derived Paths

Computed from the base path variables above. Override only if using non-standard
installation paths.

| Variable | Derived From | Value |
|---|---|---|
| `rke2_kubectl_path` | `rke2_bin_dir` | `{{ rke2_bin_dir }}/kubectl` |
| `rke2_ctr_path` | `rke2_bin_dir` | `{{ rke2_bin_dir }}/ctr` |
| `rke2_kubeconfig_path` | `rke2_config_dir` | `{{ rke2_config_dir }}/rke2.yaml` |
| `rke2_node_token_path` | `rke2_data_dir` | `{{ rke2_data_dir }}/server/node-token` |
| `rke2_uninstall_script` | `rke2_install_bin_dir` | `{{ rke2_install_bin_dir }}/rke2-uninstall.sh` |
| `rke2_backup_binary_path` | `rke2_install_bin_dir` | `{{ rke2_install_bin_dir }}/rke2.prev` |
| `rke2_backup_config_path` | `rke2_config_dir` | `{{ rke2_config_dir }}/config.yaml.prev` |
| `control_plane_ip` | inventory | `{{ hostvars[groups['control_plane'][0]]['ansible_host'] }}` |
| `rke2_server_url` | `control_plane_ip` | `https://{{ control_plane_ip }}:9345` |
| `local_kubeconfig_path` | `rke2_cluster_name` | `~/.kube/{{ rke2_cluster_name }}.yaml` |

### 3.6 Offline Artifact Paths (on the provisioner laptop)

Paths default to subdirectories of the playbook directory so the project is
self-contained. Both directories are `.gitignore`d — binaries must never be
committed. Override at runtime with `-e rke2_artifact_dir=/opt/rke2-artifacts`
to use a centralised artifact cache.

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_artifact_dir` | **Required** | `{{ playbook_dir }}/artifacts/rke2` | Directory on the provisioner containing the RKE2 installer, image tarballs, and checksum file. Must exist and be populated before the playbook runs. |
| `harbor_image_dir` | **Required\*** | `{{ playbook_dir }}/artifacts/harbor-images` | Directory on the provisioner containing Harbor image tarballs. Required when `harbor_preload_enabled: true`. |
| `debian_debs_dir` | **Required\*** | `{{ playbook_dir }}/artifacts/debs` | Directory on the provisioner containing offline `.deb` packages for Debian nodes. Required when Debian nodes are in the inventory. |

### 3.7 Offline Artifact Filenames

Version-independent filenames matching the standard RKE2 release pipeline. Change
only if using renamed bundles.

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_images_tarball` | Optional | `rke2-images.linux-amd64.tar.zst` | Filename of the RKE2 core image tarball (includes Canal images). |
| `rke2_binary_tarball` | Optional | `rke2.linux-amd64.tar.gz` | Filename of the RKE2 binary tarball. |
| `rke2_installer_script` | Optional | `rke2.sh` | Filename of the RKE2 installer script. |
| `rke2_checksum_file` | Optional | `sha256sum-amd64.txt` | Filename of the SHA256 checksum file. Used by `artifact_validate` on the provisioner and `rke2_install` on nodes after transfer. |
| `rke2_staging_dir` | Optional | `/tmp/rke2-staging` | Temporary directory created on target nodes during installation. Removed after the install completes. |

### 3.8 Debian Offline Packages

| Variable | Required | Default | Description |
|---|---|---|---|
| `debian_offline_debs` | **Required\*** | `["{{ debian_debs_dir }}/rsync_3.2.7-1ubuntu1.2_amd64.deb"]` | List of `.deb` file paths to copy from the provisioner and install on Debian nodes before any apt mirror tasks run. Required for air-gapped Debian deployments. `rsync` is included because it is needed for artifact transfer; `nftables` is excluded because it is pre-installed on Debian 13 (trixie). |

### 3.9 Cluster Networking

> **Important:** Confirm all CIDRs do not overlap existing subnets in the customer's
> environment before deployment. Changes after provisioning require a full cluster rebuild.

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_cluster_cidr` | **Required** | `10.42.0.0/16` | CIDR range for Kubernetes pod networking. Must not overlap existing subnets. |
| `rke2_service_cidr` | **Required** | `10.43.0.0/16` | CIDR range for Kubernetes service IPs. Must not overlap existing subnets. |
| `rke2_cluster_dns` | Optional | `10.43.0.10` | IP address of the CoreDNS service. Must be within `rke2_service_cidr`. |

`control_plane_ip` and `rke2_server_url` are derived automatically from the inventory
(see §3.5). Set `ansible_host` on the control plane host; no manual entry required.

### 3.10 Registry Mirror

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_registry_mirror_enabled` | Optional | `false` | When `true`, generates `/etc/rancher/rke2/registries.yaml` on each node with the mirrors defined in `rke2_registry_mirrors`. |
| `rke2_registry_mirrors` | **Required\*** | `{}` | Map of upstream registry names to mirror endpoints. Required when `rke2_registry_mirror_enabled: true`. See the commented example in `group_vars/all/main.yml`. |

Example:

```yaml
rke2_registry_mirror_enabled: true
rke2_registry_mirrors:
  "docker.io":
    endpoint:
      - "https://harbor.internal/v2/dockerhub"
  "registry.k8s.io":
    endpoint:
      - "https://harbor.internal/v2/k8s"
```

### 3.11 GPU Operator Mode

| Variable | Required | Default | Description |
|---|---|---|---|
| `gpu_operator_mode` | **Required** | `pre_installed` | Controls GPU-specific preflight checks. `pre_installed` — asserts NVIDIA kernel modules are loaded on GPU nodes. `operator_managed` — asserts NO NVIDIA modules are present. Must be decided before provisioning. Overridden per group in `group_vars/gpu_nodes/main.yml`. |

### 3.12 Harbor Image Pre-load

Harbor image tarballs are copied to the Harbor node and imported into containerd as
the first play of `harbor_setup.yml`. The play is skipped automatically when Harbor
images are already present in containerd, so re-runs are safe.

| Variable | Required | Default | Description |
|---|---|---|---|
| `harbor_staging_dir` | Optional | `/tmp/harbor-images` | Temporary directory on target nodes where Harbor tarballs are staged before import. Cleaned up after the import completes. |
| `harbor_containerd_namespace` | Optional | `k8s.io` | containerd namespace used when importing Harbor images. Must be `k8s.io` for images to be visible to Kubernetes pods. |

### 3.13 hostPath PV Directories

| Variable | Required | Default | Description |
|---|---|---|---|
| `hostpath_base_dir` | Optional | `/var/lib/hostpath` | Base directory for hostPath PersistentVolume backing storage on each node. Must be on a partition with sufficient capacity. |
| `hostpath_pv_dirs` | Optional | `[]` | List of subdirectory names to create under `hostpath_base_dir`. Set per host in `inventory/host_vars/<hostname>.yml` — each node only gets the directories it needs. The global default is an empty list; nodes without a `host_vars` entry receive no subdirectories. |
| `hostpath_dirs_enabled` | Optional | `false` | When `true`, `os_prepare` creates hostPath directories as part of the normal provisioning run. When `false`, use `setup/hostpath_dirs.yml` for explicit, standalone control. |

### 3.14 `/etc/hosts` Population

| Variable | Required | Default | Description |
|---|---|---|---|
| `hosts_file_enabled` | Optional | `true` | When `true`, `os_prepare` writes cluster node entries into `/etc/hosts` on every node. Set to `false` to skip during `os_prepare` and use `setup/hosts_file.yml` for standalone control or to refresh entries after adding nodes. |
| `cluster_domain` | Optional | `internal.lan` | Domain suffix appended to each node's `inventory_hostname` to form its FQDN. Entries are written as `<ip>  <hostname>.<domain>  <hostname>`. Do not use `cluster.local` — it is reserved by CoreDNS for in-cluster service DNS. |

### 3.15 Node Journal Logs

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_log_dir` | Optional | `{{ playbook_dir }}/logs` | Directory on the provisioner laptop where RKE2 service journals are saved. Created automatically on first run. Every node writes `rke2_<hostname>.log` after a successful join; failed nodes additionally write `rke2_<hostname>_failure.log`. |

### 3.16 Upgrade and Rollback

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_upgrade_drain_timeout` | Optional | `300s` | `kubectl drain --timeout` value during an upgrade. Increase on clusters with long-running pods. |
| `rke2_upgrade_drain_task_timeout` | Optional | `330` | Ansible-level hard timeout (seconds) for the drain command task. Must be greater than `rke2_upgrade_drain_timeout` to allow kubectl to surface its own timeout error before Ansible kills the process. |

The backup paths `rke2_backup_binary_path` and `rke2_backup_config_path` are derived
— see §3.5. Do not set them manually unless using non-standard installation paths.

### 3.17 Destroy Options

| Variable | Required | Default | Description |
|---|---|---|---|
| `destroy_remove_hostpath_dirs` | Optional | `false` | When `true`, the destroy playbook deletes the `hostpath_base_dir` subtree on all nodes, permanently removing PV data. Must be explicitly set to `true` to take effect. |
| `rke2_destroy_drain_timeout` | Optional | `120s` | `kubectl drain --timeout` value during cluster destruction. |
| `rke2_destroy_drain_task_timeout` | Optional | `150` | Ansible-level hard timeout (seconds) for the drain task. Must be greater than `rke2_destroy_drain_timeout`. |

### 3.18 Readiness Check Timing

Controls how long the playbook waits for nodes to become Ready after service start.
Total wait time = `rke2_node_ready_retries` × `rke2_node_ready_delay` seconds.

| Variable | Required | Default | Total (default) | Description |
|---|---|---|---|---|
| `rke2_node_ready_retries` | Optional | `60` | — | Number of polling attempts before declaring the node failed. |
| `rke2_node_ready_delay` | Optional | `10` | 600 s (10 min) | Seconds to wait between each readiness poll. |

### 3.19 Cluster Identity

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_cluster_name` | Optional | `rke2-cluster` | Logical name for the cluster. Used as the kubeconfig filename on the provisioner and as the context, cluster, and user name inside the fetched kubeconfig. Change when managing multiple clusters from the same provisioner to avoid context collisions. |

### 3.20 Preflight Time Synchronisation

Kubernetes components are sensitive to clock drift. etcd's Raft consensus requires
all members within **1 second** of each other; TLS certificate validation and JWT
token expiry fail beyond **~5 seconds**. In an air-gapped environment a reachable
internal NTP source on `123/udp` is required before provisioning starts.

| Variable | Required | Default | Description |
|---|---|---|---|
| `preflight_skip_time_check` | Optional | `true` | Skip all NTP sync and clock skew checks. Enabled by default because the target environment has no internal NTP server yet. Set to `false` once an NTP source is available. |

### 3.21 Preflight Disk Space Requirements

Disk space requirements are declared as lists of `{ path, min_gib }` entries.
No paths or sizes are hardcoded in task files — add any entry to the appropriate
list and the preflight check picks it up automatically.

**No dedicated partitions required.** If a checked path has no dedicated mount
point, the check resolves it upward to whatever mount actually contains it (usually
`/`) and aggregates requirements for that mount. For example, if everything is on
`/` and you declare three paths requiring 2 + 10 + 1 GiB, the check asserts that
`/` has at least 13 GiB free — not three separate assertions.

| Variable | Defined in | Description |
|---|---|---|
| `preflight_disk_requirements` | `group_vars/all/main.yml` | Paths checked on **every** node. |
| `preflight_disk_requirements_extra` | `group_vars/<group>/main.yml` | Paths checked only on nodes in that group. |

**Default entries (`group_vars/all/main.yml`):**

| Path | Min GiB |
|---|---|
| `/var/lib/containerd` | 2 |
| `{{ rke2_data_dir }}` | 10 |
| `/var/lib/kubelet` | 1 |

To add a path on all nodes, append to `preflight_disk_requirements`.
To add a path only for a specific group, append to `preflight_disk_requirements_extra`
in the matching `group_vars` file.

### 3.22 Preflight OS Requirements

Defines the set of supported OS distributions. Every node is asserted against the
`os_allowed` list during preflight — the playbook aborts if a node does not match
any entry.

| Variable | Required | Default | Description |
|---|---|---|---|
| `os_allowed` | **Required** | See below | List of `{distribution, version, codename}` entries. A node passes if it matches any one entry. Add a new entry here to support an additional distribution or version. |
| `os_check_codename` | Optional | `true` | When `true`, the codename field inside each `os_allowed` entry is also checked against `ansible_distribution_release`. Disable if the distribution does not use codenames. |

**Default `os_allowed` entries:**

| distribution | version | codename |
|---|---|---|
| `Ubuntu` | `24.04` | `noble` |
| `Debian` | `13` | `trixie` |

### 3.23 kube-proxy Arguments

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_kube_proxy_args` | Optional | `[]` | List of extra arguments passed to kube-proxy via `config.yaml`. The global default is empty (iptables mode). Overridden in OS-specific vars files — Debian nodes set `proxy-mode=nftables` and related conntrack tuning here. |

### 3.24 Node Labels and Taints (defaults)

These defaults ensure that hosts with no labels or taints defined in the inventory
still render a valid RKE2 `config.yaml`.

| Variable | Required | Default | Description |
|---|---|---|---|
| `node_labels` | Optional | `[]` | Default empty list. Per-host values set in inventory override this. |
| `node_taints` | Optional | `[]` | Default empty list. Per-host values set in inventory override this. |

### 3.25 Node Name

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_node_name` | Optional | `{{ inventory_hostname }}` | Node name RKE2 registers in the cluster. Defaults to `inventory_hostname` so that kubectl node names always match Ansible inventory names — this keeps wait_ready polling, node-token fetching, and taint/label logic consistent regardless of the OS-level hostname. Override per host in `inventory/hosts.yml` only when the inventory name cannot be used as the node name. |

---

## 4. Secrets — `ansible/group_vars/all/vault.yml`

This file must be encrypted with Ansible Vault before committing. All variables use
the `vault_` prefix by convention and are referenced by their canonical names in
`group_vars/all/main.yml`.

| Variable | Required | Description |
|---|---|---|
| `vault_rke2_token` | Optional | RKE2 cluster join token. When empty, RKE2 auto-generates a token on first boot. When set, the token is written into `config.yaml` on the control plane and used by all workers — making the token reproducible across rebuilds. Recommended for production. |
| `vault_harbor_admin_password` | **Required** | Harbor administrator password. Used by `harbor_setup.yml` to authenticate against the Harbor API. Must match the `harbor:adminPassword` secret set in the Pulumi services stack (see §10.2). |
| `vault_ssh_password` | **Required\*** | SSH password for the `ansible` user. Required when `ansible_password` is enabled in the inventory. Not needed when key-based authentication is used (default). |
| `vault_ssh_become_password` | **Required\*** | Sudo password for the `ansible` user. Required when `ansible_become_password` is enabled in the inventory. Not needed when passwordless sudo is configured (default). |

---

## 5. Control Plane Group — `ansible/group_vars/control_plane/main.yml`

Applied only to hosts in the `control_plane` group.

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_service_name` | Optional | `rke2-server` | systemd service name for the RKE2 control plane. Used by all tasks that start, stop, or query the service. |
| `rke2_disable_cloud_controller` | Optional | `true` | Disables the Kubernetes cloud controller manager. Set to `true` for bare-metal and on-prem deployments where no cloud provider integration is needed. |
| `preflight_disk_requirements_extra` | Optional | `[{path: /var/lib/etcd, min_gib: 2}]` | Extra disk space requirements checked only on control plane nodes. Merged with `preflight_disk_requirements` at runtime. See §3.21 for the list format. |
| `kubectl_access_users` | Optional | `["{{ ansible_user }}"]` | List of OS usernames on the control plane node that receive a personal copy of the kubeconfig at `~/.kube/config`. Grants non-root kubectl access without sudo. Add operator accounts here. |

---

## 6. Workers Group — `ansible/group_vars/workers/main.yml`

Applied only to hosts in the `workers` group.

| Variable | Required | Default | Description |
|---|---|---|---|
| `rke2_service_name` | Optional | `rke2-agent` | systemd service name for the RKE2 worker agent. Used by all tasks that start, stop, or query the service on worker nodes. |
| `preflight_disk_requirements_extra` | Optional | `[{path: "{{ hostpath_base_dir }}", min_gib: 10}]` | Extra disk space requirement checked only on worker nodes — ensures the hostPath PV base directory has adequate free space. See §3.21. |

---

## 7. GPU Nodes Group — `ansible/group_vars/gpu_nodes/main.yml`

Applied only to hosts in the `gpu_nodes` group. In this project, server2, server3,
and server4 are members of both `workers` and `gpu_nodes`.

| Variable | Required | Default | Description |
|---|---|---|---|
| `gpu_operator_mode` | **Required** | `pre_installed` | GPU-specific preflight behaviour for nodes in this group. Overrides the global default from `group_vars/all/main.yml`. `pre_installed` — asserts NVIDIA kernel modules are loaded. `operator_managed` — asserts no NVIDIA modules are present. Must match the GPU Operator deployment mode chosen for the cluster. |

> **Testing RKE2 provisioning without NVIDIA drivers**
> To run only the RKE2 provisioning playbook on a node that does not have NVIDIA
> drivers installed yet, comment that host out of the `gpu_nodes` group in
> `inventory/hosts.yml`. The node will still be provisioned and joined to the cluster
> as a normal worker — GPU preflight checks, kernel-headers installation, and
> `gpu_operator_mode` logic are skipped entirely because they are gated on `gpu_nodes`
> membership. Remove the comment when the GPU stack is ready.
>
> ```yaml
> gpu_nodes:
>   hosts:
>     #server2:
>     #server3:   # comment out to skip GPU checks during RKE2-only testing
>     #server4:
> ```

---

## 8. OS-Specific Variables — `ansible/roles/os_prepare/vars/`

Loaded automatically at runtime based on `ansible_distribution`. These are internal
to the `os_prepare` role — not set by the operator. Documented here for reference.

### `vars/Ubuntu.yml`

Covers Ubuntu 24.04 (noble). kube-proxy runs in **iptables mode** (default).

| Variable | Value |
|---|---|
| `rke2_kernel_modules` | `[br_netfilter, overlay]` |
| `rke2_sysctl_settings` | See below |

sysctl settings applied on Ubuntu:

| Parameter | Value | Purpose |
|---|---|---|
| `net.bridge.bridge-nf-call-iptables` | `1` | Enables iptables to process bridged traffic — required for CNI. |
| `net.bridge.bridge-nf-call-ip6tables` | `1` | Same as above for IPv6. |
| `net.ipv4.ip_forward` | `1` | Enables IP forwarding — required for pod-to-pod routing. |
| `net.ipv6.conf.all.forwarding` | `1` | Enables IPv6 forwarding. |
| `fs.inotify.max_user_instances` | `8192` | Raises inotify instance limit for kubelet and container runtimes. |
| `fs.inotify.max_user_watches` | `524288` | Raises inotify watch limit to prevent resource exhaustion. |
| `vm.overcommit_memory` | `1` | Allows memory overcommit — required for etcd and Go-based workloads. |
| `vm.max_map_count` | `262144` | Required for Elasticsearch-based workloads (e.g. Qdrant). |

### `vars/Debian.yml`

Covers Debian 13 (trixie). kube-proxy runs in **nftables mode**
(`rke2_kube_proxy_args` is set here and rendered into `config.yaml`).

| Variable | Value |
|---|---|
| `rke2_kernel_modules` | `[br_netfilter, overlay, nf_conntrack, nf_nat, nf_tables]` |
| `rke2_sysctl_settings` | See below |
| `rke2_kube_proxy_args` | `["proxy-mode=nftables", "conntrack-max-per-core=250000", "conntrack-tcp-timeout-established=86400s"]` |

Debian loads `nf_conntrack`, `nf_nat`, and `nf_tables` explicitly because Debian's
vanilla kernel does not auto-load them via `br_netfilter` the way Ubuntu's patched
kernel does.

sysctl settings applied on Debian (superset of Ubuntu):

| Parameter | Value | Purpose |
|---|---|---|
| `net.bridge.bridge-nf-call-iptables` | `1` | Enables iptables to process bridged traffic. |
| `net.bridge.bridge-nf-call-ip6tables` | `1` | Same as above for IPv6. |
| `net.ipv4.ip_forward` | `1` | Enables IP forwarding. |
| `net.ipv6.conf.all.forwarding` | `1` | Enables IPv6 forwarding. |
| `net.netfilter.nf_conntrack_max` | `1000000` | Maximum conntrack table entries — required for kube-proxy nftables mode. |
| `net.netfilter.nf_conntrack_buckets` | `250000` | Conntrack hash table buckets. |
| `net.netfilter.nf_conntrack_tcp_timeout_established` | `86400` | TCP established connection tracking timeout (seconds). |
| `net.netfilter.nf_conntrack_tcp_timeout_close_wait` | `3600` | TCP CLOSE_WAIT tracking timeout (seconds). |
| `fs.inotify.max_user_instances` | `8192` | Raises inotify instance limit. |
| `fs.inotify.max_user_watches` | `524288` | Raises inotify watch limit. |
| `vm.overcommit_memory` | `1` | Allows memory overcommit. |
| `vm.max_map_count` | `262144` | Required for Elasticsearch-based workloads. |

### `vars/RedHat.yml`

Covers Rocky Linux, AlmaLinux, and RHEL. kube-proxy runs in **iptables mode**.

| Variable | Value |
|---|---|
| `rke2_kernel_modules` | `[br_netfilter, overlay]` |
| `rke2_sysctl_settings` | See below |

sysctl settings applied on RedHat family:

| Parameter | Value | Purpose |
|---|---|---|
| `net.bridge.bridge-nf-call-iptables` | `1` | Enables iptables to process bridged traffic. |
| `net.bridge.bridge-nf-call-ip6tables` | `1` | Same as above for IPv6. |
| `net.ipv4.ip_forward` | `1` | Enables IP forwarding. |
| `fs.inotify.max_user_instances` | `8192` | Raises inotify instance limit. |
| `fs.inotify.max_user_watches` | `524288` | Raises inotify watch limit. |
| `vm.overcommit_memory` | `1` | Allows memory overcommit. |

---

## 9. Quick Reference — Variables to Set Before Every Deployment

These are the variables that must be reviewed and confirmed for every new deployment.
All others have safe defaults.

| Variable | File | Why it must be set |
|---|---|---|
| `ansible_host` (per host) | `inventory/hosts.yml` | Static IP of each server |
| `ansible_host` on control plane | `inventory/hosts.yml` | Drives `control_plane_ip` — derived automatically from this value |
| `rke2_version` | `group_vars/all/main.yml` | Must match the staged artifact version |
| `rke2_cluster_name` | `group_vars/all/main.yml` | Determines kubeconfig filename and context name |
| `rke2_cluster_cidr` | `group_vars/all/main.yml` | Confirm no overlap with customer subnets |
| `rke2_service_cidr` | `group_vars/all/main.yml` | Confirm no overlap with customer subnets |
| `cluster_domain` | `group_vars/all/main.yml` | Domain suffix for FQDN entries in `/etc/hosts` — confirm it matches internal DNS |
| `node_labels` (per host) | `inventory/hosts.yml` | Workload scheduling depends on correct labels |
| `gpu_operator_mode` | `group_vars/gpu_nodes/main.yml` | Must match the agreed GPU driver strategy |
| `hostpath_pv_dirs` (per host) | `inventory/host_vars/<hostname>.yml` | Adjust directory names to match actual PV claims |
| `vault_rke2_token` | `group_vars/all/vault.yml` | Set for reproducible cluster rebuilds |
| `vault_harbor_admin_password` | `group_vars/all/vault.yml` | Must match the Pulumi `harbor:adminPassword` secret |
| `preflight_skip_time_check` | `group_vars/all/main.yml` | Set to `false` once an internal NTP source is available |

---

## 10. Pulumi Configuration Reference

Each Pulumi stack has its own `Pulumi.<stack-name>.yaml` file that stores non-secret
config. Secrets are never stored in YAML — they are injected via
`pulumi config set --secret` and stored encrypted in the Pulumi state backend.

**Legend**

| Mark | Meaning |
|---|---|
| **Required** | Must be set before `pulumi up`; the stack will error if absent |
| Optional | Has a hard-coded default; override only to use a different value |
| **Secret** | Must be set via `pulumi config set --secret`; never commit in plain text |

---

### 10.1 `pulumi/infra` — Cluster Infrastructure

Stack file: `pulumi/infra/Pulumi.rke2-cluster.yaml`
Pulumi project name: `k8s-onprem-airgap-infra`

| Config key | Required | Default | Description |
|---|---|---|---|
| `k8s-onprem-airgap-infra:kubeconfig` | **Required** | _(none)_ | Path to the kubeconfig on the provisioner laptop. Set to `~/.kube/rke2-cluster.yaml` after Ansible fetches it. |
| `k8s-onprem-airgap-infra:components` | **Required** | _(none)_ | YAML list of component names to deploy. Valid value: `[storageclass]`. |

**Available components:**

| Component | What it creates |
|---|---|
| `storageclass` | `hostpath` StorageClass with `WaitForFirstConsumer` binding and `Retain` reclaim policy |

---

### 10.2 `pulumi/services` — Platform Services

Stack file: `pulumi/services/Pulumi.rke2-cluster.yaml`
Pulumi project name: `k8s-onprem-airgap-services`

#### Base keys

| Config key | Required | Default | Description |
|---|---|---|---|
| `k8s-onprem-airgap-services:kubeconfig` | **Required** | _(none)_ | Path to the kubeconfig on the provisioner laptop. |
| `k8s-onprem-airgap-services:components` | **Required** | _(none)_ | YAML list of components to enable. Deploy `[harbor]` first; add `metallb` and `gpu-operator` only after images are uploaded to Harbor. |

#### Harbor keys

| Config key | Required | Default | Description |
|---|---|---|---|
| `harbor:hostname` | **Required** | _(none)_ | Internal hostname for Harbor (e.g. `harbor.internal.lan`). Must match `harbor_hostname` in `ansible/group_vars/all/harbor.yml` and the `/etc/hosts` entry written by `harbor_setup.yml`. |
| `harbor:nodeHostname` | **Required** | _(none)_ | Kubernetes node name where Harbor pods are scheduled (e.g. `srv2rke2w1`). Used to pin Harbor PersistentVolumes via `nodeAffinity`. Must match `harbor_node_hostname` in `harbor.yml`. |
| `harbor:chartPath` | Optional | `./charts/harbor-1.18.2.tgz` | Path to the Harbor Helm chart tarball, relative to `pulumi/services/`. Override only when using a different chart version. |
| `harbor:adminPassword` | **Secret** | _(none)_ | Harbor administrator password. Set once before the first `pulumi up`: `pulumi config set --secret harbor:adminPassword <password>`. Must match `vault_harbor_admin_password` in `ansible/group_vars/all/vault.yml`. |
| `harbor:robotSecret` | **Secret** | _(none)_ | Harbor robot account secret. Set after `harbor_setup.yml` runs: `pulumi config set --secret harbor:robotSecret $(cat ../ansible/artifacts/harbor-robot-secret)`. When absent, Pulumi skips the `imagePullSecret` creation for MetalLB and GPU Operator — re-run `pulumi up` after registering it. |

#### MetalLB keys

| Config key | Required | Default | Description |
|---|---|---|---|
| `metallb:ipPool` | **Required\*** | _(none)_ | IP address range allocated to MetalLB for LoadBalancer services (e.g. `10.99.10.200-10.99.10.220`). Must not overlap existing subnet assignments. Required when `metallb` is in the components list. |
| `metallb:controllerNodeHostname` | **Required\*** | _(none)_ | Kubernetes node name where the MetalLB controller pod is pinned. Required when `metallb` is in the components list. |
| `metallb:chartPath` | Optional | `./charts/metallb-0.15.3.tgz` | Path to the MetalLB Helm chart tarball, relative to `pulumi/services/`. |

#### GPU Operator keys

| Config key | Required | Default | Description |
|---|---|---|---|
| `gpu-operator:gpuNodeHostname` | **Required\*** | _(none)_ | Kubernetes node name of the GPU worker (e.g. `srv3rke2w2`). Used to pin the GPU Operator controller pod. Required when `gpu-operator` is in the components list. |
| `gpu-operator:chartPath` | Optional | `./charts/gpu-operator-v25.10.1.tgz` | Path to the GPU Operator Helm chart tarball, relative to `pulumi/services/`. |

**Available components:**

| Component | What it creates | Dependency |
|---|---|---|
| `harbor` | `harbor` namespace, hostPath PVs, Harbor Helm release | `infra` stack (StorageClass must exist) |
| `metallb` | `metallb-system` namespace, MetalLB Helm release, `imagePullSecret`, `IPAddressPool`, `L2Advertisement` | Harbor running + `harbor:robotSecret` set |
| `gpu-operator` | `gpu-operator` namespace, GPU Operator Helm release, `imagePullSecret` | Harbor running + `harbor:robotSecret` set |

---

### 10.3 `pulumi/deployments` — Application Workloads

Stack file: `pulumi/deployments/Pulumi.rke2-cluster.yaml`
Pulumi project name: `k8s-onprem-airgap-deployments`

| Config key | Required | Default | Description |
|---|---|---|---|
| `k8s-onprem-airgap-deployments:kubeconfig` | **Required** | _(none)_ | Path to the kubeconfig on the provisioner laptop. |
| `k8s-onprem-airgap-deployments:components` | **Required** | _(none)_ | YAML list of verification workloads to deploy. |
| `harbor:hostname` | **Required** | _(none)_ | Harbor hostname used to construct the `imagePullSecret` registry address. Must match the value in the services stack. |
| `gpu-operator:gpuNodeHostname` | **Required\*** | _(none)_ | GPU node hostname used to schedule the `test-gpu` Job. Required when `test-gpu` is in the components list. |

**Available components:**

| Component | What it creates | Prerequisite |
|---|---|---|
| `test-metallb` | `nginx:alpine` Deployment + `LoadBalancer` Service; verifies MetalLB assigns an external IP | MetalLB deployed, `nginx:alpine` uploaded to Harbor |
| `test-gpu` | `nvidia-smi` Job on the GPU node; verifies GPU Operator, device plugin, and container toolkit | GPU Operator deployed, CUDA/nvidia-smi image uploaded to Harbor |

---

### 10.4 Quick Reference — Pulumi Values to Set Before Every Deployment

| Value | Stack | How to set | Notes |
|---|---|---|---|
| `harbor:adminPassword` | services | `pulumi config set --secret harbor:adminPassword <pw>` | Must match `vault_harbor_admin_password` |
| `harbor:hostname` | services, deployments | Edit `Pulumi.rke2-cluster.yaml` | Must match `harbor_hostname` in `harbor.yml` |
| `harbor:nodeHostname` | services | Edit `Pulumi.rke2-cluster.yaml` | Kubernetes node name where Harbor runs |
| `metallb:ipPool` | services | Edit `Pulumi.rke2-cluster.yaml` | Confirm range is free in the customer network |
| `metallb:controllerNodeHostname` | services | Edit `Pulumi.rke2-cluster.yaml` | Node to pin MetalLB controller |
| `gpu-operator:gpuNodeHostname` | services, deployments | Edit `Pulumi.rke2-cluster.yaml` | GPU worker node name |
| `harbor:robotSecret` | services | `pulumi config set --secret harbor:robotSecret $(cat ...)` | Set after `harbor_setup.yml` completes |
