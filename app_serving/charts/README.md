# Vendored charts

| Directory | Chart | Version | Source |
|---|---|---|---|
| `llm-d-router-gateway/` | llm-d router, Gateway mode (InferencePool + endpoint picker) | v1.0.0 | `oci://ghcr.io/llm-d/charts/llm-d-router-gateway` |

The chart is committed unpacked so every platform, including air-gapped on-prem
clusters, deploys the same reviewed copy without registry access. The
app-serving stack installs it from here (`DefaultRouterChartPath` in
`pkg/iac/serving/internal/config/config.go`); the installer image copies this
directory.

## Upgrading

Take the version from `ROUTER_CHART_VERSION` in `guides/env.sh` of the llm-d
release shaide follows, then from this directory:

```bash
rm -rf llm-d-router-gateway
helm pull oci://ghcr.io/llm-d/charts/llm-d-router-gateway --version <version> --untar
```

Update the version above and the EPP image (`DefaultRouterImage` in
`pkg/iac/serving/internal/config/defaults.go` and
`installer/build/manifests/images.yaml`) to the same release: the chart's own
default image tag is `main`, so shaide always sets it.
