#!/usr/bin/env python3
"""Run the native Go binary against local MySQL; never stop or mutate MySQL."""
import json
import os
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def port():
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        return listener.getsockname()[1]


def request(base, path, expected):
    try:
        response = urllib.request.urlopen(base + path, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        assert response.status == expected, f'{path}: expected {expected}, got {response.status}'
        assert response.headers.get('X-Request-ID'), 'missing request ID'
        if path.startswith('/health/'):
            assert response.headers.get('Cache-Control') == 'no-store'
            assert json.loads(body) == {'status': 'ok' if expected == 200 else 'unavailable'}
        else:
            assert body, 'empty asset/page'


def exercise(database_available):
    env = os.environ.copy()
    http_port = port()
    env['HTTP_ADDR'] = f'127.0.0.1:{http_port}'
    if not database_available:
        env['DB_PORT'] = str(port())  # Unused endpoint, does not stop the real MySQL server.
    base = f'http://127.0.0.1:{http_port}'
    with tempfile.TemporaryFile(mode='w+') as logs:
        process = subprocess.Popen(['./bin/gopanel'], env=env, stdout=logs, stderr=logs)
        try:
            deadline = time.monotonic() + 15
            while True:
                try:
                    request(base, '/health/live', 200)
                    break
                except urllib.error.URLError:
                    if process.poll() is not None or time.monotonic() >= deadline:
                        raise RuntimeError('native server failed to start') from None
                    time.sleep(0.05)
            request(base, '/health/ready', 200 if database_available else 503)
            if database_available:
                for path in ['/', '/static/app.css', '/static/htmx.min.js', '/static/sse.js']:
                    request(base, path, 200)
        finally:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=15)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise RuntimeError('native server failed to shut down') from None
        assert process.returncode == 0, 'native server exited abnormally'
        logs.seek(0)
        output = logs.read()
        assert not env.get('DB_PASSWORD') or env['DB_PASSWORD'] not in output, 'credential in native logs'
        for line in output.splitlines():
            json.loads(line)


exercise(True)
print('OK: native page/assets, live/ready and SIGTERM against local MySQL')
exercise(False)
print('OK: unavailable endpoint returns live=200/ready=503; local MySQL was not stopped')
