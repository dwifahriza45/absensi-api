#!/usr/bin/env python3
"""Writes synthetic users/attendance: run against an isolated test stack only."""
import argparse
import collections
import concurrent.futures
import http.cookies
import json
import math
import os
import secrets
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url', default='http://127.0.0.1:8082')
    parser.add_argument('--count', type=int, default=1500)
    parser.add_argument('--workers', type=int, default=64)
    parser.add_argument('--window-seconds', type=float, default=60)
    parser.add_argument('--photo', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--admin-email', default='admin@absensi.local')
    parser.add_argument('--allow-writes', action='store_true')
    parser.add_argument('--allow-remote', action='store_true')
    args = parser.parse_args()
    if not args.allow_writes:
        parser.error('--allow-writes is required: this creates test records and files')
    if not (1 <= args.count <= 5000 and 1 <= args.workers <= 256 and args.window_seconds >= 0):
        parser.error('count 1..5000, workers 1..256, window-seconds >= 0')
    if urllib.parse.urlparse(args.base_url).hostname not in ('localhost', '127.0.0.1', '::1') and not args.allow_remote:
        parser.error('remote targets require --allow-remote and an isolated test deployment')
    password = os.environ.get('LOADTEST_ADMIN_PASSWORD')
    if not password:
        parser.error('set LOADTEST_ADMIN_PASSWORD')
    photo = Path(args.photo).read_bytes()
    if len(photo) > 5 * 1024 * 1024:
        parser.error('photo exceeds 5 MiB')
    base = args.base_url.rstrip('/')

    def request(path, body=None, cookies='', content_type='application/json'):
        if isinstance(body, dict):
            body = json.dumps(body).encode()
        headers = {'Content-Type': content_type}
        if cookies:
            headers['Cookie'] = cookies
        req = urllib.request.Request(base + path, data=body, headers=headers)
        started = time.perf_counter()
        try:
            with urllib.request.urlopen(req, timeout=40) as res:
                return res.status, res.read(), res.headers, time.perf_counter() - started
        except urllib.error.HTTPError as err:
            return err.code, err.read(), err.headers, time.perf_counter() - started
        except (OSError, TimeoutError) as err:
            return 'transport_error', type(err).__name__.encode(), {}, time.perf_counter() - started

    def cookies_from(headers):
        parsed = http.cookies.SimpleCookie()
        for value in headers.get_all('Set-Cookie', []):
            parsed.load(value)
        return '; '.join(k + '=' + v.value for k, v in parsed.items())

    status, _, headers, _ = request('/api/v1/admin/auth/login', {'email': args.admin_email, 'password': password})
    if status != 200:
        raise SystemExit('Admin login failed: ' + str(status))
    admin_cookies = cookies_from(headers)
    run = secrets.randbelow(1_000_000)
    rows = [{'name': f'Load {run:06d} Employee {i:05d}', 'phone': f'08{run:06d}{i:05d}', 'mitra': 'Load Test'} for i in range(args.count)]
    status, _, _, _ = request('/api/v1/admin/employees/import', {'rows': rows, 'deactivateMissing': False}, admin_cookies)
    if status != 200:
        raise SystemExit('Import failed: ' + str(status))

    def phase(items, operation, window=0):
        started = time.perf_counter()
        def worker(index_item):
            index, item = index_item
            delay = started + window * index / len(items) - time.perf_counter()
            if delay > 0:
                time.sleep(delay)
            return operation(item)
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as pool:
            results = list(pool.map(worker, enumerate(items)))
        elapsed = time.perf_counter() - started
        latency = sorted(r[3] * 1000 for r in results)
        counts = collections.Counter(str(r[0]) for r in results)
        summary = {'requests': len(results), 'seconds': round(elapsed, 3), 'requests_per_second': round(len(results) / elapsed, 2),
                   'statuses': dict(counts), 'p50_ms': round(latency[math.ceil(len(latency)*.5)-1], 2),
                   'p95_ms': round(latency[math.ceil(len(latency)*.95)-1], 2), 'max_ms': round(max(latency), 2)}
        print(json.dumps(summary), flush=True)
        return results, summary

    logins, login_summary = phase(rows, lambda row: request('/api/v1/karyawan/auth/login', {'fullname': row['name'], 'phone': row['phone']}))
    sessions = [cookies_from(result[2]) for result in logins if result[0] == 200]
    if not sessions:
        raise SystemExit('No employee logins succeeded')
    boundary = 'AbsensiLoadBoundary'
    body = ''.join('--'+boundary+'\r\nContent-Disposition: form-data; name="'+k+'"\r\n\r\n'+v+'\r\n' for k,v in {'lat':'-6.2','lng':'106.8','accuracy':'10'}.items()).encode()
    body += ('--'+boundary+'\r\nContent-Disposition: form-data; name="selfie"; filename="synthetic.jpg"\r\nContent-Type: image/jpeg\r\n\r\n').encode() + photo + ('\r\n--'+boundary+'--\r\n').encode()
    _, attendance_summary = phase(sessions, lambda cookie: request('/api/v1/karyawan/attendances/check-in', body, cookie, 'multipart/form-data; boundary='+boundary), args.window_seconds)
    health, _, _, _ = request('/api/v1/admin/health')
    report = {'target': base, 'count': args.count, 'workers': args.workers, 'arrival_window_seconds': args.window_seconds,
              'photo_bytes': len(photo), 'login': login_summary, 'check_in': attendance_summary, 'health_after': health,
              'notes': 'Local test only. No automatic retries. Latency excludes scheduled waiting; throughput includes arrival spacing. Not a VPS capacity guarantee.'}
    Path(args.output).write_text(json.dumps(report, indent=2)+'\n')
    if login_summary['statuses'] != {'200': args.count} or attendance_summary['statuses'] != {'201': args.count} or health != 200:
        raise SystemExit('Some requests failed; see report (429/503 indicate backpressure, not successful attendance)')

if __name__ == '__main__':
    main()
