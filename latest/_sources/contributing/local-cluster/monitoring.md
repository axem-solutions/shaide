---
title: "Local cluster: Monitoring"
description: "Local development cluster - Monitoring."
weight: 40
---

# Local cluster: Monitoring

vind nodes are just Docker containers, so container-level usage is always
available with no extra install. Kubernetes-level usage (`kubectl top`)
needs a metrics-server, which isn't part of this cluster by default — the
`kubectl get ns` output in `README.md` has no metrics-server namespace.

## `docker stats` — live container CPU/memory (works immediately)

```bash
docker stats
```

No arguments shows a live-updating table for **every** container on the
Docker host, not just this cluster's — useful for a first look, noisy if
you have other containers running.

```bash
docker stats vcluster.cp.local-k8s vcluster.node.local-k8s.worker-1 vcluster.node.local-k8s.worker-2 vcluster.node.local-k8s.worker-3 --no-stream
```

Scoped to just this cluster's four containers (control plane + 3 workers —
see `README.md`'s Docker container mapping section for the naming
convention), and `--no-stream` prints a single snapshot instead of a
live-updating view — easier to copy into a bug report or compare
before/after a change.

This reads straight from the Docker host's cgroups, so it's accurate
regardless of what's running inside the vcluster and needs nothing
installed in-cluster.

## `kubectl top` — Kubernetes-level view (needs metrics-server)

`kubectl top` reads from the Metrics API, which nothing in this cluster
serves by default. Install metrics-server once per cluster:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

Then:

```bash
kubectl top node
```

Per-node CPU/memory usage as Kubernetes sees it (via kubelet's cAdvisor
stats) — same underlying numbers as `docker stats`, but expressed against
each node's allocatable capacity rather than raw host resources.

```bash
kubectl top pod -n <namespace>
```

Per-pod CPU/memory within a namespace — e.g. `-n llm-d-sim-local-gen` to
check a model's decode/prefill pods, or `-n app-shaide` for the Shaide
stack. Without `-n`, only the `default` namespace is shown; add `-A` for
every namespace at once.

**When to use which**: `docker stats` for a quick, dependency-free check
of the whole cluster's footprint on your laptop; `kubectl top` once
metrics-server is installed, when you need per-pod attribution inside a
specific namespace rather than per-container totals.
