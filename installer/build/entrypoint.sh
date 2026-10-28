#!/bin/sh
# Copies the cloud logins mounted read-only under /host-config into the
# container's home before starting the installer. EKS and GKE kubeconfigs get
# their tokens from the aws CLI and gke-gcloud-auth-plugin, which read these
# logins and refresh their caches in them. Working on a copy means a refresh
# never writes root-owned files into the user's home directory.
set -eu

copy_login() {
    src="$1"
    dst="$2"
    [ -d "$src" ] || return 0
    mkdir -p "$dst"
    # gcloud keeps logs next to its login; they are not needed and can be large.
    tar -C "$src" --exclude=./logs -cf - . | tar -C "$dst" -xf -
}

copy_login /host-config/aws "$HOME/.aws"
copy_login /host-config/gcloud "$HOME/.config/gcloud"

exec "$@"
