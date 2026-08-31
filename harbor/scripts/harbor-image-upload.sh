#!/usr/bin/env bash
# ==============================================================
# harbor-image-upload.sh
# Copies container images from public registries to Harbor via
# kubectl port-forward. Requires internet access on the provisioner.
#
# Source of truth for image list: mirrors
#   infra/on-prem/ansible/group_vars/all/images.yml
# Keep both files in sync when adding or bumping images.
#
# MetalLB and GPU Operator sections are commented out — GKE manages
# load balancing and GPU drivers natively. Uncomment if deploying
# this Harbor on a bare-metal / self-managed cluster.
#
# Prerequisites:
#   - skopeo installed on the provisioner
#   - kubectl configured for the target cluster
#   - Harbor running and projects created (harbor-setup.sh already run)
#   - gh CLI installed and authenticated (gh auth login) — required to pull
#     private images from ghcr.io/axem-solutions
#
# Usage:
#   HARBOR_ROBOT_PASSWORD=<robot-password> bash scripts/harbor-image-upload.sh
#
# With explicit kubeconfig:
#   KUBECONFIG=~/.kube/<stack-name>.yaml \
#   HARBOR_ROBOT_PASSWORD=<robot-password> \
#   bash scripts/harbor-image-upload.sh
# ==============================================================
set -euo pipefail

HARBOR_LOCAL_PORT="${HARBOR_LOCAL_PORT:-5000}"
HARBOR_HOST="localhost:${HARBOR_LOCAL_PORT}"
HARBOR_ROBOT_USER="${HARBOR_ROBOT_USER:-robot\$k8s-harbor-sa}"

if [[ -z "${HARBOR_ROBOT_PASSWORD:-}" ]]; then
  echo "ERROR: HARBOR_ROBOT_PASSWORD env var is required." >&2
  exit 1
fi

# ── kubectl args ──────────────────────────────────────────────────────────────
KUBECTL_ARGS=()
if [[ -n "${KUBECONFIG:-}" ]]; then
  KUBECTL_ARGS+=(--kubeconfig "${KUBECONFIG}")
  echo "Kubeconfig: ${KUBECONFIG}"
fi

# ── Port-forward ──────────────────────────────────────────────────────────────
echo "==> Starting port-forward: localhost:${HARBOR_LOCAL_PORT} → harbor svc/harbor:80"
kubectl "${KUBECTL_ARGS[@]}" port-forward -n harbor svc/harbor "${HARBOR_LOCAL_PORT}:80" \
  > /tmp/harbor-portforward-image-upload.log 2>&1 &
PF_PID=$!
trap 'kill "${PF_PID}" 2>/dev/null || true; echo "Port-forward closed."' EXIT
sleep 3

# ── Source registry login ─────────────────────────────────────────────────────
echo "==> Logging in to ghcr.io via gh CLI..."
skopeo login ghcr.io \
  --username "$(gh api user --jq .login)" \
  --password "$(gh auth token)"


