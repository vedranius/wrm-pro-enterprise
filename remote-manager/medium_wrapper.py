"""Authenticated ASGI boundary for the privately installed original deploy tool."""
import asyncio
import hashlib
import hmac
import json
import os
import stat
import sys

# Container aliases preserve the Docker CLI's host HOME/context. Translate only
# the explicitly forwarded runtime values before loading the original app.
container_runtime = False
for key, value in list(os.environ.items()):
    if key.startswith('WRM_MEDIUM_ENV_'):
        container_runtime = True
        os.umask(0o077)
        os.environ[key[len('WRM_MEDIUM_ENV_'):]] = value
        del os.environ[key]

if container_runtime:
    import pwd
    # OpenSSH expands ~ against passwd, not HOME. Make standard ~/.ssh profiles
    # resolve to this operator's private Linux volume without mounting host keys.
    account_home = pwd.getpwuid(os.geteuid()).pw_dir
    os.makedirs(account_home, mode=0o700, exist_ok=True)
    os.chmod(account_home, 0o700)
    ssh_home = os.path.join(account_home, '.ssh')
    if not os.path.lexists(ssh_home):
        os.symlink(os.path.join(os.environ['HOME'], '.ssh'), ssh_home)

# The original application's modules and dependencies remain unchanged.
sys.path.insert(0, os.environ['WRM_MEDIUM_TOOL'])
from app.main import app as original_app


active_requests = 0
MANIFEST_LIMIT = 200000


def _escape(path):
    try:
        from app.remote import escape_path
    except ImportError:  # older tool versions
        return path.encode('utf-8', 'surrogateescape').decode('utf-8', 'replace')
    return escape_path(path)


def _manifest(root):
    """Files, directories and symlinks of the prepared source, for WRM's native engine."""
    out = []
    root = os.fspath(root)
    for current, dirs, files in os.walk(root):
        dirs.sort()
        for name in sorted(dirs) + sorted(files):
            full = os.path.join(current, name)
            rel = os.path.relpath(full, root).replace(os.sep, '/')
            info = os.lstat(full)
            entry = {'path': _escape(rel)}
            if stat.S_ISLNK(info.st_mode):
                entry.update(type='symlink', link=_escape(os.readlink(full)))
            elif stat.S_ISDIR(info.st_mode):
                entry['type'] = 'dir'
            elif stat.S_ISREG(info.st_mode):
                digest = hashlib.sha256()
                with open(full, 'rb') as handle:
                    for chunk in iter(lambda: handle.read(1 << 20), b''):
                        digest.update(chunk)
                entry.update(type='file', size=info.st_size, sha256=digest.hexdigest())
            else:
                entry['type'] = 'other'
            out.append(entry)
            if len(out) > MANIFEST_LIMIT:
                raise ValueError(f'more than {MANIFEST_LIMIT} items in the prepared source')
    return out


def _change(change):
    return {'path': _escape(change.path), 'action': change.action, 'kind': change.kind,
            'link_target': _escape(change.link_target) if change.link_target else None,
            'itemize': change.itemize}


def _wrm_plan(request):
    """The original comparison (fetch, export, build, rsync --dry-run) as JSON."""
    service = original_app.state.compare_service
    comparison = service.run(request['project'], request['environment'], request['ref'],
                             request.get('servers') or None)
    servers = []
    for item in comparison.servers:
        current = item.current.model_dump(mode='json') if item.current is not None else None
        servers.append({'server': item.server, 'path': item.path, 'error': item.error,
                        'warnings': list(item.warnings), 'excludes': list(item.excludes),
                        'changes': [_change(c) for c in item.changes], 'current': current})
    return {'ref': comparison.ref, 'commit': comparison.commit, 'version': comparison.version,
            'build': comparison.build, 'warnings': list(comparison.warnings),
            'confirm': comparison.confirm, 'ref_allowed': comparison.ref_allowed,
            'excludes': list(comparison.excludes), 'servers': servers,
            'source': _manifest(comparison.source_dir)}


async def app(scope, receive, send):
    global active_requests
    if scope['type'] != 'http':
        return await original_app(scope, receive, send)
    supplied = dict(scope.get('headers', [])).get(b'x-wrm-medium-key', b'')
    expected = os.environ['WRM_MEDIUM_KEY'].encode()
    if not hmac.compare_digest(supplied, expected):
        await send({'type': 'http.response.start', 'status': 403,
                    'headers': [(b'content-type', b'text/plain')]})
        await send({'type': 'http.response.body', 'body': b'WRM authorization required'})
        return
    if scope['path'] == '/__wrm_ready':
        await send({'type': 'http.response.start', 'status': 200, 'headers': []})
        await send({'type': 'http.response.body', 'body': b'ready'})
        return
    if scope['path'] == '/__wrm_state':
        jobs = original_app.state.deploy_jobs
        with jobs._lock:
            running = sum(1 for job in jobs._jobs.values() if job.running)
        payload = json.dumps({'busy': active_requests > 0 or running > 0}).encode()
        await send({'type': 'http.response.start', 'status': 200,
                    'headers': [(b'content-type', b'application/json')]})
        await send({'type': 'http.response.body', 'body': payload})
        return
    if scope['path'] == '/__wrm_plan' and scope['method'] == 'POST':
        body = b''
        while True:
            message = await receive()
            body += message.get('body', b'')
            if not message.get('more_body') or len(body) > 65536:
                break
        active_requests += 1
        try:
            result = await asyncio.to_thread(_wrm_plan, json.loads(body))
        except Exception as exc:  # the original's messages (CompareError, ConfigError) are for the operator
            result = {'error': str(exc) or type(exc).__name__}
        finally:
            active_requests -= 1
        payload = json.dumps(result).encode()
        await send({'type': 'http.response.start', 'status': 200,
                    'headers': [(b'content-type', b'application/json')]})
        await send({'type': 'http.response.body', 'body': payload})
        return
    active_requests += 1
    try:
        return await original_app(scope, receive, send)
    finally:
        active_requests -= 1
