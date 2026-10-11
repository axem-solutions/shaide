---
title: "Local cluster: Env"
description: "Local development cluster - Env."
weight: 10
---

# Local cluster: Env

Reference: [vCluster in Docker (vind) — Examples](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml/experimental/docker#examples)

## Syntax

`experimental.docker.nodes[].env` accepts the same `key=value` format as
the top-level `experimental.docker.env`, e.g.:

```yaml
experimental:
  docker:
    nodes:
      - name: "worker-2"
        env:
          - "NODE_ROLE=worker"
```

## What it actually does

These are Docker container-level env vars (`docker run/create -e
KEY=VALUE`) injected into that specific node's container at creation
time — not a Kubernetes-level construct (not a Pod env, not a ConfigMap,
not read by kubelet automatically). They live in the OS environment of the
container running that node's kubelet/containerd, and are visible via
`docker exec <container> env`.

There's no vCluster-specific parsing or validation of the key names —
anything set here just becomes a plain OS environment variable inside the
container. `NODE_ROLE=worker` in the example above isn't consumed by
Kubernetes itself; it's illustrative of per-node customization (as opposed
to the cluster-wide `experimental.docker.env`, which applies the same vars
to every node container). Anything reading that variable would have to be
something running inside the container itself: a custom entrypoint script,
a sidecar process, or you manually checking it via
`docker exec vcluster.node.<cluster>.worker-2 env`.

## Caveat: `--upgrade` doesn't apply `env` changes to existing nodes

This is a **Docker-level** constraint, not a vcluster limitation: env vars
are baked into a container at `docker create`/`docker run` time and can't
be injected into an already-running container. If a node already existed
before you added/changed its `env:` entry in `cluster.yaml`, running
`vcluster create --values cluster.yaml --upgrade` will **not** update that
node's container — `--upgrade` only appears to create nodes that are
*missing*, not recreate ones that already exist to pick up config changes.

Confirmed on a live cluster: adding `NODE_ROLE=worker` to an
already-existing `worker-1`/`worker-2`, then running `--upgrade`, left
`docker exec vcluster.node.local-k8s.worker-1 env` showing no `NODE_ROLE`
at all — while a genuinely *new* node added the same way (see README.md
step 5, "Adding a node") does pick up its `env:` correctly, because it's
created fresh.

To force an existing node to pick up an `env`/`volumes` change:

```bash
# targeted (unverified — may leave a stale Node object needing
# `kubectl delete node <name>` if it doesn't cleanly re-register):
docker rm -f vcluster.node.local-k8s.worker-1
vcluster create local-k8s --values cluster.yaml --upgrade

# guaranteed (recreates every container from the current cluster.yaml):
vcluster delete local-k8s
vcluster create local-k8s --values cluster.yaml
```

## Caveat: `DEBUG`/`LOG_LEVEL` are not confirmed to do anything for vind

The `DEBUG=true` / `LOG_LEVEL=info` pair shown as an example in vCluster's
docker/vind docs are **not confirmed to have any effect** in this
deployment mode — they're just the two names the docs happened to pick to
illustrate the `env:` field syntax. Whether setting them changes anything
depends entirely on whether some process inside that container (the
`vm-container` image's entrypoint, k8s distro binaries, kubelet,
containerd) happens to read that specific variable name, and nothing in
vCluster's documentation confirms that for `vind`.

This is different from vCluster's **Helm-based** deployment mode, where
`controlPlane.statefulSet.env: [{name: DEBUG, value: "true"}]` is a
documented, confirmed mechanism (see
[Enable debug logging in the control plane](https://www.vcluster.com/docs/vcluster/learn-how-to/control-plane/container/enable-debug-logging)) —
that one sets the env on the actual vcluster syncer process's container,
and that process is documented to check `DEBUG` itself. `LOG_LEVEL` isn't
mentioned anywhere in that guide either. `experimental.docker.env` (vind)
and `controlPlane.statefulSet.env` (Helm) are two unrelated mechanisms
that happen to share a field name.

## Practical recommendations

Given the above, don't add speculative env vars to `cluster.yaml` just
because the field exists — unverified vars add config noise without a
confirmed effect. Two exceptions are grounded in generic Linux/Go runtime
conventions rather than anything vind-specific, so they're a safer bet:

- **`TZ`** (e.g. `TZ=Europe/Budapest`) — **high confidence**. Respected by
  glibc and virtually every language runtime for local-time formatting.
  Makes `docker logs`/container-internal timestamps match your laptop's
  clock instead of UTC — useful when correlating logs with your own
  actions during debugging. Baked into libc, unrelated to vind.
- **`HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`** — **medium-high
  confidence**, only relevant if your laptop sits behind a corporate
  proxy for outbound traffic. containerd, kubelet, and kube-apiserver are
  all Go binaries, and Go's standard `net/http` client honors these vars
  via `ProxyFromEnvironment` by default. Not vind-documented, but this is
  generic Go/Linux behavior that vind doesn't need to specifically opt
  into.

**Not worth adding:**

- `DEBUG` / `LOG_LEVEL` — unconfirmed, purely illustrative in the docs (see
  Caveat above).
- `NODE_ROLE` — does nothing unless you write custom tooling that reads it
  via `docker exec`.

Only add `HTTP_PROXY`/`TZ` if you have a concrete need (image pulls
actually failing behind a proxy, UTC timestamps actually being annoying) —
not preemptively.

## Testing whether a variable actually does anything

Since nothing is officially confirmed for the vind image, test directly:

```bash
docker exec vcluster.cp.local-k8s env | grep -E "DEBUG|LOG_LEVEL"
```

then compare `docker logs vcluster.cp.<name>` verbosity before/after
recreating the cluster with the var added to `cluster.yaml`.