# ── Image list ────────────────────────────────────────────────────────────────
# Format: "public-source|harbor-project/repository:tag"
IMAGES=(

  # ── MetalLB (not needed on GKE — uncomment for bare-metal clusters) ────────
  # "quay.io/metallb/controller:v0.15.3|images-infra/metallb/controller:v0.15.3"
  # "quay.io/metallb/speaker:v0.15.3|images-infra/metallb/speaker:v0.15.3"
  # "quay.io/frrouting/frr:10.4.1|images-infra/frrouting/frr:10.4.1"

  # ── GPU Operator (not needed on GKE — uncomment for bare-metal clusters) ───
  # "nvcr.io/nvidia/gpu-operator:v25.10.1|images-infra/nvcr.io/nvidia/gpu-operator:v25.10.1"
  # "nvcr.io/nvidia/cuda:13.0.1-base-ubi9|images-infra/nvcr.io/nvidia/cuda:13.0.1-base-ubi9"
  # "nvcr.io/nvidia/driver:580.105.08-ubuntu24.04|images-infra/nvcr.io/nvidia/driver:580.105.08-ubuntu24.04"
  # "nvcr.io/nvidia/cloud-native/k8s-driver-manager:v0.9.1|images-infra/nvcr.io/nvidia/cloud-native/k8s-driver-manager:v0.9.1"
  # "nvcr.io/nvidia/k8s/container-toolkit:v1.18.1|images-infra/nvcr.io/nvidia/k8s/container-toolkit:v1.18.1"
  # "nvcr.io/nvidia/k8s-device-plugin:v0.18.1|images-infra/nvcr.io/nvidia/k8s-device-plugin:v0.18.1"
  # "nvcr.io/nvidia/k8s/dcgm-exporter:4.4.2-4.7.0-distroless|images-infra/nvcr.io/nvidia/k8s/dcgm-exporter:4.4.2-4.7.0-distroless"
  # "nvcr.io/nvidia/cloud-native/dcgm:4.4.2-1-ubuntu22.04|images-infra/nvcr.io/nvidia/cloud-native/dcgm:4.4.2-1-ubuntu22.04"
  # "nvcr.io/nvidia/cloud-native/k8s-mig-manager:v0.13.1|images-infra/nvcr.io/nvidia/cloud-native/k8s-mig-manager:v0.13.1"
  # "registry.k8s.io/nfd/node-feature-discovery:v0.18.2|images-infra/registry.k8s.io/nfd/node-feature-discovery:v0.18.2"

  # ── Verification / test ───────────────────────────────────────────────────
  "nginx:alpine|images-infra/nginx:alpine"
  "alpine:3.20|images-infra/alpine:3.20"
  "python:3.12-slim|images-infra/python:3.12-slim"

  # ── Shaide application ────────────────────────────────────────────────────
  "ghcr.io/axem-solutions/shaide_server:v0.7.0|images-shaide/shaide_server:v0.7.0"
  "ghcr.io/axem-solutions/control_panel:v0.3.0|images-shaide/control_panel:v0.3.0"
  "rustfs/rustfs:1.0.0-alpha.92|images-shaide/rustfs/rustfs:1.0.0-alpha.92"
  "qdrant/qdrant:v1.17|images-shaide/qdrant/qdrant:v1.17"
  "busybox:1.37|images-infra/busybox:1.37"

  # ── Istio (gateway-provider stack) ───────────────────────────────────────
  "docker.io/istio/pilot:1.28.1|images-shaide/pilot:1.28.1"
  "docker.io/istio/proxyv2:1.28.1|images-shaide/proxyv2:1.28.1"

  # ── llm-d inference stack (app_serving) ──────────────────────────────────
  "ghcr.io/llm-d/llm-d-routing-sidecar:v0.4.0-rc.1|images-shaide/llm-d/llm-d-routing-sidecar:v0.4.0-rc.1"
  "ghcr.io/llm-d/llm-d-cuda:v0.4.0|images-shaide/llm-d/llm-d-cuda:v0.4.0"
  "registry.k8s.io/gateway-api-inference-extension/epp:v1.2.0|images-shaide/gateway-api-inference-extension/epp:v1.2.0"

  # ── ORAS (model pull jobs) ────────────────────────────────────────────────
  "ghcr.io/oras-project/oras:v1.3.1|images-infra/oras-project/oras:v1.3.1"
)

# ── Copy ──────────────────────────────────────────────────────────────────────
echo "==> Uploading ${#IMAGES[@]} images to ${HARBOR_HOST}..."
echo ""

FAILED=()
for entry in "${IMAGES[@]}"; do
  src="${entry%%|*}"
  dest_path="${entry##*|}"

  echo "  ${src}"
  echo "  → ${dest_path}"

  # Use stored ghcr.io credentials for private images; pull all other registries
  # anonymously to avoid stale/expired credentials from the local auth store.
  src_creds=()
  if [[ "${src}" != ghcr.io/* ]]; then
    src_creds+=(--src-no-creds)
  fi

  src_digest=$(skopeo inspect "${src_creds[@]}" "docker://${src}" --format '{{.Digest}}' 2>/dev/null || true)
  dest_digest=$(skopeo inspect \
    --tls-verify=false \
    --creds "${HARBOR_ROBOT_USER}:${HARBOR_ROBOT_PASSWORD}" \
    "docker://${HARBOR_HOST}/${dest_path}" \
    --format '{{.Digest}}' 2>/dev/null || true)

  if [[ -n "${src_digest}" && "${src_digest}" == "${dest_digest}" ]]; then
    echo "  ↷ already up-to-date, skipping"
    echo ""
    continue
  fi

  if skopeo copy \
    "${src_creds[@]}" \
    --dest-tls-verify=false \
    --dest-username "${HARBOR_ROBOT_USER}" \
    --dest-password "${HARBOR_ROBOT_PASSWORD}" \
    "docker://${src}" \
    "docker://${HARBOR_HOST}/${dest_path}"; then
    echo "  ✓ done"
  else
    echo "  ✗ FAILED"
    FAILED+=("${src}")
  fi
  echo ""
done

# ── Summary ───────────────────────────────────────────────────────────────────
if [[ ${#FAILED[@]} -eq 0 ]]; then
  echo "==> All images uploaded successfully."
else
  echo "==> ${#FAILED[@]} image(s) failed:" >&2
  for f in "${FAILED[@]}"; do
    echo "    - ${f}" >&2
  done
  exit 1
fi
