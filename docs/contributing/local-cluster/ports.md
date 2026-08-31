---
title: "Local cluster: Ports"
description: "Local development cluster - Ports."
weight: 60
---

# Local cluster: Ports

Reference: [vCluster in Docker (vind) — Examples](https://www.vcluster.com/docs/vcluster/configure/vcluster-yaml/experimental/docker#examples)

vind exposes three separate port-mapping mechanisms:

1. **`experimental.docker.ports`** (cluster-wide) — maps container ports to
   host ports, e.g.:
   ```yaml
   experimental:
     docker:
       ports:
         - "8080:80"
         - "8443:443"
   ```
2. **`experimental.docker.nodes[].ports`** — same mapping, scoped to one
   specific worker node, e.g.:
   ```yaml
   experimental:
     docker:
       nodes:
         - name: "worker-1"
           ports:
             - "9090:9090"
   ```
3. **`experimental.docker.loadBalancer.forwardPorts`** — auto-forwards
   `LoadBalancer` Service IPs to the host. Mainly needed on macOS/Docker
   Desktop, where the bridge IP isn't routable from the host; on Linux this
   isn't required (see the built-in LoadBalancer support already covered
   in `README.md`).

## Benefits of explicit port mappings (#1 / #2)

- **Deterministic host access without `kubectl port-forward`.** Docker maps
  the port once at container-create time; you get a stable `localhost:<port>`
  reachable from a browser/curl/script indefinitely, instead of a
  `kubectl port-forward` session that dies when the terminal closes.
- **Bypasses Kubernetes Service/NodePort entirely.** The mapping happens at
  the Docker layer, so it works even for things with no Kubernetes Service
  in front of them — e.g. a raw debug/metrics endpoint on a container, or
  kubelet-level diagnostics.
- **Node-level targeting.** Per-node `ports` (#2) pin exposure to one
  specific worker — useful when testing scheduling/affinity and confirming
  a workload landed on `worker-1` specifically, by hitting that node's
  dedicated port rather than a Service that could route anywhere.
- **Avoids the ephemeral NodePort range (30000-32767).** You choose the
  exact host port instead of Kubernetes picking one from that range, which
  matters for tooling/scripts that expect a fixed, memorable port.
- **Collision control across multiple concurrent vind clusters.** Relevant
  to the multi-cluster pattern in `README.md` — if two clusters both want
  to expose the same container port, dedicated host-side ports let you
  assign each cluster a distinct one instead of them fighting over the same
  host port.
