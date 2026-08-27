---
title: "TLS certificates"
description: "Issuing and renewing certificates for the platform gateway."
weight: 50
---

# TLS certificates

TLS is handled by cert-manager + Let's Encrypt (HTTP-01 challenge via the `letsencrypt-azure` ClusterIssuer deployed by `03_cluster`). It is enabled per-stack by setting `certManagerIssuer` in the stack config.

The `cert-manager.io/cluster-issuer` annotation on the Gateway causes cert-manager to automatically create and own the `Certificate` resource. Pulumi does **not** create a Certificate explicitly — only the annotation and the HTTP+HTTPS listeners.

## Enabling HTTPS on a stack

Set these keys in the stack's `Pulumi.<stack>.yaml`:

```yaml
gateway-provider:certManagerIssuer: letsencrypt-azure
gateway-provider:tlsSecretName: gateway-tls
gateway-provider:bootstrapTlsSecret: "true"   # Azure/AGC only, see below
```

- HTTP-01 challenge requires port 80 to be reachable from the internet.
- The ClusterIssuer (`letsencrypt-azure`) must be Ready: `kubectl get clusterissuer letsencrypt-azure`.
- DNS for the gateway hostname does **not** need to be set before `pulumi up` — cert-manager retries the ACME challenge on its own poll loop, so you can point DNS at the gateway's address *after* deploying and it'll pick up automatically. It does need to be correct before the challenge can succeed.

### Azure/AGC (`azure-alb-external`): the first-boot deadlock

On the `azure-alb-external` GatewayClass (Application Gateway for Containers), enabling TLS on a **brand-new** stack deadlocks by default:

1. The `https` listener references `tlsSecretName`, which doesn't exist yet → `ResolvedRefs: False`.
2. AGC refuses to program *any* listener (including `http`) while the Gateway overall isn't `Accepted` — see `kubectl get gateway shared-gateway -n gateway-system -o json | jq '.status.conditions'` (`ListenersNotValid`) and the `alb-controller` logs in `kube-system` (`Secret "gateway-tls" not found on the cluster. Please create the secret on the cluster.`).
3. Port 80 stays unreachable, so cert-manager's HTTP-01 self-check never succeeds and the Secret never gets created — back to step 1.

Historically this was avoided by staging the rollout across two separate `pulumi up` runs: deploy with `certManagerIssuer`/`tlsSecretName` commented out first (HTTP-only Gateway, AGC provisions cleanly), confirm it's live, *then* uncomment and run `pulumi up` again — by that point port 80 is already provisioned and working, so the new (transiently invalid) `https` listener doesn't block AGC from keeping the already-working `http` listener serving the ACME challenge. `dev-westeurope`/`dev-polandcentral` were bootstrapped this way.

That staged process is easy to forget (skipping it is exactly what deadlocked `trial-westeurope`) and isn't repeatable as a single automated deploy. The current fix: set `gateway-provider:bootstrapTlsSecret: "true"` alongside `certManagerIssuer`/`tlsSecretName` **from the start**, in one `pulumi up`. This makes Pulumi seed `tlsSecretName` with a throwaway self-signed cert on first apply, which satisfies AGC's `ResolvedRefs` check and unblocks the `http` listener without needing a prior working deploy to fall back on. cert-manager then completes the real ACME challenge and overwrites the Secret's contents in place. Pulumi never touches the Secret's data again after the initial create (`IgnoreChanges`), so it's safe to leave the flag set permanently and it won't clobber the real cert on later `pulumi up` runs.

**Recommended flow for a brand-new Azure/AGC stack (single run):**
1. Set `certManagerIssuer` + `tlsSecretName` + `bootstrapTlsSecret: "true"` in the stack config from the start.
2. `pulumi up`.
3. Get the assigned gateway address: `kubectl get gateway shared-gateway -n gateway-system -o jsonpath='{.status.addresses}'`.
4. Point DNS at it.
5. Wait — no further `pulumi up` needed; cert-manager issues the real cert automatically once DNS resolves.

Already-issued stacks (`dev-westeurope`, `dev-polandcentral`) don't need `bootstrapTlsSecret` and shouldn't set it.

## Stack status

Track which stacks have HTTPS enabled (`certManagerIssuer` set) vs. HTTP-only
(commented out) in your own deployment notes — this varies per environment and per
region's AGC availability (see [Gateway and routing](../architecture/gateway.md#when-to-use-which)).

## Verify Let's Encrypt is active

```bash
# ClusterIssuer must be Ready
kubectl get clusterissuer letsencrypt-azure

# Certificate — owned by the Gateway (auto-created by cert-manager Gateway API integration)
# READY: True = cert issued; False = still pending or error
kubectl get certificate -n gateway-system

# TLS secret populated by cert-manager
kubectl get secret gateway-tls -n gateway-system

# Gateway must have both HTTP (80) and HTTPS (443) listeners
kubectl get gateway -n gateway-system shared-gateway -o jsonpath='{.spec.listeners}' | jq .

# Dig into a pending/failed certificate
kubectl describe certificate gateway-tls -n gateway-system
kubectl get certificaterequest,order -n gateway-system
```

### Expected healthy state

