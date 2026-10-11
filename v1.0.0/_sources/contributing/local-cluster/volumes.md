---
title: "Local cluster: Volumes"
description: "Local development cluster - Volumes."
weight: 100
---

# Local cluster: Volumes

Reference: [vCluster in Docker (vind) — Examples](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml/experimental/docker#examples)

## Syntax

`experimental.docker.nodes[].volumes` accepts standard Docker bind-mount
syntax (`host-path:container-path`), scoped to one node. There's also a
cluster-wide `experimental.docker.volumes` that applies the same mounts to
every node container. Example from the docs:

```yaml
experimental:
  docker:
    nodes:
      - name: "worker-2"
        volumes:
          - "/host/data:/data"
```

## What it actually does

This is a plain Docker bind mount (`docker run -v host-path:container-path`)
— the host directory's contents appear live inside the container's
filesystem at the given path. It's two-way and immediate, not a copy: file
changes on either side are visible on the other without any sync step.

Like `env` (see [`documentation/ENV.md`](env.md)), this is a **Docker-level
construct, not a Kubernetes one** — it has nothing to do with a
`PersistentVolume`/`PersistentVolumeClaim`, and it's unrelated to
`sync.fromHost.nodes` (the vcluster config flag covered in the README's
Configuration overview, which syncs Kubernetes `Node` API objects between
host and virtual cluster — a completely different feature that happens to
share the word "host").

## Confirmed working — verified against a live vind container

Unlike the `DEBUG`/`LOG_LEVEL` env example (unconfirmed, see `ENV.md`),
this mechanism is provably in active use by vind itself.
`docker inspect vcluster.node.local-k8s.worker-1 --format '{{json .Mounts}}'`
against a real running cluster shows vind bootstrapping the node with its
own bind mounts:

```json
{"Type":"bind","Source":"/home/<user>/.vcluster/docker/kubernetes/v1.36.0/kubernetes-v1.36.0-amd64.tar.gz","Destination":"/var/lib/vcluster/bin/kubernetes-v1.36.0-amd64.tar.gz","RW":false}
{"Type":"bind","Source":"/home/<user>/.vcluster/docker/vclusters/local-k8s/k8s-resolv.conf","Destination":"/etc/k8s-resolv.conf","RW":false}
{"Type":"bind","Source":"/home/<user>/.vcluster/docker/vclusters/local-k8s/kubelet.env","Destination":"/etc/vcluster/vcluster-flags.env","RW":false}
```

Plus several named Docker volumes (not bind mounts, but the same
mechanism family) for `/etc`, `/usr/local/bin`, `/opt/cni/bin`, and `/var`
— this is how vind persists node state (installed binaries, CNI config)
independently of the container's own writable layer, which is also why
`vcluster pause`/`resume` can stop and restart the container without
losing that state.

## Practical uses

- **Local model/dataset directories** — mount a host directory with model
  weights or test fixtures directly into a node, useful for exercising
  `app_serving` without round-tripping everything through Harbor first.
- **Faster edit-test loops** — mount a host build/output directory so
  files produced on the laptop are immediately visible inside the
  container (and vice versa), without `kubectl cp` or rebuilding images.
- **Shared credentials/config** — mount host-side SSH keys, `.netrc`, or
  registry auth files read-only into a node if something running inside
  needs them for private git/registry access.

## Caveats

- The host path must exist (or Docker will create it as an empty
  directory) before the container starts — bind mounts don't fail
  silently if missing, they just mount an empty directory.
- Mount propagation is `rprivate` (confirmed via `docker inspect` above) —
  mounts created inside the container *after* boot won't appear on the
  host and vice versa; only what's mounted at container-create time is
  shared.
- File ownership/permissions follow the host side as-is; the container's
  processes (kubelet, containerd, etc.) run as whatever UID the image
  uses, so a host directory owned by your user may need permissive enough
  permissions for the container to read/write it.

## Worked example: mounting local LLM model weights for `app_serving`

`experimental.docker.nodes[].volumes` only gets a host directory onto the
**node container's** filesystem. Since that container *is* the Kubernetes
node from kubelet's perspective, a Pod scheduled on it can see that path
too — but only via an explicit Kubernetes `hostPath` volume, and only if
the Pod is guaranteed to land on that specific node. This is a two-layer
setup, not something `cluster.yaml` gives you for free.

This bypasses `app_serving`'s normal model-loading path (`modelSource` →
PVC + ORAS pull Job from Harbor, see the root `CLAUDE.md`). Useful for
local dev iteration where re-pulling a multi-GB model from Harbor on every
`pulumi up` is the bottleneck; not a substitute for the real path in any
shared/CI environment.

