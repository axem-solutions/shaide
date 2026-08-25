# app_shaide — Deployments

This directory contains Pulumi stack configuration files for all `app_shaide` deployment targets.
The section below focuses on the **on-prem stack** and how MetalLB provides load balancing
in place of a cloud-managed LB (GCP NLB, AWS NLB, etc.).

---

## Stack files

See [`Pulumi.TEMPLATE.yaml`](Pulumi.TEMPLATE.yaml) for the full config reference; copy it
to create a new stack (`cp Pulumi.TEMPLATE.yaml Pulumi.<stack-name>.yaml`). The LB
mechanism a stack ends up with depends on which config keys it sets, not on
`cloudProvider` directly:

| Config | LB mechanism |
|---|---|
| `infraStackRef` or `gatewayHostname` set | ClusterIP + HTTPRoute to a shared Gateway (any cloud, or on-prem) |
| Neither set, `cloudProvider: gcp` | `LoadBalancer` — GKE external NLB + `HealthCheckPolicy` |
| Neither set, `cloudProvider: on-prem`, `lbAnnotations` has `metallb.universe.tf/address-pool` | `LoadBalancer` — MetalLB L2 (this document) |
| Neither set, other clouds | `LoadBalancer` with `lbAnnotations` for that cloud's LB controller |

The rest of this document assumes an on-prem stack that takes the MetalLB path — i.e.
one that sets `lbAnnotations` and leaves both `infraStackRef` and `gatewayHostname`
unset. **If a stack sets `gatewayHostname` (the on-prem Gateway/HTTPRoute pattern used
by `infra/gateway-provider`), it takes the ClusterIP+HTTPRoute path instead and none of
the MetalLB behavior below applies to it** — check the specific stack's config before
assuming it uses MetalLB.

---

## MetalLB on-prem load balancing

### What is MetalLB?

MetalLB is a load-balancer controller for bare-metal Kubernetes clusters.
Cloud providers (GCP, AWS) implement the `LoadBalancer` Service type natively.
On-prem clusters have no such integration — MetalLB fills that gap by:

1. Maintaining a pool of IP addresses that it owns.
2. Assigning one IP from the pool to every Service of type `LoadBalancer`.
3. Advertising that IP to the local network so traffic can reach the cluster.

This project uses **L2 mode** — MetalLB responds to ARP requests for the assigned IP,
making one cluster node act as the next-hop for incoming traffic.

---

### Where MetalLB lives in this repo

MetalLB is provisioned by the infrastructure layer, not by `app_shaide` itself.

```
infra/on-prem/pulumi/services/          ← k8s-onprem-airgap-services stack
└── components/metallb/
    ├── namespace.go    creates metallb-system namespace
    ├── metallb.go      Helm release (chart v0.15.3 from Harbor)
    ├── values.go       Helm values — images from harbor.internal.lan/services/
    ├── ippool.go       IPAddressPool CR  (default-pool)
    │                   L2Advertisement CR (default-l2advert)
    └── pullsecret.go   Harbor robot pull secret for metallb-system namespace
```

The Pulumi deployment order within that stack is strictly enforced:

```
Namespace
   └─► PullSecret (harbor-pull-secret)
          └─► Helm release (metallb / metallb-system)
                 └─► IPAddressPool  (default-pool)
                        └─► L2Advertisement (default-l2advert)
```

---

### MetalLB components

| Component | Kind | Role |
|---|---|---|
| **Controller** | Deployment (1 pod, pinned to `srv2rke2w1`) | Watches Services; allocates IPs from the pool |
| **Speaker** | DaemonSet (all nodes) | Handles ARP for assigned IPs; routes traffic to the right node |
| **IPAddressPool** | CRD (`metallb.io/v1beta1`) | Declares the usable IP range: `10.99.10.200–10.99.10.220` |
| **L2Advertisement** | CRD (`metallb.io/v1beta1`) | Binds the pool to L2 (ARP) mode |

