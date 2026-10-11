"""Read-only vLLM discovery extension loaded through --middleware.

Register a route inside the existing router so authentication and all other
vLLM middleware still apply. No inference calls, environment dumps, or full
configuration serialization are performed.
"""

from collections.abc import Mapping

from starlette.responses import JSONResponse
from starlette.routing import Router

METADATA_PATH = "/v1/shaide/metadata"


def field(value, name, default=None):
    return value.get(name, default) if isinstance(value, Mapping) else getattr(value, name, default)


def positive(value):
    return value if type(value) is int and value > 0 else None


def names(value):
    values = [value] if isinstance(value, str) else value
    if not isinstance(values, (list, tuple)):
        return []
    return [name for name in values if isinstance(name, str) and name.strip()]


def snapshot(state):
    config = field(field(state, "vllm_config"), "model_config")
    if config is None:
        return None
    # Prefer the names actually registered by vLLM, never expose a weights path.
    paths = field(field(state, "openai_serving_models"), "base_model_paths", [])
    served = names([field(path, "name") for path in paths])
    if not served:
        served = names(field(field(state, "args"), "served_model_name"))
    if not served:
        served = names(field(config, "served_model_name"))
    model = {"served_model_name": served}
    context = positive(field(config, "max_model_len"))
    if context is not None:
        model["max_model_len"] = context
    cap = positive(field(field(config, "override_generation_config"), "max_new_tokens"))
    if cap is not None:
        model["override_generation_config"] = {"max_new_tokens": cap}
    dimensions = positive(field(field(config, "pooler_config"), "dimensions"))
    if dimensions is not None:
        model["pooler_config"] = {"dimensions": dimensions}
    multimodal = field(config, "multimodal_config")
    if multimodal is not None:
        image = field(field(multimodal, "limit_per_prompt", field(multimodal, "limit_mm_per_prompt")), "image")
        count = image if type(image) is int else field(image, "count")
        mm = {"language_model_only": field(multimodal, "language_model_only", False) is True}
        if type(count) is int and count >= 0:
            mm["limit_per_prompt"] = {"image": count}
        model["multimodal_config"] = mm
        if field(field(config, "hf_config"), "vision_config") is not None:
            model["hf_config"] = {"vision_config": {}}
    return {"schema_version": 1, "vllm_config": {"model_config": model}}


async def metadata(request):
    try:
        payload = snapshot(request.app.state)
    except (AttributeError, TypeError, ValueError):
        payload = None
    if payload is None:
        return JSONResponse({"error": "Runtime model metadata is not available."}, status_code=503)
    return JSONResponse(payload, headers={"Cache-Control": "no-store"})


class RuntimeMetadataMiddleware:
    def __init__(self, app):
        self.app = app
        router = app
        # FastAPI builds the middleware chain around its APIRouter before
        # serving requests. Add the route there, not in an outer interceptor
        # that could skip the existing authentication middleware.
        while not isinstance(router, Router) and hasattr(router, "app"):
            router = router.app
        if not isinstance(router, Router):
            raise RuntimeError("shaide metadata requires a Starlette-compatible router")
        if any(getattr(route, "path", None) == METADATA_PATH for route in router.routes):
            raise RuntimeError("shaide metadata route is already registered")
        router.add_route(METADATA_PATH, metadata, methods=["GET"], include_in_schema=False)

    async def __call__(self, scope, receive, send):
        await self.app(scope, receive, send)
