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
        ./cmd/onprem-installer


# -----------------------------------------------------------------------------
# Runtime stage
#
# Use a smaller Debian image containing only the compiled installer and the
# external command-line tools required at runtime.
# -----------------------------------------------------------------------------
FROM debian:bookworm-slim AS final

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

CMD ["/bin/installer"]
