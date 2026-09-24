---
title: "Installation troubleshooting"
description: "Problems during installation."
weight: 50
---

# Installation troubleshooting

## Installer will not start

| Symptom | Fix |
| --- | --- |
| Terminal errors | Add `-it` - the installer is interactive |
| Permission denied on Docker | Add the user to the `docker` group |
| Cannot find the model manifest | Check the mount path and `MODEL_MANIFEST_PATH` |

## State

**Cannot read previous state** - the passphrase or state directory does not match the
original install. Both are required; without them the installer treats it as a fresh
deployment.

Confirm the mount points at the same host directory used previously and that
`PULUMI_CONFIG_PASSPHRASE` is exported.

**The stack state belongs to another cluster** - a stack's state records the cluster
it was deployed to, and it does not match the selected Kubernetes context. Deploying
would move every resource to the selected cluster and delete the originals from the
recorded one, so the installer stops before changing anything. Use the state directory
of the selected cluster, or move that stack's state aside so the run starts fresh.

Before deploying a stack, the installer also asks for confirmation (default `Abort`) when:

| Prompt | Meaning |
| --- | --- |
| `...deployed without a verifiable cluster` | The state was written by an older installer that did not record the context, or with a context the kubeconfig no longer defines |
| `...will first delete N resources left pending` | An interrupted update left resources pending deletion; the next update deletes them first. The installer log lists each one and its cluster |

## Manifest validation

The installer validates the image manifest that ships inside the image, and the model
manifest you supply. A missing or unreadable model manifest fails at bootstrap with a
message naming the expected path.

## Cluster connectivity

```bash
kubectl version
kubectl auth can-i '*' '*'
```

`--network host` is required so the container reaches the cluster over the host network.

## Deployment failures

Failures during apply usually indicate an unmet cluster requirement:

| Failure | Cause |
| --- | --- |
| PVCs never bind | No default StorageClass |
| Serving pods `Pending` | No GPU capacity or missing node labels |
| No ingress address | No LoadBalancer provider |
| Image pulls fail | Registry trust not configured |

Re-run the installer after fixing - it is idempotent and resumes from current state.

See also [Troubleshooting](../operations/troubleshooting.md).
