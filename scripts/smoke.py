#!/usr/bin/env python3
"""Exercise the local Compose stack, including a temporary MySQL outage."""
import argparse
import json
import subprocess
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--base-url', default='http://127.0.0.1:8080')
args = parser.parse_args()


def compose(*arguments):
    result = subprocess.run(['docker', 'compose', *arguments], capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('Compose operation failed: ' + arguments[0])
    return result.stdout.strip()


def request(path, expected):
    try:
        response = urllib.request.urlopen(args.base_url + path, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        assert response.status == expected, f'{path}: expected {expected}, got {response.status}'
        assert response.headers.get('X-Request-ID'), 'missing request ID'
        if path.startswith('/health/'):
            assert response.headers.get('Cache-Control') == 'no-store'
            state = 'ok' if expected == 200 else 'unavailable'
            assert json.loads(body) == {'status': state}, 'unexpected health payload'
        else:
            assert body, 'empty response'


for path in ['/', '/static/app.css', '/static/htmx.min.js', '/static/sse.js', '/health/live', '/health/ready']:
    request(path, 200)
print('OK: page, local assets and health endpoints')
app_id = compose('ps', '-q', 'app')
try:
    compose('stop', 'mysql')
    request('/health/live', 200)
    request('/health/ready', 503)
    print('OK: DB outage returns ready=503 and live=200')
finally:
    compose('up', '-d', 'mysql', '--wait', '--wait-timeout', '120')

limit = time.monotonic() + 15
while True:
    try:
        request('/health/ready', 200)
        break
    except (AssertionError, urllib.error.URLError):
        if time.monotonic() >= limit:
            raise
        time.sleep(0.25)
assert compose('ps', '-q', 'app') == app_id, 'app was unexpectedly recreated'
print('OK: readiness recovered without recreating app')
compose('stop', 'app')
exit_code = subprocess.check_output(['docker', 'inspect', '--format', '{{.State.ExitCode}}', app_id], text=True).strip()
assert exit_code == '0', 'app did not stop cleanly'
compose('up', '-d', 'app', '--wait', '--wait-timeout', '60')
request('/health/ready', 200)
print('OK: SIGTERM shutdown and subsequent startup')
