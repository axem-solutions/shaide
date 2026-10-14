# Gateway API CRDs — Offline Manifests

CRD manifests are stored locally so that `pulumi up` requires no internet access on the provisioner.
Each subdirectory is a self-contained kustomize directory consumed by the `gateway-provider` stack.

| Directory | Channel | Source project | Pinned version |
|---|---|---|---|
| `gateway-api/standard/` | standard | kubernetes-sigs/gateway-api | v1.5.1 |
| `gie/` | — | kubernetes-sigs/gateway-api-inference-extension | v1.4.0 |

These are the default `gatewayApiCrdsPath` and `gieCrdsPath` on every platform. Azure
skips the Gateway API CRDs (`installGatewayApiCrds` defaults to `false`) because AGC owns
them.

---

## Prerequisites

- `kubectl` with kustomize support (`kubectl version` >= 1.27)
- Internet access on the machine running the commands

`kubectl kustomize` with a remote URL is a purely client-side operation — it fetches from
GitHub and renders YAML locally. It never connects to a Kubernetes cluster. Kubeconfig is
irrelevant; these commands can be run from any machine with internet access.

---

## Download

Run the following commands from the repository root.
Each command renders the remote kustomize directory into a single file and commits it.

### Gateway API CRDs — standard channel (v1.5.1)

Safe to apply on clusters that already have standard-channel CRDs installed.

```bash
kubectl kustomize \
  "https://github.com/kubernetes-sigs/gateway-api/config/crd?ref=v1.5.1" \
  > infra/gateway-provider/crds/gateway-api/standard/crds.yaml
```

### Gateway API Inference Extension CRDs (v1.4.0)

```bash
kubectl kustomize \
  "https://github.com/kubernetes-sigs/gateway-api-inference-extension/config/crd?ref=v1.4.0" \
  > infra/gateway-provider/crds/gie/crds.yaml
```

---

## Verify

```bash
# Count CRDs in each rendered file
grep "^kind: CustomResourceDefinition" \
  infra/gateway-provider/crds/gateway-api/standard/crds.yaml | wc -l

grep "^kind: CustomResourceDefinition" \
  infra/gateway-provider/crds/gie/crds.yaml | wc -l
```

---

## Commit

After downloading, commit both `crds.yaml` files:

```bash
git add infra/gateway-provider/crds/gateway-api/standard/crds.yaml \
        infra/gateway-provider/crds/gie/crds.yaml
git commit -m "chore(gateway-provider): pin gateway-api v1.5.1 and GIE v1.4.0 CRDs offline"
```

---

## Upgrading

To upgrade to a newer version:

1. Re-run the download commands above with the new `?ref=` tag.
2. Update the version column in this table.
3. Update the versions noted in `../README.md` and `../deployments/Pulumi.TEMPLATE.yaml`.
4. Commit and re-run `pulumi up`.
