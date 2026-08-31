---
title: "Upgrades"
description: "Upgrading the platform and rolling back."
weight: 30
---

# Upgrades

## Platform

Re-run the installer with a newer bundle, reusing the same state directory and
passphrase:

```bash
STORAGE_PATH=<storage-path>
PULUMI_CONFIG_PASSPHRASE=<choose-a-passphrase>
HF_TOKEN=<your-huggingface-token>

mkdir -p "${STORAGE_PATH}"

docker run --rm -it \
  --network host \
  -e PULUMI_CONFIG_PASSPHRASE \
  -e HF_TOKEN \
  -v "$HOME/.kube/config:/.kube/config:ro" \
  --mount "type=bind,src=${STORAGE_PATH},dst=/var/shaide-installer" \
  ghcr.io/axem-solutions/shaide/installer:dev
```

The installer diffs desired against current state and applies only what changed.

> The state directory and passphrase **must** match the original install. Without them
> the installer cannot read previous state and will attempt a fresh deployment.

## Before upgrading

- Read the release notes for breaking changes.
- Back up installer state and platform data - see [Backup and restore](backup-restore.md).
- Upgrade non-production first.

## Kubernetes

Upgrade the cluster through your provider, or with the RKE2 upgrade playbook on-prem.
Keep every node on the same version - shaide does not support mixed-version clusters.

## Rollback

Re-run the installer with the previous bundle. Rollback is not always clean: schema or
storage-format changes may not reverse. Restore from backup when in doubt.
