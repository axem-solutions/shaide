---
title: "Node registry trust"
description: "Configuring cluster nodes to trust the internal registry."
weight: 40
---

# Node registry trust

How cluster nodes pull images from Harbor, and why a ClusterIP-only Service is reachable
from processes that are not pods.

## The problem

Harbor's Service is ClusterIP-only — by definition only reachable from inside the pod
network. Yet `kubelet`/`containerd`, which run directly on the node's host OS, pull images
from it successfully. Two separate steps make this work.

## Step 1 — writing `hosts.toml` (no network)

The `node-registry-config` DaemonSet runs a `busybox` container that mounts the node's
`/etc/containerd/certs.d` via `hostPath` and writes a config file there:

```
                         node filesystem
                      ┌────────────────────────────────┐
                      │ /etc/containerd/certs.d/       │
                      │   harbor.harbor.svc.cluster... │
  ┌───────────────┐   │     /hosts.toml  ◄──────────┐  │
  │  busybox      │   │                             │  │
  │  (DaemonSet   │───┼── hostPath mount ───────────┘  │
  │   pod)        │   │   (bind mount, not network)    │
  └───────────────┘   └────────────────────────────────┘
        │
        └─ writes file, then either idles (`sleep infinity`) or loops back and
           rewrites the same file on an interval — the container script's choice,
           not something a network call decides
```

No network call is involved. `${HARBOR_IP}` is a literal value substituted when the manifest
is applied; the write itself is a local bind-mount operation. One DaemonSet pod per node
produces one `hosts.toml` per node's disk. Whichever variant is deployed, this pod never
participates in step 2 below — an image pull is never routed through it.

## Step 2 — pulling an image (this is where the network happens)

Later, `containerd` — not a pod, a daemon running directly on the host OS, in the node's
**root network namespace**, the same namespace `kubelet` and `kube-proxy` occupy — reads that
file and uses `http://<HARBOR_IP>` instead of resolving the hostname over HTTPS.

```
 ┌─────────────────────────────── node (root network namespace) ──────────────────────────────────┐
 │                                                                                                │
 │   kubelet ──"pull image X"──► containerd                                                       │
 │                                   │                                                            │
 │                                   │ reads /etc/containerd/certs.d/harbor..../hosts.toml        │
 │                                   │ → use http://<HARBOR_IP> (ClusterIP)                       │
 │                                   ▼                                                            │
 │                          GET http://<HARBOR_IP>/v2/...                                         │
 │                                   │                                                            │
 │                                   ▼                                                            │
 │                    iptables / IPVS rules  ◄── programmed here by kube-proxy                    │
 │                    (DNAT: ClusterIP → real pod IP)                                             │
 │                                   │                                                            │
 └───────────────────────────────────┼────────────────────────────────────────────────────────────┘
                                     │  (routed, not a bind-mounted file — real packets)
                                     ▼
                     ┌──────────────────────────────────┐
                     │   harbor pod (some node)         │
                     │   10.x.x.x:8080 (actual pod IP)  │
                     └──────────────────────────────────┘
```

## Why a non-pod process can reach a ClusterIP

A ClusterIP is not bound to anything; it exists only as a set of DNAT rules. `kube-proxy`
installs those rules into the **root network namespace of every node**, not into a
pod-exclusive sandbox.

```
        pod netns                         root netns (the node itself)
   ┌────────────────┐                 ┌─────────────────────────────────┐
   │  app container │─── veth ───────►│                                 │
   └────────────────┘                 │   iptables / IPVS               │
                                      │   KUBE-SERVICES chain           │
   ┌────────────────┐                 │   (DNAT ClusterIP → pod IP)     │
   │  containerd    │────────────────►│                                 │
   └────────────────┘   (already here,│                                 │
                         no veth)     └─────────────────────────────────┘
```

A pod reaches a ClusterIP by routing through the root netns via its veth pair. `containerd`
is already running in that same root netns, so it hits the identical DNAT rules directly.
From `kube-proxy`'s point of view there is no difference between the two.

## Why the DaemonSet tolerates every taint

`tolerations: [{operator: Exists}]` schedules it on every node, tainted or not — because a
pull can happen on any node at any time. A node missing `hosts.toml` falls back to
containerd's default HTTPS resolution, which fails: Harbor runs HTTP-only, no TLS.

## Summary

| Step | Actor | Namespace | Network call |
|---|---|---|---|
| Write `hosts.toml` | `busybox` (DaemonSet) | pod netns | No |
| Pull image | `containerd` | node root netns | Yes — DNAT'd by `kube-proxy` to a Harbor pod |

The DaemonSet's job ends at "file exists on disk." The network call that actually reaches
Harbor is made later, by a different process that happens to already run in the namespace
where ClusterIPs are routable.
