#!/usr/bin/env bash
set -euo pipefail

# Build and run the installer image with the paths and environment contract
# consumed by installer/internal/config. The installer itself is interactive;
# this wrapper deliberately keeps the terminal attached with docker run -it.

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if [[ -f "$script_dir/../../build/Dockerfile" ]]; then
  # Scripts intentionally live inside installer/installer-bundle/scripts.
  repo_root=$(cd "$script_dir/../../.." && pwd)
else
  printf 'error: cannot locate installer/build/Dockerfile from %s\n' "$script_dir" >&2
  exit 1
fi

image_name=${INSTALLER_IMAGE:-installer:latest}
host_bundle=${HOST_BUNDLE_ARCHIVE:-${BUNDLE_ARCHIVE:-$repo_root/installer/installer-bundle/bundle.tar.gz}}
host_kubeconfig=${HOST_KUBECONFIG:-${KUBECONFIG:-${HOME}/.kube/config}}
storage_path=${INSTALLER_STORAGE_PATH:-$repo_root/shaide-installer-data}
host_ssh_key=${HOST_SSH_KEY:-${SSH_PRIVATE_KEY:-}}
host_gcloud_config=${HOST_GCLOUD_CONFIG:-${GCLOUD_CONFIG:-}}
container_ssh_key=${CONTAINER_PRIVATE_KEY_PATH:-${PRIVATE_KEY_PATH:-/root/.ssh/id_ed25519}}
build_image=true
run_image=true
extra_docker_args=()

