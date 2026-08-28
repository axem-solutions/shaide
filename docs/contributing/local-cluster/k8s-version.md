---
title: "Local cluster: K8s version"
description: "Local development cluster - K8s version."
weight: 20
---

# Local cluster: K8s version

## Setting the version

Use `controlPlane.distro.k8s.image.tag`:

```yaml
controlPlane:
  distro:
    k8s:
      enabled: true
      image:
        tag: v1.35.0   # specify the Kubernetes version
```

`image.registry`/`image.repository` default to `ghcr.io`/`loft-sh/kubernetes`
and don't need to be set unless pointing at a mirror. Omitting `image.tag`
falls back to whatever vCluster's current default is — `v1.36.0` as of
this writing, but that default moves forward as new vCluster versions
ship. Don't treat it as fixed; check the actual value on a live cluster
with `kubectl version` rather than trusting this doc, and pin `image.tag`
explicitly if you need reproducibility instead of relying on the default.

## Confirmed working — tested both pinned and default

Verified directly against a live cluster by recreating with two different
tags and checking `kubectl get node -o wide`:

**Pinned to `v1.35.0`:**

```bash
controlPlane:
  distro:
    k8s:
      enabled: true
      image:
        tag: v1.35.0
```

```bash
$ kubectl get node -o wide
NAME        STATUS   ROLES                  AGE    VERSION   CONTAINER-RUNTIME
local-k8s   Ready    control-plane,master   107s   v1.35.0   containerd://2.1.6
worker-1    Ready    <none>                 101s   v1.35.0   containerd://2.1.6
worker-2    Ready    <none>                 94s    v1.35.0   containerd://2.1.6
worker-3    Ready    <none>                 85s    v1.35.0   containerd://2.1.6
```

**Default at time of testing (`tag` omitted, resolved to `v1.36.0` — this
will drift as vCluster ships new releases, don't assume it still applies):**

```bash
$ kubectl get node -o wide
NAME        STATUS   ROLES                  AGE   VERSION   CONTAINER-RUNTIME
local-k8s   Ready    control-plane,master   28m   v1.36.0   containerd://2.2.3
worker-1    Ready    <none>                 28m   v1.36.0   containerd://2.2.3
worker-2    Ready    <none>                 28m   v1.36.0   containerd://2.2.3
worker-3    Ready    <none>                 28m   v1.36.0   containerd://2.2.3
```

`VERSION` matches the configured tag exactly on every node, control plane
included — pinning genuinely applies cluster-wide, not just to the API
server.

## It's not just the Kubernetes version — the whole node image changes

Notice `CONTAINER-RUNTIME` differs too: `containerd://2.1.6` on `v1.35.0`
vs `containerd://2.2.3` on `v1.36.0`. Each `image.tag` is a full node
image bundling a specific containerd build alongside that Kubernetes
version — pinning the tag pins more than just `kubectl version` output,
it pins the whole node runtime stack. Worth knowing if you're chasing a
containerd-specific bug or behavior difference, not just a Kubernetes API
version.

One more quirk observed during testing: the CLI log for a `v1.35.0` pin
shows `Pulling image ghcr.io/loft-sh/kubernetes:v1.35.0-full` — vind
appends a `-full` suffix to whatever tag you specify internally; you don't
need to (and shouldn't) write `-full` yourself in `cluster.yaml`.

## Why pin at all

Without a pin, every developer's cluster resolves to whatever the current
default tag is at the moment they run `vcluster create` — that default
changes over time as vCluster releases new versions. Pinning keeps every
developer's local-k8s cluster on the identical Kubernetes version, so
"works on my machine" doesn't come down to who created their cluster more
recently.

## Caveat: requires full cluster recreation

Same as `networking.podCIDR`/`serviceCIDR` (see `NETWORKING.md`) — the
node image is baked in at container-creation time, so changing the pinned
version requires a full recreate, not `--upgrade`:

```bash
vcluster delete local-k8s
vcluster create local-k8s --values cluster.yaml
```
