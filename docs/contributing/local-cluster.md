---
title: "Local Kubernetes cluster"
description: "Running a local cluster for development and testing."
weight: 20
---

# Local Kubernetes cluster

A vCluster running on the **Docker driver** (a.k.a. **vind** — vCluster in
Docker). The setup does **not** need kind, k3d, or any pre-existing Kubernetes
host cluster — the vCluster CLI creates and manages the underlying node
containers directly via the Docker API.

[Official Documentation](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml/experimental/docker)

## Purpose

A disposable, laptop-sized Kubernetes cluster for developers to exercise the
`app_serving` / `app_shaide` Pulumi stacks (or any other manifests in this
repo) locally, without needing a cloud cluster. Unlike every other module
under `infra/`, this one is **not** Pulumi — it's a plain `vcluster` CLI
invocation against a values file, since standing up a real cluster (rather
than deploying resources onto one) is out of scope for Pulumi here.

## Documentation index

This README covers setup and day-to-day usage. Deeper dives into specific
`cluster.yaml` config knobs and one-off scenarios live under
[`documentation/`](local-cluster/):

```
infra/local-k8s/
├── README.md                        # this file — setup, usage, terminal output examples
├── cluster.yaml                     # vcluster values file (single source of truth)
└── documentation/
    ├── ENV.md                       # setting env vars via experimental.docker.env / nodes[].env
    ├── K8S_VERSION.md               # pinning the Kubernetes version (controlPlane.distro.k8s.image.tag)
    ├── LB.md                        # built-in LoadBalancer Service support (no MetalLB needed)
    ├── MONITORING.md                # checking node/pod resource usage (docker stats, kubectl top)
    ├── NETWORKING.md                # custom podCIDR / serviceCIDR
    ├── PORTS.md                     # exposing container ports to the host
    ├── REMOTE_GPU.md                # joining a remote Azure GPU VM as a Private Node
    ├── STORAGE.md                   # the default local-path StorageClass
    ├── TROUBLESHOOTING.md           # common failure modes and fixes
    └── VOLUMES.md                   # mounting host volumes into node containers
```

