---
title: "Air-gapped installation"
description: "Installing on a cluster with no internet access."
weight: 25
---

# Air-gapped installation

The **cluster** runs with no internet access. Every container image and model weight it
needs is served from the internal registry inside the cluster.

The **provisioning machine** has internet access. It reaches the origin
registries and Hugging Face, and pushes what it fetches into the internal registry.

## How it works

1. The bundle is transferred to the provisioning machine. It carries the Pulumi
   deployments, manifests, charts, CRDs, and Harbor's bootstrap image archives.
2. The installer deploys Harbor into the cluster, preloading its bootstrap images onto the
   Harbor node over SSH.
3. The installer pulls service and application images from their origin registries, and
   model weights from Hugging Face, then pushes both into the internal registry.
4. Cluster nodes pull exclusively from that registry.

## Requirements

| Item | Detail |
| --- | --- |
| Provisioning machine | Reachable: origin registries and Hugging Face |
| Bundle | Deployments, manifests, and Harbor bootstrap archives |
| Registry trust | Nodes must trust the internal registry's CA |
| Storage | Capacity for all images and model weights |
| Credentials | `HF_TOKEN`, plus registry tokens for any private images |

## Registry trust

Nodes must trust the internal registry before they can pull. Untrusted nodes fail with
`ImagePullBackOff` or x509 errors. See
[Node registry trust](../operations/node-registry-trust.md).

## Verifying isolation

```bash
kubectl get pods -A -o jsonpath='{range .items[*]}{.spec.containers[*].image}{"\n"}{end}' \
  | sort -u
```

Every image should reference the internal registry. Any `docker.io`, `ghcr.io` or
`quay.io` reference will fail on a disconnected cluster.
