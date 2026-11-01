import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
import argparse
import os
import subprocess
import time
import uuid

import backup
import backup_incremental as incremental


class BackupTest(unittest.TestCase):
    def row(self):
        return {"employee_id": "EMP-20261101-000001", "employee_name": "Hamdan Soleh",
                "attendance_id": "ATT-20261101-000001", "work_date": "2026-11-01",
                "label": "masuk", "photo_id": "PHO-20261101-000001",
                "storage_key": "selfie_in/2026/11/01/fake.jpg",
                "content_type": "image/jpeg", "size_bytes": 5}

    def stream(self, name, data=b"photo"):
        result = io.BytesIO()
        with tarfile.open(fileobj=result, mode="w") as archive:
            info = tarfile.TarInfo(name)
            info.size = len(data)
            archive.addfile(info, io.BytesIO(data))
        result.seek(0)
        return result

    def test_roundtrip_and_corruption(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            row = self.row()
            original, monthly = backup.export_storage(
                self.stream("./" + row["storage_key"]), folder / "storage.tar.gz",
                folder / "photos.tar.gz", "2026-11", [row], {row["storage_key"]})
            self.assertEqual(monthly[0]["path"],
                             "2026-11/EMP-20261101-000001_Hamdan-Soleh/2026-11-01/masuk.jpg")
            (folder / "database.dump").write_bytes(b"fake dump for offline integrity test")
            (folder / "photos.csv").write_text("employee_id\n", encoding="utf-8")
            manifest = {"version": 1, "month": "2026-11", "storage": original, "photos": monthly,
                        "checksums": {name: backup.sha_file(folder / name) for name in
                                      ["storage.tar.gz", "photos.tar.gz", "database.dump", "photos.csv"]}}
            (folder / "manifest.json").write_text(json.dumps(manifest))
            backup.verify(folder)
            self.assertEqual(original[0]["sha256"], hashlib.sha256(b"photo").hexdigest())
            with tarfile.open(folder / "storage.tar.gz") as archive:
                self.assertEqual(archive.extractfile(row["storage_key"]).read(), b"photo")
            (folder / "database.dump").write_bytes(b"damaged")
            with self.assertRaisesRegex(ValueError, "Checksum mismatch"):
                backup.verify(folder)

    def test_missing_photo_fails(self):
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            row = self.row()
            with self.assertRaisesRegex(ValueError, "missing"):
                backup.export_storage(self.stream("unrelated.jpg"), folder / "storage.tar.gz",
                                      folder / "photos.tar.gz", "2026-11", [row], {row["storage_key"]})

    def test_paths_and_months(self):
        for value in ["../secret", "/etc/passwd", "foo/../bar", "foo\\bar", "a//b", "a/./b"]:
            with self.assertRaises(ValueError):
                backup.safe_path(value)
        for value in ["2026-13", "2026-1", "2026-01'; DROP TABLE photos; --"]:
            with self.assertRaises(ValueError):
                backup.month_bounds(value)
        self.assertEqual(backup.month_bounds("2026-12"), ("2026-12-01", "2027-01-01"))
        self.assertEqual(backup.month_bounds("2028-02"), ("2028-02-01", "2028-03-01"))

    def test_symlink_rejected(self):
        stream = io.BytesIO()
        with tarfile.open(fileobj=stream, mode="w") as archive:
            item = tarfile.TarInfo("link.jpg")
            item.type, item.linkname = tarfile.SYMTYPE, "/etc/passwd"
            archive.addfile(item)
        stream.seek(0)
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            with self.assertRaisesRegex(ValueError, "link"):
                backup.export_storage(stream, folder / "storage.tar.gz", folder / "photos.tar.gz",
                                      "2026-11", [], set())


@unittest.skipUnless(os.environ.get("BACKUP_DOCKER_TEST") == "1", "Set BACKUP_DOCKER_TEST=1 for isolated Docker integration")
class DockerBackupTest(unittest.TestCase):
    def test_backup_and_restore_in_isolated_environment(self):
        with tempfile.TemporaryDirectory(prefix="absensi-backup-test-") as directory:
            folder = Path(directory)
            storage = folder / "storage"
            storage.mkdir()
            key = "selfie_in/2026/11/01/fake.jpg"
            photo = storage / key
            photo.parent.mkdir(parents=True)
            photo.write_bytes(b"photo")
            compose_file = folder / "compose.yaml"
            compose_file.write_text(f"""name: backup-test-{uuid.uuid4().hex[:12]}
services:
  app:
    image: nginx:stable-alpine
    stop_signal: SIGTERM
    entrypoint: [sh, -c, 'trap "exit 0" TERM; sleep 3600 & wait']
    volumes:
      - {json.dumps(str(storage) + ':/app/storage')}
  db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: backup_test
      POSTGRES_DB: backup_test
      POSTGRES_PASSWORD: isolated-test-only
    tmpfs:
      - /var/lib/postgresql/data
""")
            compose = backup.Compose(compose_file)
            try:
                compose.run("up", "-d", stdout=subprocess.DEVNULL)
                for attempt in range(60):
                    try:
                        compose.query("SELECT row_to_json(r) FROM (SELECT 1 AS ready) r;")
                        break
                    except subprocess.CalledProcessError:
                        if attempt == 59:
                            raise
                        time.sleep(1)
                compose.query("""
                CREATE TABLE employees(employee_id text PRIMARY KEY, name text);
                CREATE TABLE photos(photo_id text PRIMARY KEY, storage_key text, content_type text, size_bytes bigint);
                CREATE TABLE attendances(attendance_id text PRIMARY KEY, employee_id text, work_date date,
                                         check_in_selfie_id text, check_out_selfie_id text);
                CREATE TABLE work_reports(attendance_id text, session_no int, photo_id text);
                INSERT INTO employees VALUES ('EMP-20261101-000001', 'Hamdan Soleh');
                INSERT INTO photos VALUES ('PHO-20261101-000001', 'selfie_in/2026/11/01/fake.jpg', 'image/jpeg', 5);
                INSERT INTO attendances VALUES ('ATT-20261101-000001', 'EMP-20261101-000001', '2026-11-01',
                                                'PHO-20261101-000001', NULL);
                """)
                output = folder / "snapshot"
                backup.backup(argparse.Namespace(month="2026-11", output=str(output), compose_file=str(compose_file)))
                manifest = backup.verify(output)
                self.assertEqual(len(manifest["photos"]), 1)
                self.assertEqual(len(manifest["storage"]), 1)
                # Backup must restore the app's prior running state.
                self.assertTrue(compose.run("ps", "--status", "running", "-q", "app",
                                            stdout=subprocess.PIPE, text=True).stdout.strip())
                compose.run("exec", "-T", "db", "createdb", "-U", "backup_test", "restored")
                with open(output / "database.dump", "rb") as dump:
                    compose.run("exec", "-T", "db", "pg_restore", "--exit-on-error", "--single-transaction",
                                "--no-owner", "--no-acl", "-U", "backup_test", "-d", "restored", stdin=dump)
                result = compose.run("exec", "-T", "db", "psql", "-X", "-At", "-U", "backup_test", "-d",
                                     "restored", "-c", "SELECT count(*) FROM attendances;",
                                     stdout=subprocess.PIPE, text=True)
                self.assertEqual(result.stdout.strip(), "1")
                # Restore originals to a separate empty destination; no user data is touched.
                restored = folder / "restored-storage"
                restored.mkdir()
                with tarfile.open(output / "storage.tar.gz") as archive:
                    for item in archive:
                        target = restored / backup.safe_path(item.name)
                        target.parent.mkdir(parents=True, exist_ok=True)
                        target.write_bytes(archive.extractfile(item).read())
                self.assertEqual((restored / key).read_bytes(), photo.read_bytes())
                # Nightly snapshots reuse old objects and copy only new DB photos.
                repo = folder / "incremental-repository"
                config = {"repository": str(repo), "compose_file": str(compose_file)}
                first = incremental.nightly(config)
                self.assertEqual((first["new_photos"], first["reused_photos"]), (1, 0))
                second = incremental.nightly(config)
                self.assertEqual((second["new_photos"], second["reused_photos"]), (0, 1))
                second_key = "selfie_out/2026/11/01/fake.jpg"
                second_photo = storage / second_key
                second_photo.parent.mkdir(parents=True)
                second_photo.write_bytes(b"second")
                compose.query("""
                INSERT INTO photos VALUES ('PHO-20261101-000002', 'selfie_out/2026/11/01/fake.jpg', 'image/jpeg', 6);
                UPDATE attendances SET check_out_selfie_id = 'PHO-20261101-000002';
                """)
                third = incremental.nightly(config)
                self.assertEqual((third["new_photos"], third["reused_photos"]), (1, 1))
                incremental.verify(repo)
                bundle = folder / "incremental-restore-bundle"
                incremental.materialize(repo, first["snapshot_id"], "2026-11", bundle)
                old_snapshot = backup.verify(bundle)
                self.assertEqual(len(old_snapshot["photos"]), 1)
                # Restoring an older DB snapshot does not pick up later photos/data.
                compose.run("exec", "-T", "db", "createdb", "-U", "backup_test", "incremental_restored")
                with open(bundle / "database.dump", "rb") as dump:
                    compose.run("exec", "-T", "db", "pg_restore", "--exit-on-error", "--single-transaction",
                                "--no-owner", "--no-acl", "-U", "backup_test", "-d", "incremental_restored", stdin=dump)
                result = compose.run("exec", "-T", "db", "psql", "-X", "-At", "-U", "backup_test", "-d",
                                     "incremental_restored", "-c", "SELECT count(*) FROM photos;",
                                     stdout=subprocess.PIPE, text=True)
                self.assertEqual(result.stdout.strip(), "1")
                obj = incremental.object_path(repo, third["photos"][0]["sha256"])
                data = obj.read_bytes()
                obj.write_bytes(b"bad")
                with self.assertRaises(ValueError):
                    incremental.verify(repo)
                obj.write_bytes(data)
                # Missing NEW photo must not publish a complete snapshot or leave API down.
                compose.query("INSERT INTO photos VALUES ('PHO-20261101-000003', 'work_report/2026/11/01/missing.jpg', 'image/jpeg', 5);")
                with self.assertRaises(ValueError):
                    incremental.nightly(config)
                self.assertEqual(incremental.snapshot(repo)[1]["snapshot_id"], third["snapshot_id"])
                self.assertTrue(compose.run("ps", "--status", "running", "-q", "app",
                                            stdout=subprocess.PIPE, text=True).stdout.strip())
                # A missing source photo aborts the backup but still resumes the app.
                photo.unlink()
                failed = folder / "incomplete"
                with self.assertRaises(ValueError):
                    backup.backup(argparse.Namespace(month="2026-11", output=str(failed), compose_file=str(compose_file)))
                self.assertFalse(failed.exists())
                self.assertTrue(compose.run("ps", "--status", "running", "-q", "app",
                                            stdout=subprocess.PIPE, text=True).stdout.strip())
            finally:
                compose.run("down", "--volumes", stdout=subprocess.DEVNULL)


if __name__ == "__main__":
    unittest.main()