---

### Architecture

```
  ON-PREM LAN  (10.99.10.0/24)
  ┌──────────────────────────────────────────────────────────────┐
  │                                                              │
  │   Client / Router                                            │
  │       │                                                      │
  │       │  "Who has 10.99.10.200?"  (ARP request)             │
  │       │                                                      │
  │       ▼                                                      │
  │  ┌─────────────────────────────────────────────────────┐    │
  │  │              RKE2 Cluster                            │    │
  │  │                                                      │    │
  │  │  ┌──────────────┐   ┌──────────────┐  ┌──────────┐ │    │
  │  │  │  srv2rke2w1  │   │  srv3rke2w2  │  │  ...     │ │    │
  │  │  │  (control +  │   │  (GPU worker)│  │          │ │    │
  │  │  │   worker)    │   │              │  │          │ │    │
  │  │  │              │   │              │  │          │ │    │
  │  │  │ ┌──────────┐ │   │ ┌──────────┐ │  │          │ │    │
  │  │  │ │ MetalLB  │ │   │ │ MetalLB  │ │  │          │ │    │
  │  │  │ │ Speaker  │ │   │ │ Speaker  │ │  │          │ │    │
  │  │  │ │ (ARP     │ │   │ │          │ │  │          │ │    │
  │  │  │ │ reply ◄──┼─┼───┼─┼──────────┼─┼──┼──────────┼─┼──► │
  │  │  │ │ "I do!") │ │   │ │          │ │  │          │ │    │
  │  │  │ └──────────┘ │   │ └──────────┘ │  │          │ │    │
  │  │  │              │   │              │  │          │ │    │
  │  │  │ ┌──────────┐ │   │ ┌──────────┐ │  │          │ │    │
  │  │  │ │ MetalLB  │ │   │ │shaide-   │ │  │          │ │    │
  │  │  │ │Controller│ │   │ │server pod│ │  │          │ │    │
  │  │  │ └──────────┘ │   │ └──────────┘ │  │          │ │    │
  │  │  └──────────────┘   └──────────────┘  └──────────┘ │    │
  │  │                                                      │    │
  │  │  Service: shaide-server  type=LoadBalancer           │    │
  │  │  Annotation: metallb.universe.tf/address-pool:       │    │
  │  │              default-pool                            │    │
  │  │  Assigned IP: 10.99.10.200  ◄── from pool           │    │
  │  └─────────────────────────────────────────────────────┘    │
  └──────────────────────────────────────────────────────────────┘
```

---

### IP assignment flow

```
 pulumi up (app_shaide, on-prem stack)
       │
       ▼
 Kubernetes API receives Service manifest
 ┌─────────────────────────────────────────┐
 │  kind: Service                          │
 │  spec.type: LoadBalancer                │
 │  metadata.annotations:                  │
 │    metallb.universe.tf/address-pool:    │
 │      default-pool                       │
 └─────────────────────────────────────────┘
       │
       ▼
 MetalLB Controller watches Services
       │  finds type=LoadBalancer
       │  reads address-pool annotation
       │  picks first free IP from default-pool
       ▼
 Assigns  status.loadBalancer.ingress[0].ip = 10.99.10.20x
       │
       ▼
 MetalLB Speaker (on the node running the pod)
       │  sends gratuitous ARP for 10.99.10.20x
       │  LAN switches update their ARP tables
       ▼
 Inbound traffic for 10.99.10.20x arrives at that node
       │
       ▼
 kube-proxy / iptables DNAT
       │  routes to shaide-server pod : 8080
       ▼
 shaide-server responds on port 80 (Service) → 8080 (container)
```

---

### How app_shaide triggers MetalLB

The annotation is the only coupling between `app_shaide` and MetalLB.
It lives in the on-prem stack's `Pulumi.<stack-name>.yaml`, and only takes effect when
that stack leaves both `infraStackRef` and `gatewayHostname` unset (see the table above):

