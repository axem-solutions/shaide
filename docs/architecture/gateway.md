---
title: "Gateway and routing"
description: "Istio, Gateway API and the inference gateway that routes requests to model replicas."
weight: 30
---

# Gateway and routing

## Overview

The `gateway-provider` Pulumi stack is responsible for installing the networking foundation
that all model-serving workloads depend on. It performs three distinct jobs:

1. **Install CRDs** — Gateway API CRDs and Gateway API Inference Extension (GIE) CRDs must be
   present in the cluster before any Gateway or InferencePool resource can be created.
2. **Install Istio** — Deploys `istio-base` and `istiod` via Helm, enabling Istio as the
   Gateway API implementation.
3. **Create shared Gateway** — When `infraStackRef` (cloud) or `gatewayHostname` (on-prem) is
   configured, creates a `shared-gateway` resource in `gateway-system` backed by a
   cloud-managed or Istio/MetalLB load balancer. `app_shaide` and other stacks attach
   `HTTPRoute` resources to this gateway.

### Why a shared Gateway instead of a LoadBalancer Service per workload?

Without a shared gateway, each workload (e.g. `app_shaide`) would need its own `LoadBalancer`
Service. On GCP this means a separate L4 Network Load Balancer per service — each with its own
external IP, its own GCP LB resource, and its own cost.

The shared gateway consolidates all external traffic behind a single entry point:

| | LoadBalancer per Service | Shared Gateway |
|---|---|---|
| GCP LB resources | One per service | One for all services |
| External IPs | One per service | One static IP |
| TLS certificate | One per service | One GKE-managed cert |
| Cost | Scales with number of services | Fixed |

On-prem the same principle applies — one MetalLB IP and one Istio proxy instead of one per
service. The goal on on-prem is to mirror the cloud infrastructure as closely as possible,
which is why `shared-gateway` is deployed on RKE2 as well.

---

## CRD Installation

CRDs are installed via `kustomize.NewDirectory` — the kustomize rendering engine is embedded
in the Pulumi Kubernetes provider. Each CRD is tracked as an individual Pulumi resource,
enabling drift detection and eliminating the `kubectl` binary dependency.

### SSA field-manager conflicts (GCP)

GKE clusters pre-install Gateway API CRDs via `kube-addon-manager`, which holds ownership of
fields such as `metadata.annotations.gateway.networking.k8s.io/bundle-version` and
`spec.versions`. Pulumi's Server-Side Apply would conflict with this ownership.

A `patchForceCRDs` transform is applied to both kustomize directories. It injects the
`pulumi.com/patchForce: "true"` annotation on every CRD resource, equivalent to
`kubectl apply --force-conflicts`. Pulumi takes ownership of the conflicting fields without
an error. On RKE2, where no prior owner exists, the annotation has no effect.

```
kustomize.NewDirectory
    └── transform: patchForceCRDs
            └── sets metadata.annotations["pulumi.com/patchForce"] = "true"
                on every kubernetes:apiextensions.k8s.io/v1:CustomResourceDefinition
```

---

## Traffic Flows

### GCP

```
Client
    │
    ▼
GKE L7 LB (203.0.113.10)
    │
    ▼
shared-gateway  [gateway-system]        ← gateway-provider
    │
    └── HTTPRoute                       ← app_shaide
            └──▶ shaide-server (ClusterIP)  [app-shaide ns]
                     │
                     └──▶ infra-<model>-inference-gateway-istio (in-cluster FQDN)
                              │
                              ▼
                      per-model Gateway (istio, ClusterIP)   ← app_serving (llm-d-infra)
                              │
                              └── HTTPRoute                  ← app_serving
                                      └──▶ InferencePool
                                               └──▶ model pods
```

### On-prem (RKE2)

```
Client
    │
    ▼
MetalLB L2 (DNS: shaide.internal.lan)
    │
    ▼
shared-gateway-istio Service (LoadBalancer)  [gateway-system]
    │
    ▼
Istio Envoy proxy
    │
    ▼
shared-gateway  [gateway-system]             ← gateway-provider
    │
    └── HTTPRoute                            ← app_shaide
            └──▶ shaide-server (ClusterIP)  [app-shaide ns]
                     │
                     └──▶ infra-<model>-inference-gateway-istio (in-cluster FQDN)
                              │
                              ▼
                      per-model Gateway (istio, ClusterIP)   ← app_serving (llm-d-infra)
                              │
                              └── HTTPRoute                  ← app_serving
                                      └──▶ InferencePool
                                               └──▶ model pods
```

### Azure — AGC

```
Client
    │
    ▼
AGC frontend (*.alb.azure.com)
    │
    ▼
shared-gateway  [gateway-system]             ← gateway-provider
    │
    └── HTTPRoute                            ← app_shaide
            └──▶ shaide-server (ClusterIP)  [app-shaide ns]
                     │
                     └──▶ infra-<model>-inference-gateway-istio (in-cluster FQDN)
                              │
                              ▼
                      per-model Gateway (istio, ClusterIP)   ← app_serving (llm-d-infra)
                              │
                              └── HTTPRoute                  ← app_serving
                                      └──▶ InferencePool
                                               └──▶ model pods
```

