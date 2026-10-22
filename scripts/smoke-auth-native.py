#!/usr/bin/env python3
"""Use private development seed credentials to test auth across a native Go restart."""
import http.cookiejar
import json
import os
from pathlib import Path
import re
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


with socket.socket() as listener:
    listener.bind(('127.0.0.1', 0))
    port = listener.getsockname()[1]
base = f'http://127.0.0.1:{port}'
env = os.environ.copy()
env['HTTP_ADDR'] = f'127.0.0.1:{port}'
credentials = json.loads(Path('.local/seed-credentials.json').read_text())
credential = next(row for row in credentials if row['email'] == 'shared@gopanel.test')
jar = http.cookiejar.CookieJar()
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect())


def request(path, expected=200, fields=None):
    data = urllib.parse.urlencode(fields).encode() if fields is not None else None
    req = urllib.request.Request(base + path, data=data)
    try:
        response = client.open(req, timeout=8)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read().decode()
        assert response.status == expected, f'{path}: unexpected status {response.status}'
        assert credential['password'] not in body, 'password appeared in HTML'
        return body


def csrf(body):
    match = re.search(r'name="csrf_token" value="([^"]+)"', body)
    assert match, 'missing CSRF field'
    return match.group(1)


def start(logs):
    process = subprocess.Popen(['./bin/gopanel'], env=env, stdout=logs, stderr=logs)
    try:
        deadline = time.monotonic() + 15
        while True:
            try:
                request('/health/live')
                return process
            except urllib.error.URLError:
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError('native server failed to start') from None
                time.sleep(0.05)
    except BaseException:
        stop(process)
        raise


def stop(process):
    if process is not None:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
            raise RuntimeError('native auth shutdown timed out') from None
        assert process.returncode == 0, 'native auth process exited abnormally'


process = None
with tempfile.TemporaryFile(mode='w+') as logs:
    try:
        process = start(logs)
        login = request('/login')
        request('/login', 303, {'csrf_token': csrf(login), 'email': credential['email'], 'password': credential['password']})
        selector = request('/app/tenants/select')
        options = re.findall(r'<option value="([0-9a-f-]{36})">([^<]+)</option>', selector)
        north = next(value for value, label in options if 'norte' in label)
        south = next(value for value, label in options if 'sur' in label)
        request('/app/tenants/activate', 303, {'csrf_token': csrf(selector), 'tenant_id': north})
        active = request('/app')
        assert 'Empresa norte' in active and 'admin' in active
        before_restart_csrf = csrf(active)
        stop(process)
        process = None
        process = start(logs)
        recovered = request('/app')
        assert 'Empresa norte' in recovered and csrf(recovered) == before_restart_csrf, 'session not recovered from MySQL'
        request('/app/tenants/activate', 303, {'csrf_token': csrf(recovered), 'tenant_id': south, 'role': 'owner'})
        active = request('/app')
        assert 'Empresa sur' in active and 'viewer' in active
        request('/logout', 403, {'csrf_token': before_restart_csrf})
        request('/logout', 303, {'csrf_token': csrf(active)})
        request('/app', 303)
    finally:
        stop(process)
    logs.seek(0)
    output = logs.read()
    for secret in [credential['password'], env.get('DB_PASSWORD', '')]:
        assert not secret or secret not in output, 'credential appeared in logs'
    for line in output.splitlines():
        json.loads(line)
print('OK: native login, tenant selection/rotation, process restart with persistent session, stale CSRF rejection and logout')
