---
title: "Model management"
description: "Adding, swapping and removing served models."
weight: 10
---

# Model management

## Listing

```bash
curl https://<endpoint>/v1/models -H "Authorization: Bearer <key>"
kubectl get pods -A -l app.kubernetes.io/part-of=app-serving,llm-d.ai/role=decode
```

## Adding

1. Pin the Hugging Face revision in the model's `model.yaml`; the installer
   publishes the weights to the internal registry - see [Model registry](model-registry.md).
2. Add the model definition under the serving stack's model deployments.
3. Reference it in the stack configuration.
4. Apply the serving stack.

Convention-based discovery: a `model.yaml` in `deployments/models/<category>/<model>/` is
picked up automatically once referenced in stack config.

Detail: [Model deployment flow](../architecture/model-deployment-flow.md).

## Swapping on a single GPU

Where GPU capacity allows only one large model, scale the outgoing model to zero before
scheduling its replacement - otherwise the new pod stays `Pending` on insufficient VRAM.

## Removing

Remove the model from stack configuration and re-apply. Weights remain in the internal
registry until deleted there.

## Verifying

```bash
kubectl -n shaide-serving get pods -w
curl https://<endpoint>/v1/models -H "Authorization: Bearer <key>"
```

A new model appears in the API list once at least one replica is ready.
