---
title: "Backup and restore"
description: "What to back up and how to recover."
weight: 40
---

# Backup and restore

## What to back up

| Item | Location | Why |
| --- | --- | --- |
| Installer state | Provisioning machine state directory | Required for upgrades |
| State passphrase | Your secret manager | Without it state is unreadable |
| Application database | shaide server PVC | Users, keys, configuration |
| Vector database | Qdrant PVC | Index data |
| Object storage | Object storage PVC | Uploaded artifacts |
| Registry contents | Registry PVC | Images and model weights |

Registry contents are reproducible from their origin registries and Hugging Face, so
they are the lowest priority.

## Volumes

```bash
kubectl get pvc -A
```

Snapshot through your storage provider (EBS, PD, Azure Disk snapshots), or with
`VolumeSnapshot` where the CSI driver supports it.

## Installer state

```bash
tar czf shaide-installer-state-$(date +%F).tar.gz -C /var/lib/shaide-installer .
```

Store alongside the passphrase - neither is useful alone.

## Restore

1. Restore volume snapshots.
2. Restore the installer state directory.
3. Re-run the installer with the original passphrase to reconcile.

Test restores on a non-production cluster before you need them.