- Azure-managed L7 load balancer — Microsoft runs the infrastructure outside the cluster
- AKS provides the `azure-alb-external` GatewayClass built-in
- AGC assigns its own stable hostname (`*.alb.azure.com`); DNS uses CNAME, not A record
- No static IP needed — the hostname is stable
- Requires `Microsoft.ServiceNetworking/trafficControllers`
- The `ApplicationLoadBalancer` CR + delegated ALB subnet (`snet-alb-<stack>`) connect AGC to the cluster
- Zero-ops: Microsoft manages scaling, patching, and availability

### Azure — Istio

**Not supported.** AGC is the only supported gateway implementation on AKS. Istio remains
installed for inference routing (the per-model gateways above), but it is not used as the
shared ingress gateway on Azure.

Downstream stacks (`app_shaide`, `app_serving`) are unaffected — the `Gateway` and `HTTPRoute`
resources are identical regardless of which implementation backs them.

**Same Gateway API spec, different infrastructure behind it.**

GKE's gateway controller provisions a Google Cloud L7 load balancer externally — no in-cluster
Service is created. The IP is a GCP resource, not a Kubernetes Service.

Istio's gateway controller creates a physical `Deployment` + `LoadBalancer` Service in-cluster.
MetalLB handles L2 ARP to advertise the IP on the LAN.

```bash
# GCP — no in-cluster Service:
$ kubectl get svc -n gateway-system
No resources found in gateway-system namespace.

$ kubectl get gateway -n gateway-system
NAME             CLASS                              ADDRESS          PROGRAMMED
shared-gateway   gke-l7-regional-external-managed   203.0.113.10   True

# On-prem — Istio creates a backing Service:
$ kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get svc -n gateway-system
NAME                   TYPE           CLUSTER-IP      EXTERNAL-IP    PORT(S)
shared-gateway-istio   LoadBalancer   10.43.100.50   10.0.10.50   15021:31238/TCP,80:30418/TCP

$ kubectl --kubeconfig ~/.kube/rke2-cluster.yaml get gateway -n gateway-system
NAME             CLASS   ADDRESS        PROGRAMMED
shared-gateway   istio   10.0.10.50   True
```

---

## Azure AGC vs Azure Istio — Operational Differences

### How traffic reaches the cluster

**AGC** operates entirely outside the AKS cluster. Microsoft provisions an Azure-managed L7 load
balancer (the Application Gateway for Containers resource) in response to the `ApplicationLoadBalancer`
CR. The ALB Controller pod runs inside the cluster and watches for `Gateway` and `HTTPRoute`
resources, then programs the external AGC resource over the Azure control plane. No Envoy pod is
created; traffic never touches an in-cluster proxy before reaching the backend pods.

**Istio** runs the data plane inside the cluster. The `istiod` control plane and an Envoy
(`shared-gateway-istio`) pod are both deployed into `gateway-system`. An Azure Standard Load
Balancer (L4) forwards external traffic to the Envoy pod's NodePort, and Envoy applies the
`Gateway` / `HTTPRoute` routing rules before passing requests to the backend.

### IP address and DNS

**AGC** does not expose a routable public IP. Instead it assigns a stable Azure-internal hostname
of the form `<hash>.fz52.alb.azure.com`. DNS must use a **CNAME** record pointing to that hostname.
The hostname is stable across Gateway updates, cluster upgrades, and node replacements.

**Istio** exposes a public IP through the `shared-gateway-istio` LoadBalancer Service. To prevent
the IP from changing when the Service or cluster is recreated, a static `PublicIPAddress` resource
is pre-allocated in the AKS node resource group (`mc-aks-<stack>`) by the 03_cluster stack and
bound to the Gateway via `spec.addresses` (`type: IPAddress`). DNS uses an **A record** pointing
to that static IP.

### Failure domain

With **AGC**, a misconfigured `Gateway` or `HTTPRoute` affects only the Azure-side programming.
The in-cluster workloads continue running; only external routing is disrupted.

With **Istio**, the Envoy pod is in the request path. If `shared-gateway-istio` crashes or is
evicted, all external traffic stops. The Deployment has a single replica by default — consider
scaling it for production workloads.

### Health probes

**AGC** manages its own health checks against the backend pods and handles probe configuration
automatically.

**Istio** relies on the Azure Load Balancer to health-check the Envoy pod. Azure defaults to an
HTTP probe on port 80 with path `/`, which Envoy answers with 404 (no matching route). This is
worked around by propagating the annotation
`service.beta.kubernetes.io/port_80_health-probe_port: "15021"` via `spec.infrastructure.annotations`
on the `Gateway` resource — Azure then probes Istio's dedicated status endpoint
(`15021/healthz/ready`) instead.

### Resource consumption

| | AGC | Istio |
|---|---|---|
| In-cluster pods | ALB Controller only | istiod + Envoy (shared-gateway-istio) |
| Azure resources | AGC frontend, ALB subnet | Standard LB, Public IP |
| Cluster CPU/memory | Minimal | ~200m CPU, ~256Mi per istiod + Envoy |
| Required subnet | Delegated `/24` (`snet-alb-<stack>`) | None |

### Gateway implementation by target

| Target | GatewayClass | Backed by |
|---|---|---|
| GKE | `gke-l7-regional-external-managed` | Google Cloud L7 load balancer |
| AKS | `azure-alb-external` | Application Gateway for Containers |
| On-prem | `istio` | Istio Envoy behind MetalLB |

On **AKS, AGC is the only supported option.** It offloads L7 routing to Azure, keeps
in-cluster resource usage down, and needs no health-probe workaround.
