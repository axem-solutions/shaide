# Gateway API CRDs — Offline Manifests

CRD manifests are stored locally so that `pulumi up` requires no internet access on the provisioner.
Each subdirectory is a self-contained kustomize directory consumed by the `gateway-provider` stack.

| Directory | File | Source project | Pinned version |
|---|---|---|---|
| `gateway-api/standard/` | `crds.yaml` | kubernetes-sigs/gateway-api (standard channel) | v1.6.2 |
| `gie/` | `crds.yaml` | kubernetes-sigs/gateway-api-inference-extension | v1.6.2 |
| `gie/` | `llm-d-router-crds.yaml` | llm-d/llm-d-router (InferenceObjective, InferenceModelRewrite) | v1.0.0 |

The versions follow the pins of llm-d v1.0 (`guides/env.sh` in the llm-d repository).
Since GIE v1.5, `InferenceObjective` and `InferenceModelRewrite` are served by llm-d
under the `llm-d.ai` group, so they ship with the router rather than with GIE. The llm-d
router (EPP) watches them, so they must be installed even when no objects use them.

These are the default `gatewayApiCrdsPath` and `gieCrdsPath` on every platform. Azure
skips the Gateway API CRDs (`installGatewayApiCrds` defaults to `false`) because AGC owns
them.

---

## Prerequisites

- `kubectl` with kustomize support (`kubectl version` >= 1.27)
- `curl`
- Internet access on the machine running the commands

`kubectl kustomize` with a remote URL is a purely client-side operation — it fetches from
GitHub and renders YAML locally. It never connects to a Kubernetes cluster. Kubeconfig is
irrelevant; these commands can be run from any machine with internet access.

---

## Download

Run the following commands from the repository root.

### Gateway API CRDs — standard channel (v1.6.2)

Safe to apply on clusters that already have standard-channel CRDs installed.

```bash
kubectl kustomize \
  "https://github.com/kubernetes-sigs/gateway-api/config/crd?ref=v1.6.2" \
  > infra/gateway-provider/crds/gateway-api/standard/crds.yaml
```

### Gateway API Inference Extension CRDs (v1.6.2)

```bash
kubectl kustomize \
  "https://github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd?ref=v1.6.2" \
  > infra/gateway-provider/crds/gie/crds.yaml
```

### llm-d router CRDs (v1.0.0)

```bash
curl -sSfL \
  https://github.com/llm-d/llm-d-router/releases/download/v1.0.0/manifests.yaml \
  > infra/gateway-provider/crds/gie/llm-d-router-crds.yaml
```

---

## Verify

```bash
# Count CRDs in each rendered directory: 10 and 4 for the pinned versions
kubectl kustomize infra/gateway-provider/crds/gateway-api/standard \
  | grep -c "^kind: CustomResourceDefinition"

kubectl kustomize infra/gateway-provider/crds/gie \
  | grep -c "^kind: CustomResourceDefinition"
```

---

## Upgrading

To upgrade to a newer version:

1. Re-run the download commands above with the new tags. Take the versions from
   `guides/env.sh` of the llm-d release shaide follows (`GATEWAY_API_VERSION`,
   `GAIE_VERSION`, `ROUTER_RELEASE_VERSION`).
2. Update the version column in this table.
3. Update the versions noted in `../README.md` and `../deployments/Pulumi.TEMPLATE.yaml`.
4. Commit and re-run `pulumi up`.

A CRD that a new bundle no longer contains is deleted by `pulumi up`, together with every
object of that kind. Check the cluster for such objects before upgrading.