| Doc | What it covers |
|---|---|
| [`ENV.md`](local-cluster/ENV.md) | Setting environment variables on node containers via `experimental.docker.env` (all nodes) or `experimental.docker.nodes[].env` (per node). |
| [`K8S_VERSION.md`](local-cluster/K8S_VERSION.md) | Pinning the Kubernetes version via `controlPlane.distro.k8s.image.tag` so every developer's cluster stays in sync. |
| [`LB.md`](local-cluster/LB.md) | vind's built-in `LoadBalancer` support — a dedicated HAProxy container per Service, no MetalLB or cloud integration required. |
| [`MONITORING.md`](local-cluster/MONITORING.md) | Checking node/pod resource usage: `docker stats` (works immediately) vs. `kubectl top` (needs metrics-server, not installed by default). |
| [`NETWORKING.md`](local-cluster/NETWORKING.md) | Customizing `podCIDR` / `serviceCIDR` (e.g. to avoid clashing with a developer's VPN or home network). |
| [`PORTS.md`](local-cluster/PORTS.md) | Exposing node-container ports to the host via `experimental.docker.ports`. |
| [`REMOTE_GPU.md`](local-cluster/REMOTE_GPU.md) | Step-by-step: creating a vind cluster with Private Nodes enabled and joining a remote Azure GPU VM to it. |
| [`STORAGE.md`](local-cluster/STORAGE.md) | The default `local-path` StorageClass vind ships out of the box — when you need it and when you don't. |
| [`TROUBLESHOOTING.md`](local-cluster/TROUBLESHOOTING.md) | Known failure modes (e.g. flannel `subnet.env: no such file or directory`) and how to fix them. |
| [`VOLUMES.md`](local-cluster/VOLUMES.md) | Mounting host paths into node containers via `experimental.docker.volumes`. |

## What is `vind`?

- `vind` is a kind alternative built into the vCluster CLI: it creates
  Kubernetes nodes as plain Docker containers, with no separate cluster
  tool required.
- `experimental.docker.nodes` adds **real, additional worker nodes** (not
  just simulated ones) — confirmed via `kubectl get node` showing the
  control-plane node plus `worker-1`/`worker-2` all `Ready`. This alone
  gets you a genuine multi-node cluster, fully open-source, no license
  needed.

## High availability (out of scope)

vCluster can also run the control plane itself as 3 HA replicas with
clustered embedded etcd (`controlPlane.statefulSet.highAvailability.replicas`
+ `controlPlane.backingStore.etcd.embedded`). **This requires a vCluster
Pro/Enterprise license** — confirmed: `vcluster create --values <file>`
fails with `"you have vCluster pro features enabled, but seems like you are
not logged in..."` unless you've run `vcluster platform login`.

We don't have a Pro license, and HA isn't needed for a local dev cluster, so
`cluster.yaml` intentionally uses the default single-replica control plane
with the default SQLite backing store. It's worth knowing this ceiling
exists in case a future use case needs it.

## Prerequisites

- Docker installed and running
- [vCluster CLI](https://www.vcluster.com/docs/getting-started/setup) installed
- A vCluster Pro/Enterprise license — only needed for HA (see above); not
  required for this setup

## Configuration overview

- **Driver:** `docker` (vind), selected via `vcluster use driver docker`
- **Nodes:** 1 primary (automatic) + 2 extra worker containers
  (`experimental.docker.nodes: worker-1, worker-2`) — real node containers.
- **Distro:** `k8s`
- **Backing store:** default SQLite store, single control-plane pod — no
  license needed.
- **Resources:** control-plane pod requests 200m CPU / 256Mi memory, capped
  at 1 CPU / 1Gi memory. Rough total footprint across all 3 Docker
  containers (control plane + 2 workers) is on the order of 1-2 CPUs and
  2-3GB RAM at idle — check `docker stats` after creation if you need exact
  numbers for your machine.
- **Scheduling:** no `topologySpreadConstraints` is set — only relevant if
  you switch to a multi-replica (Pro/HA) control plane, in which case add a
  `topologySpreadConstraints` block under `controlPlane.statefulSet.scheduling`
  and confirm placement with `docker ps` / `kubectl get pods -o wide -n
  <release>` after creating the cluster.
- **Node sync:** `sync.fromHost.nodes.enabled: false` — flip to `true` if
  workloads inside the vCluster need node affinity/DaemonSets against the
  real vind node containers.

## Getting started

### 1. Select the Docker driver

```bash
vcluster use driver docker
```

### 2. Create the cluster

```bash
vcluster create local-k8s --values cluster.yaml
```

The CLI auto-connects and switches your kubecontext by default.

> **Note — kubeconfig changes:** `vcluster create` auto-runs `connect`
> afterward (`--connect` defaults to `true`), which **merges a new context
> into your existing kubeconfig** (`~/.kube/config` or `$KUBECONFIG`) and
> switches `current-context` to it. It does not overwrite the whole file —
> just adds/updates that one context. Use `vcluster disconnect` to switch
> back, `vcluster create --connect=false` to skip this, or
> `vcluster connect --print` / `--kube-config <path>` to print the context
> or write it elsewhere instead of merging it in. Unlike the kind version,
> there's no separate host-cluster context step here — this is the only
> kubeconfig change that happens.

### 3. Verify

```bash
vcluster list
```

```bash
kubectl config current-context
# expected: vcluster-docker_local-k8s
```

```bash
kubectl get nodes
# expect: control-plane node + worker-1 + worker-2, all Ready
```

To reconnect later in a new shell:

```bash
vcluster connect local-k8s
```

or

```bash
kubectl config get-contexts
kubectl config use-context vcluster-docker_local-k8s
kubectl config current-context
```

### 4. Pause / resume (stop the containers without losing state)

To stop the cluster without deleting it: for the docker driver, `pause`
stops the underlying Docker containers and frees CPU/memory, while
preserving all state — it does not scale down/delete workloads the way it
does on the Helm/kind driver.

```bash
vcluster pause local-k8s
```

Resume it later (back up in seconds, workloads pick up where they left off):

```bash
vcluster resume local-k8s
```

### 5. Adding a node

vind supports in-place changes — no need to delete/recreate the cluster.
Add a new entry under `experimental.docker.nodes` in `cluster.yaml`:

```yaml
experimental:
  docker:
    nodes:
      - name: worker-1
      - name: worker-2
      - name: worker-3   # new
```

Then re-apply with `--upgrade` (required — without it, `vcluster create`
refuses to touch a cluster that already exists under that name):

```bash
vcluster create local-k8s --values cluster.yaml --upgrade
```

Confirm it joined:

```bash
kubectl get node
```

Per-node `env` config (syntax, what it actually does, and which
variables are/aren't confirmed to have any effect) is covered in
[`documentation/ENV.md`](local-cluster/ENV.md).

### 6. Multiple clusters from one file

`cluster.yaml` has no cluster-name field — the name always comes from the
CLI argument — so the same file can be reused across several invocations
to run more than one independent vind cluster at once, each with its own
Docker network and node containers:

```bash
vcluster create local-k8s-a --values cluster.yaml
vcluster create local-k8s-b --values cluster.yaml
```

Only the last one created becomes your active kubectl context; switch
between them with `vcluster connect <name>`. Resource footprint (see
Configuration overview above) multiplies per cluster.

### 7. Delete

```bash
vcluster delete local-k8s
```

This removes the vCluster and its underlying `worker-1`/`worker-2` Docker
containers. Run `docker ps -a` afterwards if you want to confirm nothing was
left behind.


## Terminal Outputs

### Setup

```bash
$ vcluster use driver docker
13:03:55 warn There is a newer version of vcluster: v0.35.2. Run `vcluster upgrade` to upgrade to the newest version.

13:03:55 done Successfully switched driver to docker
```

### Create

```bash
$ vcluster create local-k8s --values cluster.yaml
13:04:43 warn There is a newer version of vcluster: v0.35.2. Run `vcluster upgrade` to upgrade to the newest version.

13:04:43 info Ensuring environment for vCluster local-k8s...
13:04:43 warn Could not load kernel module br_netfilter: exit status 1. If node join fails, run: sudo modprobe overlay && sudo modprobe bridge && sudo modprobe br_netfilter
13:04:43 done Created network vcluster.local-k8s
13:04:46 info Starting vCluster standalone local-k8s
13:04:48 info Waiting for vCluster standalone node to be joined...
13:05:05 done vCluster standalone node joined successfully
13:05:05 info Adding node worker-1 to vCluster local-k8s
13:05:06 info Joining node vcluster.node.local-k8s.worker-1 to vCluster local-k8s...
13:05:11 info Adding node worker-2 to vCluster local-k8s
13:05:14 info Joining node vcluster.node.local-k8s.worker-2 to vCluster local-k8s...
13:05:20 done Successfully created virtual cluster local-k8s
13:05:20 info Finding docker container vcluster.cp.local-k8s...
13:05:20 info Waiting for vCluster kubeconfig to be available...
13:05:20 info Waiting for vCluster to become ready...
13:05:20 done vCluster is ready
13:05:20 done Switched active kube context to vcluster-docker_local-k8s
- Use `vcluster disconnect` to return to your previous kube context
- Use `kubectl get namespaces` to access the vcluster
```

### Verify

```bash
$ vcluster list

      NAME    | STATUS  | CONNECTED |  AGE
  ------------+---------+-----------+--------
    local-k8s | running | True      | 5m17s
```

```bash
$ kubectl config current-context
vcluster-docker_local-k8s
```

```bash
$ kubectl get nodes
NAME        STATUS   ROLES                  AGE     VERSION
local-k8s   Ready    control-plane,master   2m32s   v1.36.0
worker-1    Ready    <none>                 2m26s   v1.36.0
worker-2    Ready    <none>                 2m18s   v1.36.0
```

```bash
$ kubectl get ns
NAME                 STATUS   AGE
default              Active   3m27s
kube-flannel         Active   3m16s
kube-node-lease      Active   3m27s
kube-public          Active   3m27s
kube-system          Active   3m27s
local-path-storage   Active   3m16s
```

### Pause

```bash
$ vcluster list

      NAME    | STATUS  | CONNECTED |  AGE
  ------------+---------+-----------+--------
    local-k8s | running | True      | 6m42s

$ vcluster pause local-k8s
13:11:34 info Pausing vCluster local-k8s...
13:11:35 info Stopping node worker-2 from vCluster local-k8s...
13:11:35 info Stopping node worker-1 from vCluster local-k8s...
13:11:36 done Successfully paused vCluster local-k8s

$ vcluster list

      NAME    | STATUS | CONNECTED |  AGE
  ------------+--------+-----------+--------
    local-k8s | exited | True      | 6m53s

$ kubectl get nodes
The connection to the server localhost:12660 was refused - did you specify the right host or port?
```

### Resume

```bash
$ vcluster list

      NAME    | STATUS | CONNECTED |  AGE
  ------------+--------+-----------+--------
    local-k8s | exited | True      | 7m59s

$ vcluster resume local-k8s
13:12:58 info Resuming vCluster local-k8s...
13:12:58 info Starting node worker-2 from vCluster local-k8s...
13:12:58 info Starting node worker-1 from vCluster local-k8s...
13:12:59 done Successfully resumed vCluster local-k8s

$ vcluster list

      NAME    | STATUS  | CONNECTED |  AGE
  ------------+---------+-----------+--------
    local-k8s | running | True      | 8m21s

$ kubectl get nodes
NAME        STATUS   ROLES                  AGE     VERSION
local-k8s   Ready    control-plane,master   8m6s    v1.36.0
worker-1    Ready    <none>                 8m      v1.36.0
worker-2    Ready    <none>                 7m52s   v1.36.0
```

### Upgrade

```bash
$ grep -A 5 '^experimental:' cluster.yaml
experimental:
  docker:
    nodes:
      - name: worker-1
      - name: worker-2
      - name: worker-3

$ vcluster list

      NAME    | STATUS  | CONNECTED | AGE
  ------------+---------+-----------+------
    local-k8s | running | True      | 15m

$ vcluster create local-k8s --values cluster.yaml --upgrade
13:19:58 warn There is a newer version of vcluster: v0.35.2. Run `vcluster upgrade` to upgrade to the newest version.

13:19:58 info vCluster container local-k8s already exists, recreating it...
13:19:59 info Ensuring environment for vCluster local-k8s...
13:19:59 warn Could not load kernel module br_netfilter: exit status 1. If node join fails, run: sudo modprobe overlay && sudo modprobe bridge && sudo modprobe br_netfilter
13:20:01 info Adding node worker-3 to vCluster local-k8s
13:20:02 info Joining node vcluster.node.local-k8s.worker-3 to vCluster local-k8s...
13:20:24 done Successfully created virtual cluster local-k8s
13:20:24 info Finding docker container vcluster.cp.local-k8s...
13:20:24 info Waiting for vCluster kubeconfig to be available...
13:20:24 info Waiting for vCluster to become ready...
13:20:24 done vCluster is ready
13:20:24 done Switched active kube context to vcluster-docker_local-k8s
- Use `vcluster disconnect` to return to your previous kube context
- Use `kubectl get namespaces` to access the vcluster

$ vcluster list

      NAME    | STATUS  | CONNECTED | AGE
  ------------+---------+-----------+------
    local-k8s | running | True      | 27s

$ kubectl get nodes
NAME        STATUS     ROLES                  AGE   VERSION
local-k8s   Ready      control-plane,master   15m   v1.36.0
worker-1    Ready      <none>                 15m   v1.36.0
worker-2    Ready      <none>                 15m   v1.36.0
worker-3    NotReady   <none>                 5s    v1.36.0

$ kubectl get nodes
NAME        STATUS   ROLES                  AGE   VERSION
local-k8s   Ready    control-plane,master   16m   v1.36.0
worker-1    Ready    <none>                 16m   v1.36.0
worker-2    Ready    <none>                 16m   v1.36.0
worker-3    Ready    <none>                 80s   v1.36.0
```

### Delete

```bash
$ vcluster list

      NAME    | STATUS  | CONNECTED | AGE
  ------------+---------+-----------+-------
    local-k8s | running | True      | 3m1s

$ vcluster delete local-k8s
13:23:07 info Removing vCluster container vcluster.cp.local-k8s...
13:23:08 info Removing vCluster node worker-3...
13:23:09 info Removing vCluster node worker-2...
13:23:09 info Removing vCluster node worker-1...
13:23:10 info Deleted kube context vcluster-docker_local-k8s
13:23:10 done Successfully deleted virtual cluster local-k8s

$ vcluster list

    NAME | STATUS | CONNECTED | AGE
  -------+--------+-----------+------

$ kubectl config use-context vcluster-docker_local-k8s
error: no context exists with the name: "vcluster-docker_local-k8s"
```

## Docker container mapping

Every Kubernetes node in `cluster.yaml` is one real Docker container, all
running the same `ghcr.io/loft-sh/vm-container` image with `/entrypoint.sh`.
Naming follows `vcluster.<role>.<cluster-name>[.<node-name>]`:

- **`vcluster.cp.<cluster-name>`** — the control-plane node. Only this
  container publishes a port to the host: `<random-port>:8443`, the
  Kubernetes API server — this is what your kubeconfig actually points at
  after `vcluster connect`.
- **`vcluster.node.<cluster-name>.<worker-name>`** — one per entry under
  `experimental.docker.nodes` (`worker-1`, `worker-2`, `worker-3`). No
  ports published by default; use `experimental.docker.nodes[].ports` (see
  [`documentation/PORTS.md`](local-cluster/PORTS.md)) to expose one.

This mapping is why `docker exec vcluster.cp.<name> ...` / `docker exec
vcluster.node.<name>.<worker> ...` shows up throughout the other docs
(`documentation/ENV.md`, `documentation/TROUBLESHOOTING.md`) — it's the
one debugging entry point that works even when the Kubernetes API itself
is unreachable.

```bash
$ docker ps
CONTAINER ID   IMAGE                          COMMAND            CREATED       STATUS       PORTS                                           NAMES
96a1075b352e   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   2 hours ago   Up 2 hours                                                   vcluster.node.local-k8s.worker-3
f3cba5829f44   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   2 hours ago   Up 2 hours                                                   vcluster.node.local-k8s.worker-2
61e7dd1a8ed6   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   2 hours ago   Up 2 hours                                                   vcluster.node.local-k8s.worker-1
34dea50e8187   ghcr.io/loft-sh/vm-container   "/entrypoint.sh"   2 hours ago   Up 2 hours   0.0.0.0:10354->8443/tcp, [::]:10354->8443/tcp   vcluster.cp.local-k8s
```

To check actual resource usage per container (the Configuration overview
above only gives a rough estimate):

```bash
$ docker stats --no-stream vcluster.cp.local-k8s vcluster.node.local-k8s.worker-1 vcluster.node.local-k8s.worker-2 vcluster.node.local-k8s.worker-3
```
