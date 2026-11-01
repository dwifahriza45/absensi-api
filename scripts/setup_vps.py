#!/usr/bin/env python3
"""Create private VPS configuration once; never overwrite existing secrets."""
import argparse
import os
from pathlib import Path
import secrets

parser = argparse.ArgumentParser()
parser.add_argument('--dockerhub-user', required=True)
parser.add_argument('--tag', required=True, help='sha-<full commit SHA> from GitHub Actions')
args = parser.parse_args()
if not args.dockerhub_user.replace('-', '').replace('_', '').isalnum():
    parser.error('Invalid Docker Hub username')
if not args.tag.startswith('sha-') or len(args.tag) != 44 or any(c not in '0123456789abcdef' for c in args.tag[4:]):
    parser.error('Tag must be sha- followed by the full 40-character lowercase commit SHA')
root = Path(__file__).resolve().parents[1]
path = root / '.env.vps'
if path.exists():
    parser.error('.env.vps already exists; edit it directly to preserve credentials')
values = {
    'JWT_SECRET': secrets.token_hex(32),
    'PHONE_HASH_SECRET': secrets.token_hex(32),
    'DB_PASSWORD': secrets.token_hex(24),
    'MINIO_ROOT_PASSWORD': secrets.token_hex(32),
    'MINIO_APP_ACCESS_KEY': secrets.token_hex(12),
    'MINIO_APP_SECRET_KEY': secrets.token_hex(32),
    'DOCKER_IMAGE': args.dockerhub_user + '/absensi-api',
    'IMAGE_TAG': args.tag,
    'MINIO_IMAGE': args.dockerhub_user + '/absensi-minio:' + args.tag,
    'MINIO_MC_IMAGE': args.dockerhub_user + '/absensi-minio-mc:' + args.tag,
}
values['S3_ACCESS_KEY_ID'] = values['MINIO_APP_ACCESS_KEY']
values['S3_SECRET_ACCESS_KEY'] = values['MINIO_APP_SECRET_KEY']
content = (root / 'ops/vps.env.example').read_text()
content = '\n'.join(line.split('=', 1)[0] + '=' + values[line.split('=', 1)[0]] if '=' in line and line.split('=', 1)[0] in values else line for line in content.splitlines()) + '\n'
fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
with os.fdopen(fd, 'w') as stream:
    stream.write(content)
print('Created private .env.vps with random secrets. Keep it in your disaster recovery backup.')
