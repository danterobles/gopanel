#!/usr/bin/env python3
"""Check seeded role permissions against a native Go process without changing business data."""
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


credentials = json.loads(Path('.local/seed-credentials.json').read_text())
with socket.socket() as listener:
    listener.bind(('127.0.0.1', 0))
    port = listener.getsockname()[1]
base = f'http://127.0.0.1:{port}'
env = os.environ.copy()
env['HTTP_ADDR'] = f'127.0.0.1:{port}'
client = None


def request(path, expected=200, fields=None):
    data = urllib.parse.urlencode(fields).encode() if fields is not None else None
    req = urllib.request.Request(base + path, data=data)
    try:
        response = client.open(req, timeout=8)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read().decode()
        assert response.status == expected, f'{path}: unexpected HTTP {response.status}'
        for row in credentials:
            assert row['password'] not in body, 'credential appeared in HTML'
        return body


def csrf(body):
    match = re.search(r'name="csrf_token" value="([^"]+)"', body)
    assert match, 'missing CSRF'
    return match.group(1)


def login(email, tenant=None):
    global client
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()), NoRedirect())
    row = next(row for row in credentials if row['email'] == email)
    body = request('/login')
    request('/login', 303, {'csrf_token': csrf(body), 'email': row['email'], 'password': row['password']})
    body = request('/app/tenants/select')
    if tenant:
        options = re.findall(r'<option value="([0-9a-f-]{36})">([^<]+)</option>', body)
        selected = next(value for value, label in options if tenant in label.lower())
        request('/app/tenants/activate', 303, {'csrf_token': csrf(body), 'tenant_id': selected})
        body = request('/app')
    return csrf(body)


with tempfile.TemporaryFile(mode='w+') as logs:
    process = subprocess.Popen(['./bin/gopanel'], env=env, stdout=logs, stderr=logs)
    try:
        client = urllib.request.build_opener(NoRedirect())
        deadline = time.monotonic() + 15
        while True:
            try:
                request('/health/ready')
                break
            except urllib.error.URLError:
                assert process.poll() is None and time.monotonic() < deadline, 'native server failed to start'
                time.sleep(0.05)
        token = login('shared@gopanel.test', 'norte')
        body = request('/app/members')
        assert 'Agregar miembro existente' not in body, 'Admin sees management form'
        request('/app/settings', 403)
        customers = request('/app/customers?search=Cliente&page_size=10&sort=name_asc')
        assert 'Nuevo cliente' in customers and '@norte.test' in customers and '@sur.test' not in customers, 'Admin customer view is not scoped to Norte'
        products = request('/app/products?page_size=10&sort=price_asc')
        assert 'Nuevo producto' in products and 'MXN' in products, 'Admin products unavailable'
        request('/app/members', 403, {'csrf_token': token, 'email': 'platform@gopanel.test', 'role': 'viewer'})
        request('/logout', 303, {'csrf_token': token})
        token = login('shared@gopanel.test', 'sur')
        request('/app/members', 403)
        customers = request('/app/customers?page_size=10')
        assert 'Nuevo cliente' not in customers and '@sur.test' in customers and '@norte.test' not in customers, 'Viewer customer view is not scoped to Sur'
        request('/app/customers/new', 403, {'csrf_token': token, 'name': 'must not create', 'status': 'active', 'role': 'owner'})
        products = request('/app/products?page_size=10')
        assert 'Nuevo producto' not in products and 'MXN' in products, 'Viewer product controls are not protected'
        request('/app/products/new', 403, {'csrf_token': token, 'sku': 'MUST-NOT-CREATE', 'name': 'forged', 'price': '1.00', 'currency': 'MXN', 'status': 'active'})
        request('/logout', 303, {'csrf_token': token})
        token = login('owner.norte@gopanel.test', 'norte')
        body = request('/app/members')
        assert 'Agregar miembro existente' in body, 'Owner cannot manage members'
        request('/app/settings')
        request('/app/customers/new')
        request('/app/products/new')
        request('/app/settings', 403, {'name': 'must not apply'})
        request('/logout', 303, {'csrf_token': token})
        token = login('platform@gopanel.test')
        body = request('/platform')
        assert 'Métricas técnicas' in body and 'empresa-norte' in body, 'platform administration unavailable'
        request('/app/members', 303)
        request('/logout', 303, {'csrf_token': token})
    finally:
        process.send_signal(signal.SIGTERM)
        try:
            process.wait(timeout=15)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
            raise RuntimeError('native RBAC shutdown timed out') from None
    assert process.returncode == 0, 'native process failed'
    logs.seek(0)
    output = logs.read()
    for secret in [row['password'] for row in credentials] + [env.get('DB_PASSWORD', '')]:
        assert not secret or secret not in output, 'credential appeared in logs'
    for line in output.splitlines():
        json.loads(line)
print('OK: native Owner/Admin/Viewer, tenant-scoped customers/products, independent platform access, CSRF and HTML management pages; business data unchanged')