### 1. Bind-mount the host models directory onto a specific worker

```yaml
# cluster.yaml
experimental:
  docker:
    nodes:
      - name: worker-1
        volumes:
          - "/home/you/models/gemma-4:/mnt/models/gemma-4"
      - name: worker-2
      - name: worker-3
```

Apply with `vcluster create local-k8s --values cluster.yaml --upgrade`
(see README.md step 5, "Adding a node" — same `--upgrade` requirement
applies to any `cluster.yaml` change on an existing cluster).

### 2. Verify the mount landed on the right node container

```bash
docker exec vcluster.node.local-k8s.worker-1 ls /mnt/models/gemma-4
```

### 3. Create a static-provisioning StorageClass

Mirrors the on-prem module's pattern
(`infra/on-prem/pulumi/infra/components/storageclass/storageclass.go`) —
`no-provisioner` means Kubernetes won't try to dynamically create the
volume; you're supplying it yourself.

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: local-models
provisioner: kubernetes.io/no-provisioner
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Retain
```

### 4. Create a PersistentVolume pinned to that node via `nodeAffinity`

The `hostPath` here is the path *inside the node container* from step 1
(`/mnt/models/gemma-4`), not the original host path. `worker-1` is the
exact node name as `kubectl get nodes` shows it.

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: pv-gemma-4
spec:
  capacity:
    storage: 20Gi
  accessModes:
    - ReadOnlyMany
  persistentVolumeReclaimPolicy: Retain
  storageClassName: local-models
  hostPath:
    path: /mnt/models/gemma-4
  nodeAffinity:
    required:
      nodeSelectorTerms:
        - matchExpressions:
            - key: kubernetes.io/hostname
              operator: In
              values:
                - worker-1
```

### 5. Create a matching PersistentVolumeClaim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc-gemma-4
spec:
  accessModes:
    - ReadOnlyMany
  storageClassName: local-models
  resources:
    requests:
      storage: 20Gi
  volumeName: pv-gemma-4
```

### 6. Point `app_serving`'s model config at the PVC

Whatever consumes model weights in `app_serving` (the `modelservice`
component) needs to reference `pvc-gemma-4` instead of going through the
`modelSource` → ORAS pull path. Check the specific stack's model config
under `app_serving/deployments/models/<ModelName>/` for the exact field —
this varies per model definition, so there's no one-line snippet here;
treat `pvc-gemma-4` as a drop-in replacement for whatever PVC the ORAS Job
would otherwise have created.

### Quick sanity check without `app_serving` at all

Before wiring this into a real model-serving stack, confirm the PV/PVC
chain works with a throwaway pod:

```bash
kubectl run model-check --image=busybox --restart=Never \
  --overrides='{"spec":{"nodeSelector":{"kubernetes.io/hostname":"worker-1"},"containers":[{"name":"model-check","image":"busybox","command":["ls","/models"],"volumeMounts":[{"name":"models","mountPath":"/models"}]}],"volumes":[{"name":"models","persistentVolumeClaim":{"claimName":"pvc-gemma-4"}}]}}' \
  -- ls /models

kubectl logs model-check
kubectl delete pod model-check
```

`nodeSelector` here is fine for a disposable sanity-check pod — it's the
simplest way to say "must land on `worker-1`." For the real Pod spec
wired into `app_serving` in step 6, prefer Pod-level `affinity.nodeAffinity`
instead: it supports `In`/`NotIn`/`Exists` operators, matching against
multiple values, and `preferredDuringSchedulingIgnoredDuringExecution`
(soft preference, falls back instead of failing to schedule) —
`nodeSelector` only supports exact-match `AND` on a fixed label set, so
`nodeAffinity` is the more production-grade choice once this moves past a
quick local check.

Note this is a different `nodeAffinity` from step 4's — that one is on the
`PersistentVolume` itself and is mandatory (`PersistentVolume.spec` has no
`nodeSelector` field at all; `nodeAffinity` is the only way to pin a
static PV to a node). The one described here is on the **Pod**, and is a
choice between two valid options, not a hard requirement.
