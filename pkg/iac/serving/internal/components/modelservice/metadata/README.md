# Read-only runtime model metadata

The platform mounts `shaide_metadata.py` from an immutable, content-addressed
ConfigMap and loads `shaide_metadata.RuntimeMetadataMiddleware` with vLLM's
`--middleware` option. The module registers `GET /v1/shaide/metadata` inside the
existing FastAPI router. Authentication and other existing middleware still
apply; POST/PUT/DELETE are not supported. No sidecar or additional Service is
needed. It uses the same model-serving port and gateway route as `/v1/models`.

Enable this per model in its modelservice values file:

```yaml
shaide:
  runtimeMetadata:
    enabled: true
```

This flag enables the endpoint; it is not a second model-settings catalog.
The response reads `app.state.vllm_config.model_config` on every request and
prefers the public names registered in `app.state.openai_serving_models`.
Existing vLLM CLI flags in values.yaml continue to configure the model. The
values file need not be available inside the serving container.

Only public served names, runtime context size, an explicit generation cap,
image support/count hints, and explicit pooling dimensions are returned under
`schema_version: 1` / `vllm_config.model_config`. Environment variables, system
information, API keys, file paths, and unrelated model configuration are never
serialized. Unsupported or missing fields are omitted. An uninitialized runtime
returns HTTP 503; absent embedding dimensions are reported as an error by
shaide-server, without inference probes or guesses from hidden size.

The platform explicitly sets `VLLM_SERVER_DEV_MODE=0` and rejects a conflicting
deployment setting. The extension does not enable `/server_info`, sleep/cache,
or collective-RPC APIs. Keep model-serving Services private; shaide-server's
administrator-only discovery API is the Control Panel entry point. If vLLM
requires an API key, metadata requires it too; shaide-server currently does not
forward credentials to these endpoints and reports them unavailable.

The integration preserves container arguments, environment, volumes, resource
limits, other containers, and disabled prefill workloads. It operates only on
`modelCommand: vllmServe` containers in explicitly opted-in models. The module's
content hash changes the ConfigMap reference, triggering a pod rollout when
code changes. Deployments use the existing Recreate strategy for single-GPU
models. The installer embeds the module via Go's `go:embed`, so rebuilding the
installer includes it without copying a separate source directory at runtime.

GPT-OSS-20B opts in. Other compatible vLLM models can enable the same flag;
non-vLLM serving processes need their own implementation of the versioned
endpoint. No separate metadata values are required for generative models.
Embedders must explicitly configure actual output dimensions in vLLM's pooler
configuration if their runtime would otherwise omit them.

Run tests with the same FastAPI/Starlette versions as llm-d-cuda:v0.7.0:

```sh
uv venv /tmp/shaide-metadata-venv
uv pip install --python /tmp/shaide-metadata-venv/bin/python -r requirements-test.txt
/tmp/shaide-metadata-venv/bin/python -m unittest discover -s . -v
```

Tests construct a real FastAPI/Starlette middleware stack with runtime state;
they require neither a GPU nor any inference requests.
