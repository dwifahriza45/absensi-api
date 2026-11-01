#!/usr/bin/env python3
"""Set the seeded admin password over Compose stdin without shell/history secrets."""
import getpass
from pathlib import Path
import subprocess

password = getpass.getpass('Password baru admin (minimal 12 karakter): ')
if len(password) < 12 or password == 'Admin123!':
    raise SystemExit('Gunakan password baru minimal 12 karakter.')
if password != getpass.getpass('Ulangi password: '):
    raise SystemExit('Password tidak cocok.')
# SQL literal escaping; no shell interpolation and no command-line password.
literal = "'" + password.replace("'", "''") + "'"
sql = "UPDATE users SET password_hash = crypt(" + literal + ", gen_salt('bf', 10)) WHERE email = 'admin@absensi.local' AND role = 'admin' RETURNING email;\n"
subprocess.run(['docker', 'compose', '--env-file', '.env.vps', '-f', 'docker-compose.vps.yml', 'exec', '-T', 'db', 'sh', '-c', 'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'], input=sql, text=True, cwd=Path(__file__).resolve().parents[1], check=True)
