#!/usr/bin/env python3
"""Test the private ASGI boundary without installing the private deploy tool."""
import asyncio
import importlib.util
import json
import os
from pathlib import Path
import sys
import threading
import tempfile
import types
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True


class MediumWrapperTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.entered = asyncio.Event()
        self.resume = asyncio.Event()
        self.jobs = types.SimpleNamespace(_lock=threading.Lock(), _jobs={})

        async def original(scope, receive, send):
            self.entered.set()
            await self.resume.wait()
            await send({'type': 'http.response.start', 'status': 200, 'headers': []})
            await send({'type': 'http.response.body', 'body': b'original response'})

        original.state = types.SimpleNamespace(deploy_jobs=self.jobs)
        fake = types.ModuleType('app.main')
        fake.app = original
        self.old_module = sys.modules.get('app.main')
        self.old_app = sys.modules.get('app')
        sys.modules['app'] = types.ModuleType('app')
        sys.modules['app.main'] = fake
        self.old_env = dict(os.environ)
        os.environ['WRM_MEDIUM_TOOL'] = '/not-used'
        os.environ['WRM_MEDIUM_KEY'] = 'private-runtime-key'
        source = Path(__file__).resolve().parents[1] / 'remote-manager' / 'medium_wrapper.py'
        spec = importlib.util.spec_from_file_location('wrm_medium_wrapper_test', source)
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)

    def tearDown(self):
        for key, old in [('app.main', self.old_module), ('app', self.old_app)]:
            if old is None:
                sys.modules.pop(key, None)
            else:
                sys.modules[key] = old
        os.environ.clear()
        os.environ.update(self.old_env)

    async def test_container_aliases_and_private_ssh_home(self):
        import pwd
        with tempfile.TemporaryDirectory() as directory:
            account_home = str(Path(directory) / 'account')
            private_home = str(Path(directory) / 'private-volume')
            original_mask = os.umask(0o077)
            os.umask(original_mask)
            try:
                with patch.object(pwd, 'getpwuid', return_value=types.SimpleNamespace(pw_dir=account_home)), patch.dict(os.environ, {
                    'WRM_MEDIUM_ENV_HOME': private_home,
                    'WRM_MEDIUM_ENV_WRM_MEDIUM_TOOL': '/not-used',
                    'WRM_MEDIUM_ENV_WRM_MEDIUM_KEY': 'aliased-runtime-key',
                }):
                    spec = importlib.util.spec_from_file_location('wrm_alias_test', self.module.__file__)
                    module = importlib.util.module_from_spec(spec)
                    spec.loader.exec_module(module)
                    self.assertEqual(os.environ['HOME'], private_home)
                    self.assertEqual(os.environ['WRM_MEDIUM_KEY'], 'aliased-runtime-key')
                    self.assertNotIn('WRM_MEDIUM_ENV_WRM_MEDIUM_KEY', os.environ)
                    self.assertEqual(os.readlink(Path(account_home) / '.ssh'), str(Path(private_home) / '.ssh'))
            finally:
                os.umask(original_mask)

    async def request(self, path, key='private-runtime-key'):
        output = []

        async def send(message):
            output.append(message)

        async def receive():
            return {'type': 'http.request', 'body': b'', 'more_body': False}

        await self.module.app({'type': 'http', 'path': path,
                               'headers': [(b'x-wrm-medium-key', key.encode())]}, receive, send)
        return output

    async def test_authentication_and_readiness(self):
        denied = await self.request('/__wrm_ready', 'wrong-key')
        self.assertEqual(denied[0]['status'], 403)
        ready = await self.request('/__wrm_ready')
        self.assertEqual(ready[0]['status'], 200)
        self.assertEqual(ready[1]['body'], b'ready')

    async def test_delegation_and_busy_request(self):
        task = asyncio.create_task(self.request('/usporedba'))
        await self.entered.wait()
        busy = await self.request('/__wrm_state')
        self.assertTrue(json.loads(busy[1]['body'])['busy'])
        self.resume.set()
        response = await task
        self.assertEqual(response[1]['body'], b'original response')
        idle = await self.request('/__wrm_state')
        self.assertFalse(json.loads(idle[1]['body'])['busy'])

    async def test_background_job_prevents_stop(self):
        self.jobs._jobs['job'] = types.SimpleNamespace(running=True)
        result = await self.request('/__wrm_state')
        self.assertTrue(json.loads(result[1]['body'])['busy'])
        self.jobs._jobs['job'].running = False
        result = await self.request('/__wrm_state')
        self.assertFalse(json.loads(result[1]['body'])['busy'])


if __name__ == '__main__':
    unittest.main()