usage() {
  cat <<'EOF'
Usage:
  deployment.sh [options] [-- docker-run-args...]

Default entrypoint:
  ./installer/installer-bundle/scripts/deployment.sh

Builds installer/build/Dockerfile from the repository root and runs the
interactive installer container with the required mounts.

Options:
  --image NAME                 Image tag (default: installer:latest)
  --bundle PATH                Host bundle archive
  --kubeconfig PATH            Host kubeconfig
  --storage PATH               Persistent installer storage directory
  --ssh-key PATH               Host SSH private key for Harbor image preload
  --gcloud-config PATH         Host gcloud config directory (optional)
  --no-build                   Run an existing image without building it
  --build-only                 Build the image and do not run it
  -h, --help                   Show this help

Environment passed through when set:
  HF_TOKEN GHCR_USERNAME GHCR_TOKEN DOCKERHUB_USERNAME DOCKERHUB_PASSWORD
  PULUMI_CONFIG_PASSPHRASE

Optional image build arguments:
  PULUMI_VERSION PULUMI_KUBERNETES_PLUGIN_VERSION

Host-side defaults can also be supplied with HOST_BUNDLE_ARCHIVE,
HOST_KUBECONFIG, INSTALLER_STORAGE_PATH, HOST_SSH_KEY, and
HOST_GCLOUD_CONFIG (or their corresponding command-line options). Set
CONTAINER_PRIVATE_KEY_PATH or PRIVATE_KEY_PATH to change the mounted key path
inside the container.

Examples:
  # Build and run the checked-in example bundle:
  ./installer/installer-bundle/scripts/build-bundle.sh \
    installer/installer-bundle/example \
    installer/installer-bundle/example.tar.gz
  ./installer/installer-bundle/scripts/deployment.sh \
    --bundle installer/installer-bundle/example.tar.gz

  # Run the default bundle path (installer/installer-bundle/bundle.tar.gz):
  ./installer/installer-bundle/scripts/deployment.sh
  ./installer/installer-bundle/scripts/deployment.sh --no-build --bundle ./bundle.tar.gz
  ./installer/installer-bundle/scripts/deployment.sh --ssh-key ~/.ssh/id_ed25519
  ./installer/installer-bundle/scripts/deployment.sh -- --env LOG_LEVEL=debug
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

absolute_path() {
  local value=$1
  if [[ "$value" = /* ]]; then
    printf '%s\n' "$value"
  else
    printf '%s/%s\n' "$PWD" "$value"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --image)
      [[ $# -ge 2 ]] || die "--image requires a value"
      image_name=$2
      shift 2
      ;;
    --bundle)
      [[ $# -ge 2 ]] || die "--bundle requires a value"
      host_bundle=$2
      shift 2
      ;;
    --kubeconfig)
      [[ $# -ge 2 ]] || die "--kubeconfig requires a value"
      host_kubeconfig=$2
      shift 2
      ;;
    --storage)
      [[ $# -ge 2 ]] || die "--storage requires a value"
      storage_path=$2
      shift 2
      ;;
    --ssh-key)
      [[ $# -ge 2 ]] || die "--ssh-key requires a value"
      host_ssh_key=$2
      shift 2
      ;;
    --gcloud-config)
      [[ $# -ge 2 ]] || die "--gcloud-config requires a value"
      host_gcloud_config=$2
      shift 2
      ;;
    --no-build)
      build_image=false
      shift
      ;;
    --build-only)
      run_image=false
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      extra_docker_args=("$@")
      break
      ;;
    *)
      die "unknown option: $1 (use --help for usage)"
      ;;
  esac
done

require_command docker

if [[ "$run_image" == true ]]; then
  host_bundle=$(absolute_path "$host_bundle")
  host_kubeconfig=$(absolute_path "$host_kubeconfig")
  storage_path=$(absolute_path "$storage_path")

  [[ -f "$host_bundle" ]] || die "bundle archive not found: $host_bundle (build it with installer/installer-bundle/scripts/build-bundle.sh)"
  [[ -r "$host_bundle" ]] || die "bundle archive is not readable: $host_bundle"
  [[ -f "$host_kubeconfig" ]] || die "kubeconfig not found: $host_kubeconfig"
  [[ -r "$host_kubeconfig" ]] || die "kubeconfig is not readable: $host_kubeconfig"

  if [[ -n "$host_ssh_key" ]]; then
    [[ "$container_ssh_key" = /* ]] || die "container SSH key path must be absolute: $container_ssh_key"
    host_ssh_key=$(absolute_path "$host_ssh_key")
    [[ -f "$host_ssh_key" ]] || die "SSH private key not found: $host_ssh_key"
    [[ -r "$host_ssh_key" ]] || die "SSH private key is not readable: $host_ssh_key"
  fi

  if [[ -n "$host_gcloud_config" ]]; then
    host_gcloud_config=$(absolute_path "$host_gcloud_config")
    [[ -d "$host_gcloud_config" ]] || die "gcloud config directory not found: $host_gcloud_config"
  fi

  mkdir -p "$storage_path"
  [[ -t 0 && -t 1 ]] || die "the installer is interactive; run this script from a terminal"
fi

if [[ "$build_image" == true ]]; then
  build_args=()
  if [[ -n "${PULUMI_VERSION:-}" ]]; then
    build_args+=(--build-arg "PULUMI_VERSION=${PULUMI_VERSION}")
  fi
  if [[ -n "${PULUMI_KUBERNETES_PLUGIN_VERSION:-}" ]]; then
    build_args+=(--build-arg "PULUMI_KUBERNETES_PLUGIN_VERSION=${PULUMI_KUBERNETES_PLUGIN_VERSION}")
  fi

  docker build \
    "${build_args[@]}" \
    -f "$repo_root/installer/build/Dockerfile" \
    -t "$image_name" \
    "$repo_root"
fi

[[ "$run_image" == true ]] || exit 0

docker_args=(
  run --rm -it
  --network host
  -e KUBECONFIG=/.kube/config
  -e BUNDLE_ARCHIVE_PATH=/.bundle/bundle.tar.gz
  -v "${host_kubeconfig}:/.kube/config:ro"
  -v "${host_bundle}:/.bundle/bundle.tar.gz:ro"
  --mount "type=bind,src=${storage_path},dst=/var/shaide-installer"
)

pass_env=(
  HF_TOKEN
  GHCR_USERNAME
  GHCR_TOKEN
  DOCKERHUB_USERNAME
  DOCKERHUB_PASSWORD
  PULUMI_CONFIG_PASSPHRASE
)
for variable in "${pass_env[@]}"; do
  if [[ -n "${!variable:-}" ]]; then
    # Pass the variable by name so its secret value is not repeated in the
    # docker CLI argument list visible to process inspection tools.
    docker_args+=(-e "$variable")
  fi
done

if [[ -n "$host_ssh_key" ]]; then
  docker_args+=(
    -e "PRIVATE_KEY_PATH=${container_ssh_key}"
    -v "${host_ssh_key}:${container_ssh_key}:ro"
  )
fi

if [[ -n "$host_gcloud_config" ]]; then
  docker_args+=(
    -e CLOUDSDK_CONFIG=/root/.config/gcloud
    -v "${host_gcloud_config}:/root/.config/gcloud"
  )
fi

docker_args+=("${extra_docker_args[@]}" "$image_name")
exec docker "${docker_args[@]}"
