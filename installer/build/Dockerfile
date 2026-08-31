# syntax=docker/dockerfile:1.7

# -----------------------------------------------------------------------------
# Builder stage
#
# Compile the Go installer in a dedicated stage so the Go toolchain, source
# code, and build caches are not included in the final runtime image.
# -----------------------------------------------------------------------------
FROM golang:1.26.3-alpine AS builder

WORKDIR /build/installer

COPY installer/go.mod installer/go.sum ./
COPY pkg/go.mod pkg/go.sum /build/pkg/

# Cache downloaded Go modules between builds using the same BuildKit builder.
#
# The cache mount is build-time storage and is not included in the image.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download


COPY installer/ ./
COPY pkg/ /build/pkg/


# Cache mounts:
#   /go/pkg/mod             downloaded Go modules
#   /root/.cache/go-build   compiled Go package cache
#
# Build flags:
#   CGO_ENABLED=0           produce a binary without C runtime dependencies
#   -mod=readonly           fail instead of modifying go.mod or go.sum
#   -trimpath               remove local filesystem paths from the binary
#   -ldflags="-s -w"        remove symbol and debug information to reduce size
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build \
        -mod=readonly \
        -trimpath \
        -ldflags="-s -w" \
        -o /installer \
        ./cmd/installer


# -----------------------------------------------------------------------------
# Helm chart downloader stage
#
# Download the pinned third-party charts at build time. Keeping this work in a
# separate stage avoids adding curl or its download cache to the runtime image.
# -----------------------------------------------------------------------------
FROM alpine:3.23 AS chart-downloader

ARG HARBOR_CHART_VERSION=1.18.2
ARG LOKI_CHART_VERSION=14.1.0
ARG GRAFANA_CHART_VERSION=12.3.2
ARG ALLOY_CHART_VERSION=1.8.1
ARG PROMETHEUS_CHART_VERSION=29.21.0

RUN apk add --no-cache ca-certificates curl \
    && mkdir -p \
        /opt/shaide-installer/projects/harbor/charts \
        /opt/shaide-installer/projects/monitoring/charts

RUN curl -fsSL --retry 3 \
    -o "/opt/shaide-installer/projects/harbor/charts/harbor-${HARBOR_CHART_VERSION}.tgz" \
    "https://helm.goharbor.io/harbor-${HARBOR_CHART_VERSION}.tgz"

RUN curl -fsSL --retry 3 \
    -o "/opt/shaide-installer/projects/monitoring/charts/loki-${LOKI_CHART_VERSION}.tgz" \
    "https://github.com/grafana-community/helm-charts/releases/download/loki-${LOKI_CHART_VERSION}/loki-${LOKI_CHART_VERSION}.tgz"

RUN curl -fsSL --retry 3 \
    -o "/opt/shaide-installer/projects/monitoring/charts/grafana-${GRAFANA_CHART_VERSION}.tgz" \
    "https://github.com/grafana-community/helm-charts/releases/download/grafana-${GRAFANA_CHART_VERSION}/grafana-${GRAFANA_CHART_VERSION}.tgz"

RUN curl -fsSL --retry 3 \
    -o "/opt/shaide-installer/projects/monitoring/charts/alloy-${ALLOY_CHART_VERSION}.tgz" \
    "https://github.com/grafana/helm-charts/releases/download/alloy-${ALLOY_CHART_VERSION}/alloy-${ALLOY_CHART_VERSION}.tgz"

RUN curl -fsSL --retry 3 \
    -o "/opt/shaide-installer/projects/monitoring/charts/prometheus-${PROMETHEUS_CHART_VERSION}.tgz" \
    "https://github.com/prometheus-community/helm-charts/releases/download/prometheus-${PROMETHEUS_CHART_VERSION}/prometheus-${PROMETHEUS_CHART_VERSION}.tgz"


# -----------------------------------------------------------------------------
# Runtime stage
#
# Use a smaller Debian image containing only the compiled installer and the
# external command-line tools required at runtime.
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS final

