---
title: "Installer CLI"
description: "Installer invocation, mounts and environment variables."
weight: 30
---

# Installer CLI

The installer runs as a container and is driven interactively.

```bash
docker run --rm -it \
  --network host \
  -e PULUMI_CONFIG_PASSPHRASE \
  -v "$HOME/.kube/config:/.kube/config:ro" \
  -v "$PWD/bundle.tar.gz:/.bundle/bundle.tar.gz:ro" \
  --mount "type=bind,src=$PWD/shaide-installer-data,dst=/var/shaide-installer" \
  ghcr.io/axem-solutions/shaide/installer:latest
```

## Docker flags

| Flag | Purpose |
| --- | --- |
| `--rm -it` | Interactive TUI; required |
| `--network host` | Reach the cluster over the host network |
| `-v .../.kube/config` | Cluster access, read-only |
| `-v .../bundle.tar.gz` | Installation payload, read-only |
| `--mount ...` | Persistent state; must be reused across runs |

## Mount paths

| Container path | Contents |
| --- | --- |
| `/.kube/config` | kubeconfig |
| `/.bundle/bundle.tar.gz` | Bundle archive |
| `/var/shaide-installer` | Persistent state and extracted bundle |

## Environment variables

| Variable | Purpose |
| --- | --- |
| `PULUMI_CONFIG_PASSPHRASE` | Encrypts installer state. Reuse across runs |
| `HF_TOKEN` | Hugging Face token, when pulling weights directly |
| `PRIVATE_KEY_PATH` | SSH key path, for node image preload |

Configuration keys are documented in
[Configuration reference](configuration.md) and
[Stack configuration](../installation/configuration.md).
