#!/usr/bin/env python3
"""Set the seeded admin password over Compose stdin without shell/history secrets."""
import argparse
import getpass
from pathlib import Path
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--email', default='admin@absensi.local')
args = parser.parse_args()
email_literal = "'" + args.email.replace("'", "''") + "'"

password = getpass.getpass('Password baru admin (minimal 12 karakter): ')
if len(password) < 12 or password == 'Admin123!':
    raise SystemExit('Gunakan password baru minimal 12 karakter.')
if password != getpass.getpass('Ulangi password: '):
    raise SystemExit('Password tidak cocok.')
# SQL literal escaping; no shell interpolation and no command-line password.
literal = "'" + password.replace("'", "''") + "'"
sql = ("BEGIN;\n"
       "CREATE TEMP TABLE changed_admin AS SELECT id FROM users WHERE email = " + email_literal + " AND role = 'admin';\n"
       "DO $$ BEGIN IF (SELECT count(*) FROM changed_admin) <> 1 THEN RAISE EXCEPTION 'Akun admin tidak ditemukan atau tidak unik'; END IF; END $$;\n"
       "UPDATE users SET password_hash = crypt(" + literal + ", gen_salt('bf', 10)) WHERE id IN (SELECT id FROM changed_admin) RETURNING email;\n"
       "COMMIT;\n")
subprocess.run(['docker', 'compose', '--env-file', '.env', '-f', 'docker-compose.dev.yml', 'exec', '-T', 'db', 'sh', '-c', 'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'], input=sql, text=True, cwd=Path(__file__).resolve().parents[1], check=True)
