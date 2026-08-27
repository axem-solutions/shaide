---
title: "Quickstart"
description: "Install shaide and make your first API call."
weight: 20
---

# Quickstart

## 1. Check the cluster

Confirm your cluster meets [the requirements](../cluster-requirements/index.md):

```bash
kubectl auth can-i '*' '*'
kubectl get nodes
kubectl describe node <gpu-node> | grep -A2 "nvidia.com/gpu"
kubectl get storageclass
```

## 2. Set the environment variables

| Variable | Required | Purpose |
| --- | --- | --- |
| `PULUMI_CONFIG_PASSPHRASE` | Yes | Encrypts installer state. Reuse the same value on every run |
| `HF_TOKEN` | Yes | Downloads model weights from Hugging Face |
| `GHCR_TOKEN` | No | Only needed if using private images |
| `PRIVATE_KEY_PATH` | On-prem | Path **inside the container** to the SSH key used for Harbor image preload |
| `CLOUDSDK_CONFIG` | No | Lets the GKE auth plugin work inside the container |

```bash
export PULUMI_CONFIG_PASSPHRASE="<passphrase>"
export HF_TOKEN="<token>"
```

> Store the passphrase with your other platform secrets. Without it the installer cannot
> read its previous state on upgrades.

## 3. Run the installer

```bash
docker run --rm -it \
  --network host \
  -e PULUMI_CONFIG_PASSPHRASE \
  -e HF_TOKEN \
  -v "$HOME/.kube/config:/.kube/config:ro" \
  -v "$PWD/bundle.tar.gz:/.bundle/bundle.tar.gz:ro" \
  --mount "type=bind,src=$PWD/shaide-installer-data,dst=/var/shaide-installer" \
  ghcr.io/axem-solutions/shaide/installer:latest
```

The installer prompts for configuration and deploys the platform. 
Installation can take some time, becase model weights must be uploaded to the internal Harbor 
registry and pulled onto GPU nodes.

Full walkthrough: [Installer guide](../installation/installer-guide.md).

## 4. Verify

```bash
curl https://<endpoint>/v1/models -H "Authorization: Bearer <key>"
```

Returns the models currently served. More checks in
[Verify installation](verify-installation.md).

## 5. First request

```python
from openai import OpenAI

client = OpenAI(base_url="https://<endpoint>/v1", api_key="<key>")

response = client.chat.completions.create(
    model="<model-id>",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(response.choices[0].message.content)
```

## Next

- [OpenAI-compatible API](../usage/openai-api.md)
- [Agent integrations](../usage/agent-integrations.md)
- [Architecture](../architecture/overview.md)