# Links the published package to this repository on GHCR. CI sets the same
# label through docker/metadata-action; declaring it here means locally built
# images that are pushed by hand are linked too.
LABEL org.opencontainers.image.source=https://github.com/axem-solutions/shaide
LABEL org.opencontainers.image.description="shaide platform installer"
LABEL org.opencontainers.image.licenses=Apache-2.0

ARG PULUMI_VERSION=3.244.0
ARG PULUMI_KUBERNETES_PLUGIN_VERSION=4.31.1

# --no-install-recommends:
#   Avoids optional packages that are not required.
#
# ca-certificates:
#   Enables HTTPS certificate verification.
#
# curl:
#   Used to install Pulumi and may also be required by the installer.
#
# python3 and python3-venv:
#   Required for the isolated Hugging Face CLI environment.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        python3 \
        python3-venv \
    \
    # Install Hugging Face tooling in an isolated Python virtual environment.
    && python3 -m venv /opt/huggingface \
    && /opt/huggingface/bin/pip install --no-cache-dir \
        "huggingface_hub[cli]==0.30.2" \
        hf_xet \
    \
    # Remove apt package indexes because they are not needed at runtime.
    && rm -rf /var/lib/apt/lists/*

# Install the Kubernetes resource plugin during the image build so the runtime
# environment does not need to download it, including in air-gapped setups.
RUN curl -fsSL https://get.pulumi.com \
    | sh -s -- --version "${PULUMI_VERSION}" \
    && /root/.pulumi/bin/pulumi plugin install \
        resource kubernetes \
        "${PULUMI_KUBERNETES_PLUGIN_VERSION}"

# Make the Pulumi and Hugging Face executables available without absolute paths.
ENV PATH="/root/.pulumi/bin:/opt/huggingface/bin:${PATH}"

# Let Xet maximize network, CPU, and disk concurrency for model transfers.
ENV HF_XET_HIGH_PERFORMANCE=1

# The Go compiler, source code, module cache, and build cache remain in the
# builder stage and are not included in the final image.
COPY --from=builder /installer /bin/installer


# -----------------------------------------------------------------------------
# Installer assests 
#
# Package the  Pulumi project files and manifests into the runtime image. 
# -----------------------------------------------------------------------------

# App Serving
COPY app_serving/deployments/Pulumi.yaml \
    /opt/shaide-installer/projects/app-serving/Pulumi.yaml
# app-serving resolves model values as <project>/deployments/models/<category>/<name>
# (config.go resolveModelPaths), so the deployments/ level must be preserved.
COPY app_serving/deployments/models/ \
    /opt/shaide-installer/projects/app-serving/deployments/models/
COPY app_serving/charts/ \
    /opt/shaide-installer/projects/app-serving/charts/

# App Shaide
COPY app_shaide/deployments/Pulumi.yaml \
    /opt/shaide-installer/projects/app-shaide/Pulumi.yaml


# Harbor
COPY harbor/deployment/Pulumi.yaml \
    /opt/shaide-installer/projects/harbor/Pulumi.yaml
COPY --from=chart-downloader \
    /opt/shaide-installer/projects/harbor/charts/ \
    /opt/shaide-installer/projects/harbor/charts/


# Gateway Provider
COPY infra/gateway-provider/deployments/Pulumi.yaml \
    /opt/shaide-installer/projects/gateway-provider/Pulumi.yaml
COPY infra/gateway-provider/crds/ \
    /opt/shaide-installer/projects/gateway-provider/crds/

# Monitoring
COPY monitoring/deployments/Pulumi.yaml \
    /opt/shaide-installer/projects/monitoring/Pulumi.yaml
COPY --from=chart-downloader \
    /opt/shaide-installer/projects/monitoring/charts/ \
    /opt/shaide-installer/projects/monitoring/charts/

# Manifests
# Only the image manifest is copied, the models are provided at runtime.
COPY installer/build/manifests/images.yaml \
    /opt/shaide-installer/manifests/images.yaml

CMD ["/bin/installer"]