```
NAME                READY   AGE
letsencrypt-azure   True    ...

NAME          READY   SECRET        AGE
gateway-tls   True    gateway-tls   ...

NAME          TYPE                DATA   AGE
gateway-tls   kubernetes.io/tls   2      ...
```

The `gateway-tls` Certificate is owned by `shared-gateway` via `ownerReferences`. Check with:
```bash
kubectl get certificate gateway-tls -n gateway-system -o jsonpath='{.metadata.ownerReferences}'
```

## Verify Envoy has the cert loaded

```bash
# List secrets loaded in Envoy's SDS — should include "kubernetes-gateway://gateway-system/gateway-tls"
kubectl exec -n gateway-system deploy/shared-gateway-istio -- \
  curl -s localhost:15000/config_dump | \
  jq '.configs[] | select(.["@type"] | contains("SecretsConfigDump")) | .dynamic_active_secrets[].name'
```

## Test HTTPS end-to-end from inside the cluster

Direct pod IP with `--resolve` injects the correct SNI (required — Envoy rejects connections without matching SNI):

```bash
POD_IP=$(kubectl get pod -n gateway-system -l gateway.networking.k8s.io/gateway-name=shared-gateway \
  -o jsonpath='{.items[0].status.podIP}')

kubectl run test --rm -it --image=nicolaka/netshoot --restart=Never -- \
  curl -v --insecure \
  --resolve <your-gateway-hostname>:443:$POD_IP \
  https://<your-gateway-hostname>/control-panel
```

A 200 with `server: istio-envoy` and a valid Let's Encrypt cert confirms Envoy is working correctly.

## Diagnose HTTPS hanging at TCP connect (Azure LB)

If `curl -v https://<hostname>` hangs at `Trying <ip>:443...`, the Azure Load Balancer is dropping connections — either no LB rule for 443 or the health probe is failing.

```bash
# 1. Confirm port 443 is on the Service
kubectl get svc shared-gateway-istio -n gateway-system

# 2. Confirm health probe annotations propagated to the Service
kubectl get svc shared-gateway-istio -n gateway-system -o yaml | grep health-probe

# 3. Check the Azure LB directly
MC_RG="mc-aks-<stack-name>"   # node resource group (mc-aks-<stack>)
SUB="<your-subscription-id>"

az network lb probe list -g $MC_RG --lb-name kubernetes --subscription $SUB -o table
az network lb rule list  -g $MC_RG --lb-name kubernetes --subscription $SUB -o table | grep 443
```

### Known issue: Azure LB port 443 probe uses Https instead of Http

When port 443 is added to a Service, Azure defaults the health probe to `Https` protocol. Istio's readiness endpoint (`15021`) is plain HTTP — the HTTPS probe fails, backend is marked unhealthy, connections are silently dropped.

The fix is already in `main.go` via `infrastructure.annotations`:
```
service.beta.kubernetes.io/port_443_health-probe_port:     "15021"
service.beta.kubernetes.io/port_443_health-probe_protocol: "http"
```

These redirect the probe to Istio's HTTP readiness port. Verify in Azure:
```
TCP-443 probe → Port: 31560, Protocol: Http, Path: /healthz/ready  ✅
TCP-443 probe → Port: 31560, Protocol: Https, Path: /healthz/ready ❌ (old/broken state)
```

After a `pulumi up`, wait 2–3 minutes for Azure to reconcile the new probe before testing.

## Pulumi SSA field manager conflict

**Symptom:**
```
Apply failed with 2 conflicts: conflicts with "pulumi-kubernetes-XXXXXXXX":
- .spec.addresses
- .spec.listeners[name="http"].hostname
```

**Cause:** Each `pulumi up` uses a new unique field manager ID. A previous run (or accidental wrong-context run) left ownership of Gateway fields with an old ID.

**Fix — clear stale ownership via kubectl, then re-run pulumi up:**
```bash
kubectl get gateway shared-gateway -n gateway-system -o json | \
  jq 'del(.metadata.managedFields)' | \
  kubectl apply --server-side --force-conflicts -f -
```

The `pulumi.com/patchForce: "true"` annotation on the Gateway (set via `annotationsOutput` in `main.go`) prevents recurrence — future runs always force-take ownership.

## Useful Envoy admin commands

All commands use `localhost:15000` (Envoy's admin port, accessible only from inside the pod):

```bash
# Active listeners (confirm 443 is present)
kubectl exec -n gateway-system deploy/shared-gateway-istio -- \
  curl -s localhost:15000/listeners

# Full xDS config dump (listeners, routes, clusters, endpoints)
kubectl exec -n gateway-system deploy/shared-gateway-istio -- \
  curl -s localhost:15000/config_dump | jq .

# istioctl shortcuts (cleaner output)
istioctl proxy-config listeners -n gateway-system deploy/shared-gateway-istio
istioctl proxy-config routes    -n gateway-system deploy/shared-gateway-istio
istioctl proxy-config clusters  -n gateway-system deploy/shared-gateway-istio
```

Envoy ports on the gateway pod:
| Port | Purpose |
|---|---|
| `80` / `443` | Traffic ingress |
| `15000` | Admin interface (localhost only) |
| `15021` | Istio readiness/liveness (`/healthz/ready`) — used for Azure LB health probe |
| `15090` | Prometheus metrics |
