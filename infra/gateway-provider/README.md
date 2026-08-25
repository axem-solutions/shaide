# gateway-provider

Pulumi stack that installs the networking foundation required by all model-serving workloads.

- Installs Gateway API CRDs and Gateway API Inference Extension (GIE) CRDs
- Installs Istio control plane (`istio-base` + `istiod`) as the Gateway API implementation
- Creates a shared `Gateway` resource in `gateway-system` for centralised ingress

For architecture details, traffic flows, and design decisions see [DESIGN.md](DESIGN.md).

---

## Stack Config Reference

| Key | Required | Default | Description |
|---|---|---|---|
| `cloudProvider` | Yes | — | `gcp`, `azure`, or `on-prem` |
| `gatewayClassName` | Yes | — | Gateway controller class: `gke-l7-regional-external-managed` (GCP), `azure-alb-external` (Azure), or `istio` (on-prem) |
| `namespace` | No | `istio-system` | Namespace for Istio control plane |
| `kubeconfig` | No | `~/.kube/config` | Path to kubeconfig; omit on GCP/Azure (uses cloud auth plugin) |
| `istioHub` | No | `docker.io/istio` | Container image registry for Istio images |
| `istioTag` | No | `1.28.1` | Istio version |
| `installGatewayApiCrds` | No | `false` on Azure, `true` elsewhere | Whether this stack installs upstream Gateway API CRDs. Keep `false` on AKS managed Gateway API clusters because Azure owns those CRDs. |
| `gatewayApiCrdsPath` | No | GitHub URL v1.5.1 experimental | Override CRD source (use local path for air-gapped environments) |
| `gieCrdsPath` | No | GitHub URL v1.4.0 | Override GIE CRD source (use local path for air-gapped environments) |
| `infraStackRef` | No | — | Pulumi StackReference to cloud infra stack exporting `gatewayHostname`, `gatewayCertName`, `gatewayStaticIPName`, `albSubnetId`. Set on GCP and Azure stacks. |
| `gatewayHostname` | No | — | Direct hostname for the shared gateway (on-prem alternative to `infraStackRef`). Set on RKE2 stack. |
| `albName` | No | — | Azure AGC `ApplicationLoadBalancer` resource name. When set, creates the ALB in `gateway-system` using `albSubnetId` from `infraStackRef`. |
| `tlsCertAnnotation` | No | — | Provider annotation for TLS certificate binding. GCP: `networking.gke.io/cert-manager-certs` |
| `tlsSecretName` | No | — | K8s Secret name for TLS cert via `certificateRefs`. Used by on-prem (manual) and Azure (cert-manager). |
| `certManagerIssuer` | No | — | cert-manager ClusterIssuer name. When set, creates a Certificate resource + HTTP listener for ACME challenge. Azure: `letsencrypt-azure` |
| `bootstrapTlsSecret` | No | `false` | Azure/AGC only. Seeds `tlsSecretName` with a throwaway self-signed cert on first `pulumi up` so the ALB Controller will program the `http` listener instead of deadlocking on the not-yet-issued secret. See [TLS.md](TLS.md#azureagc-azure-alb-external-the-first-boot-deadlock). Safe to leave set permanently. |

`infraStackRef` and `gatewayHostname` are mutually exclusive. If neither is set, no shared
gateway or `gateway-system` namespace is created.

### GatewayClass per environment

| Environment | `gatewayClassName` | Controller |
|---|---|---|
| GCP | `gke-l7-regional-external-managed` | GKE Gateway controller (built-in) |
| Azure | `azure-alb-external` | ALB Controller for AGC (AKS add-on, deployed by `infra/azure/03_cluster`) |
| On-prem | `istio` | Istio ingress gateway |

### TLS per environment

| Environment | Mechanism | Config keys |
|---|---|---|
| GCP | GCP Certificate Manager (cloud-managed, auto-renewal) | `infraStackRef` + `tlsCertAnnotation` |
| Azure | cert-manager + Let's Encrypt (cluster-hosted, auto-renewal) | `infraStackRef` + `certManagerIssuer` + `tlsSecretName` (+ `bootstrapTlsSecret` on a brand-new stack) |
| On-prem | Manual cert as Pulumi secrets | `tlsCert` + `tlsKey` (+ optional `tlsSecretName`) |

### Gateway address per environment

| Environment | Address type | DNS record | Source |
|---|---|---|---|
| GCP | `NamedAddress` (static IP) | A record → IP | `gatewayStaticIPName` from `infraStackRef` |
| Azure AGC | `Hostname` (AGC-managed `*.alb.azure.com`) | CNAME → hostname | Assigned by AGC automatically; no static IP needed |
| On-prem | none | — | no external address |

---

## Deployment Order

```
1. gateway-provider   ← this stack (CRDs + Istio + shared-gateway)
2. app_serving        ← per-model Gateway + InferencePool + HTTPRoute
3. app_shaide         ← HTTPRoute to shared-gateway
```

---

## Purge Istio

If Istio is already installed by another stack, uninstall it before running this stack.

```bash
kubectl delete ns istio-system

kubectl delete crd \
  authorizationpolicies.security.istio.io \
  destinationrules.networking.istio.io \
  envoyfilters.networking.istio.io \
  gateways.networking.istio.io \
  peerauthentications.security.istio.io \
  proxyconfigs.networking.istio.io \
  requestauthentications.security.istio.io \
  serviceentries.networking.istio.io \
  sidecars.networking.istio.io \
  telemetries.telemetry.istio.io \
  virtualservices.networking.istio.io \
  wasmplugins.extensions.istio.io \
  workloadentries.networking.istio.io \
  workloadgroups.networking.istio.io

kubectl delete clusterrolebinding \
  istio-reader-clusterrole-istio-system \
  istiod-clusterrole-istio-system \
  istiod-gateway-controller-istio-system

kubectl delete clusterrole \
  istiod-clusterrole-istio-system \
  istiod-gateway-controller-istio-system \
  istio-reader-clusterrole-istio-system

kubectl delete validatingwebhookconfiguration \
  istiod-default-validator \
  istio-validator-istio-system

kubectl delete mutatingwebhookconfiguration istio-sidecar-injector
```
