"""Opt-in: production photoctl image + fake private S3 HTTP endpoint + isolated DB."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
import uuid

import backup
import backup_incremental as inc


@unittest.skipUnless(os.environ.get("BACKUP_S3_TEST_IMAGE"), "Set BACKUP_S3_TEST_IMAGE to a built API image")
class S3BackupTest(unittest.TestCase):
    def test_remote_photos_incremental_and_materialize(self):
        with tempfile.TemporaryDirectory(prefix="backup-s3-test-") as temp:
            root = Path(temp)
            bucket = root / "bucket"
            key = "selfie_in/2026/11/01/example.jpg"
            photo = bucket / "private-bucket" / "production" / key
            photo.parent.mkdir(parents=True)
            photo.write_bytes(b"test remote photo")
            for directory in [bucket, *photo.parents]:
                if directory == root:
                    break
                directory.chmod(0o755)
            photo.chmod(0o644)
            compose_file = root / "compose.yaml"
            image = os.environ["BACKUP_S3_TEST_IMAGE"]
            compose_file.write_text(f"""name: backup-s3-{uuid.uuid4().hex[:12]}
services:
  app:
    image: {json.dumps(image)}
    pull_policy: never
    entrypoint: [sh, -c, 'trap "exit 0" TERM; sleep 3600 & wait']
    stop_signal: SIGTERM
    environment:
      APP_ENV: development
      STORAGE_DRIVER: s3
      S3_ENDPOINT: http://s3:80
      S3_REGION: auto
      S3_BUCKET: private-bucket
      S3_PREFIX: production
      S3_ACCESS_KEY_ID: fake-id
      S3_SECRET_ACCESS_KEY: fake-secret
      S3_FORCE_PATH_STYLE: 'true'
    volumes:
      - local-storage:/app/storage
  s3:
    image: nginx:stable-alpine
    volumes:
      - {json.dumps(str(bucket) + ':/usr/share/nginx/html:ro')}
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: backup_test
      POSTGRES_DB: backup_test
      POSTGRES_PASSWORD: isolated-test-only
    tmpfs:
      - /var/lib/postgresql/data
volumes:
  local-storage:
""")
            compose = backup.Compose(compose_file)
            try:
                compose.run("up", "-d", stdout=subprocess.DEVNULL)
                for attempt in range(60):
                    try:
                        compose.query("SELECT row_to_json(r) FROM (SELECT 1 AS ready) r;")
                        break
                    except subprocess.CalledProcessError:
                        if attempt == 59: raise
                        time.sleep(1)
                compose.query("""
                CREATE TABLE employees(employee_id text PRIMARY KEY, name text);
                CREATE TABLE photos(photo_id text PRIMARY KEY, storage_key text, content_type text, size_bytes bigint);
                CREATE TABLE attendances(attendance_id text PRIMARY KEY, employee_id text, work_date date,
                                         check_in_selfie_id text, check_out_selfie_id text);
                CREATE TABLE work_reports(attendance_id text, session_no int, photo_id text);
                INSERT INTO employees VALUES ('EMP-20261101-000001', 'Hamdan Soleh');
                INSERT INTO photos VALUES ('PHO-20261101-000001', 'selfie_in/2026/11/01/example.jpg', 'image/jpeg', 17);
                INSERT INTO attendances VALUES ('ATT-20261101-000001', 'EMP-20261101-000001', '2026-11-01',
                                                'PHO-20261101-000001', NULL);
                """)
                repo = root / "repo"
                config = {"repository": str(repo), "compose_file": str(compose_file)}
                first = inc.nightly(config)
                self.assertEqual(first["storage_backend"], "s3")
                self.assertEqual(first["new_photos"], 1)
                self.assertEqual(inc.read_object(repo, first["photos"][0]), photo.read_bytes())
                second = inc.nightly(config)
                self.assertEqual((second["new_photos"], second["reused_photos"]), (0, 1))
                inc.verify(repo)
                self.assertFalse(any(p.name == "storage.env" for p in repo.rglob("*")))
                self.assertNotIn("fake-secret", (repo / "snapshots" / first["snapshot_id"] / "snapshot.json").read_text())
                bundle = root / "bundle"
                inc.materialize(repo, "latest", "2026-11", bundle)
                self.assertEqual(len(backup.verify(bundle)["photos"]), 1)
                with self.assertRaisesRegex(ValueError, "S3/R2"):
                    backup.backup(argparse.Namespace(month="2026-11", output=str(root / "wrong-full"), compose_file=str(compose_file)))
                self.assertTrue(compose.run("ps", "--status", "running", "-q", "app", stdout=subprocess.PIPE, text=True).stdout.strip())
            finally:
                compose.run("down", "--volumes", stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