```yaml
app-shaide:lbAnnotations:
  metallb.universe.tf/address-pool: default-pool
```

`pkg/iac/shaide/internal/config/config.go` reads it as `cfg.LBAnnotations` (a `map[string]string`).

`pkg/iac/shaide/internal/components/shaide/deploy.go` uses it at Service creation time:

```go
// Both infraStackRef (cloud StackReference) and gatewayHostname (direct on-prem
// hostname) route through the shared Gateway instead of a LoadBalancer — either one
// being set is enough to switch the Service to ClusterIP.
useGateway := cfg.Routing.InfraStackRef != "" || cfg.Routing.GatewayHostname != ""

svcType := "LoadBalancer"
var annotations pulumi.StringMap
if useGateway {
    svcType = "ClusterIP"                        // Gateway/HTTPRoute path (any cloud, or on-prem)
} else {
    annotations = toStringMap(cfg.LBAnnotations) // LoadBalancer path — MetalLB annotation applied here
}
```

The resulting Service manifest applied to the cluster:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: shaide-server
  namespace: app-shaide
  annotations:
    metallb.universe.tf/address-pool: default-pool
spec:
  type: LoadBalancer
  selector:
    app.kubernetes.io/name: shaide-server
  ports:
    - name: http
      port: 80
      targetPort: 8080
      protocol: TCP
```

---

### Prerequisites (deployment order)

MetalLB must be running before `app_shaide` is deployed.
The correct order is:

```
1. k8s-onprem-airgap-infra    (infra/on-prem/pulumi/infra)
   └─ creates hostPath StorageClass

2. k8s-onprem-airgap-services  (infra/on-prem/pulumi/services)
   └─ deploys Harbor → MetalLB → GPU Operator
      MetalLB IPAddressPool: default-pool  10.99.10.200–10.99.10.220
      MetalLB L2Advertisement: default-l2advert

3. app_shaide  (this stack, on-prem)
   └─ creates shaide-server Service (LoadBalancer)
      MetalLB assigns IP from default-pool
```

If step 2 is skipped, the Service will be stuck in `<pending>` for `EXTERNAL-IP`
because no controller processes the `LoadBalancer` type.

---

### Verify the IP was assigned

```bash
kubectl -n app-shaide get svc shaide-server
# NAME            TYPE           CLUSTER-IP     EXTERNAL-IP     PORT(S)        AGE
# shaide-server   LoadBalancer   10.43.x.x      10.99.10.200    80:3xxxx/TCP   1m
```

Check which node is currently advertising the IP (speaker election):

```bash
kubectl -n metallb-system get ipaddresspool default-pool -o yaml
kubectl -n metallb-system get l2advertisement default-l2advert -o yaml
```

Verify ARP from the LAN side:

```bash
arp -n 10.99.10.200
# Address     HWtype  HWaddress           Flags Iface
# 10.99.10.200  ether  xx:xx:xx:xx:xx:xx   C     eth0
```

---

### Configuration reference

| Key | Where | Description |
|---|---|---|
| `metallb:ipPool` | `infra/.../services/Pulumi.rke2-cluster.yaml` | IP range given to MetalLB (`10.99.10.200-10.99.10.220`) |
| `metallb:controllerNodeHostname` | same | Node the MetalLB controller pod is pinned to |
| `metallb:chartPath` | same | Path to the air-gapped Helm chart tarball |
| `IPAddressPool.metadata.name` | `ippool.go` | Pool name used in the annotation (`default-pool`) |
| `app_shaide:lbAnnotations` | on-prem `Pulumi.<stack-name>.yaml` | Annotation applied to the shaide-server Service |

The annotation key `metallb.universe.tf/address-pool` and the pool name `default-pool`
**must match exactly** — a typo causes MetalLB to ignore the Service and leave
`EXTERNAL-IP` as `<pending>`.
