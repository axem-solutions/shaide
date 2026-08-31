---
title: "Provisioner Prerequisites"
description: "What the installer needs before you run it."
weight: 10
---

# Provisioner Prerequisites

Cluster-side requirements are covered in
[Cluster requirements](../cluster-requirements/overview.md). This page covers the
provisioning machine and the credentials the installer needs.

## Provisioning machine

The Linux host that runs the installer container.

| Requirement | Detail |
| --- | --- |
| Docker | Installed and usable by the current user (`docker info`) |
| Disk | 100 GB free, in a directory preserved across runs |
| Terminal | Interactive - the installer is a TUI and requires `-it` |
| Cluster access | kubeconfig with `cluster-admin` |

Create the state directory:

```bash
mkdir -p /var/lib/shaide-installer
```

This must be preserved across re-runs and upgrades - it holds installer state.

## Files

| File | Purpose |
| --- | --- |
| `models.yaml` | Models to publish into the internal registry |
| Installer image | Pulled from `ghcr.io` |

See [Model manifest](../installation/installer-guide.md#model-manifest).

## Credentials

Have these ready before starting:

| Credential | Purpose |
| --- | --- |
| Installer state passphrase | Encrypts installer state. **Reuse the same value on every run** |
| Hugging Face token | Model downloads. Needs read access to selected models |
| Registry admin password | Administers the internal registry |
| Registry robot password | Image push/pull. Store for future updates |
| shaide admin password | shaide administrator account |
| SSH private key | For on-premises image preload only |

Losing the state passphrase means the installer cannot read its previous state - keep it
with your other platform secrets.

## Next

[Quickstart](quickstart.md)
