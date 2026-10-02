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
| A model is missing from the selector | The log at the start of `Select models` names each skipped model directory and why, usually a missing `shaide.revision` |

## State

**Cannot read previous state** - the passphrase or state directory does not match the
original install. Both are required; without them the installer treats it as a fresh
deployment.

Confirm the mount points at the same host directory used previously and that
`PULUMI_CONFIG_PASSPHRASE` is exported.

**The stack state belongs to another cluster** - a stack's state records the cluster
it was deployed to, and it does not match the selected Kubernetes context. For state
written by an older installer, which did not record the context, the installer compares
the recorded object UIDs with the selected cluster instead: objects that exist there
only under different UIDs mean the state is another cluster's. Deploying
would move every resource to the selected cluster and delete the originals from the
recorded one, so the installer stops before changing anything. Use the state directory
of the selected cluster, or move that stack's state aside so the run starts fresh.

Before deploying a stack, the installer also asks for confirmation (default `Abort`) when:

| Prompt | Meaning |
| --- | --- |
| `...deployed without a verifiable cluster` | The state records no usable context (an older installer, or a context the kubeconfig no longer defines), and no recorded object could be compared on the selected cluster: they are missing or unreadable. When one of them is live with its recorded UID, the stack is verified without asking. The installer log names the objects it checked |
| `...will first delete N resources left pending` | An interrupted update left resources pending deletion; the next update deletes them first. The installer log lists each one and its cluster |

## Existing Harbor

When the installer finds a Harbor it did not deploy, it switches to update mode and
creates any of its projects that are missing (`ai-models`, `shaide`, `services`, and
any others the manifests name) as public projects. It needs the Harbor admin password
for that. It reads the password from the `harbor-core` secret in the Harbor namespace,
and asks for it if that secret is missing or out of date.

Harbor rejects a push into a project that does not exist with `401 Unauthorized`. If an
upload still fails that way, the installer checks the project first: when the project
is missing it offers to create it, and it only asks for new credentials when the
project exists.

## Manifest validation

The image manifest and the model catalog both ship inside the installer image. If either
is missing or unreadable, the installer fails at bootstrap with a message naming the
path; this indicates a broken image build.

A packaged model that fails validation is not offered, and the run continues. The reason
is logged at the start of the `Select models` stage, for example a missing or duplicate
`ms-<slug>/values.yaml`, an unpinned `shaide.revision`, a missing
`modelArtifacts.name` or `modelArtifacts.size`, or a decode GPU limit that is not a
whole number.

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
