import unittest
from types import SimpleNamespace as Obj

from fastapi import FastAPI
from starlette.responses import JSONResponse
from starlette.testclient import TestClient

from shaide_metadata import METADATA_PATH, RuntimeMetadataMiddleware


class Authentication:
    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope['type'] == 'http' and scope['path'].startswith('/v1'):
            if dict(scope['headers']).get(b'authorization') != b'Bearer test-key':
                await JSONResponse({'error': 'unauthorized'}, status_code=401)(scope, receive, send)
                return
        await self.app(scope, receive, send)


def application(config=None):
    app = FastAPI()
    if config is not None:
        app.state.vllm_config = Obj(model_config=config, secret='never-expose')
    app.state.args = Obj(served_model_name=['served-alias'], api_key='never-expose')

    @app.get('/v1/models')
    def models():
        return {'data': [{'id': 'served-alias'}]}

    app.add_middleware(Authentication)
    # Same order as vLLM: the extension is outside Authentication.
    app.add_middleware(RuntimeMetadataMiddleware)
    return app


class MetadataTests(unittest.TestCase):
    def test_authentication_and_existing_routes_are_preserved(self):
        with TestClient(application(Obj(max_model_len=100000))) as client:
            self.assertEqual(client.get(METADATA_PATH).status_code, 401)
            client.headers['Authorization'] = 'Bearer test-key'
            self.assertEqual(client.get('/v1/models').json()['data'][0]['id'], 'served-alias')
            self.assertEqual(client.post(METADATA_PATH).status_code, 405)
            self.assertEqual(client.delete(METADATA_PATH).status_code, 405)
            self.assertEqual(client.get('/server_info').status_code, 404)
            self.assertEqual(client.post('/sleep').status_code, 404)
            self.assertEqual(client.post('/collective_rpc').status_code, 404)

    def test_only_allowlisted_runtime_fields_are_returned(self):
        config = Obj(model='/private/weights', tokenizer='/private/tokenizer',
                     max_model_len=100000, served_model_name=['config-alias'],
                     override_generation_config={'max_new_tokens': 4096, 'secret': 'never-expose'},
                     multimodal_config=Obj(limit_per_prompt={'image': Obj(count=2)}, language_model_only=False),
                     hf_config=Obj(vision_config={'secret': 'never-expose'}, hidden_size=4096),
                     pooler_config=Obj(dimensions=768), secret='never-expose')
        app = application(config)
        app.state.openai_serving_models = Obj(base_model_paths=[Obj(name='actual-name', model_path='/private/weights')])
        with TestClient(app) as client:
            response = client.get(METADATA_PATH, headers={'Authorization': 'Bearer test-key'})
            self.assertEqual(response.status_code, 200)
            self.assertEqual(response.headers['cache-control'], 'no-store')
            self.assertEqual(response.json(), {'schema_version': 1, 'vllm_config': {'model_config': {
                'served_model_name': ['actual-name'], 'max_model_len': 100000,
                'override_generation_config': {'max_new_tokens': 4096},
                'pooler_config': {'dimensions': 768},
                'multimodal_config': {'language_model_only': False, 'limit_per_prompt': {'image': 2}},
                'hf_config': {'vision_config': {}},
            }}})
            self.assertNotIn('never-expose', response.text)
            self.assertNotIn('/private', response.text)
            # Read from current state on every request, not a YAML/startup snapshot.
            config.max_model_len = 8192
            self.assertEqual(client.get(METADATA_PATH, headers={'Authorization': 'Bearer test-key'}).json()['vllm_config']['model_config']['max_model_len'], 8192)

    def test_missing_dimensions_are_not_inferred(self):
        config = Obj(max_model_len=8192, hf_config=Obj(hidden_size=4096))
        with TestClient(application(config)) as client:
            response = client.get(METADATA_PATH, headers={'Authorization': 'Bearer test-key'})
            self.assertNotIn('pooler_config', response.json()['vllm_config']['model_config'])

    def test_uninitialized_runtime_is_unavailable(self):
        with TestClient(application()) as client:
            response = client.get(METADATA_PATH, headers={'Authorization': 'Bearer test-key'})
            self.assertEqual(response.status_code, 503)
            self.assertEqual(response.json(), {'error': 'Runtime model metadata is not available.'})

    def test_text_only_and_disabled_images(self):
        for mm in [None, Obj(limit_per_prompt={'image': 0}), Obj(limit_per_prompt={'image': 4}, language_model_only=True)]:
            config = Obj(max_model_len=8192, multimodal_config=mm)
            with TestClient(application(config)) as client:
                model = client.get(METADATA_PATH, headers={'Authorization': 'Bearer test-key'}).json()['vllm_config']['model_config']
                self.assertNotIn('hf_config', model)
                if mm is None:
                    self.assertNotIn('multimodal_config', model)


if __name__ == '__main__':
    unittest.main()
