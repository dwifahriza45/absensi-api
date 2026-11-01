import io
import hashlib
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

import backup_incremental as inc


class IncrementalTest(unittest.TestCase):
    def test_s3_export_uses_private_envfile_not_cli_or_all_app_secrets(self):
        with tempfile.TemporaryDirectory() as directory:
            work = Path(directory)
            info = {"Image": "test-image", "Config": {"Env": ["STORAGE_DRIVER=s3", "APP_ENV=production",
                    "S3_ACCESS_KEY_ID=fake-access", "S3_SECRET_ACCESS_KEY=fake-secret", "JWT_SECRET=not-for-helper",
                    "DATABASE_URL=not-for-helper", "PHONE_HASH_SECRET=not-for-helper"]},
                    "NetworkSettings": {"Networks": {"test-network": {}}}}
            args = inc.export_command(info, "test-container", work)
            self.assertNotIn("fake-secret", " ".join(args))
            self.assertNotIn("not-for-helper", " ".join(args))
            self.assertNotIn("--volumes-from", args)
            self.assertEqual(args[-3:], ["/usr/local/bin/photoctl", "test-image", "export"])
            self.assertIn("test-network", args)
            env = (work / "storage.env").read_text()
            self.assertIn("S3_SECRET_ACCESS_KEY=fake-secret", env)
            self.assertNotIn("JWT_SECRET", env)
            self.assertNotIn("DATABASE_URL", env)
            self.assertNotIn("PHONE_HASH_SECRET", env)
            self.assertEqual((work / "storage.env").stat().st_mode & 0o777, 0o600)

    def test_monthly_export_only_reads_requested_month(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repo = root / "repo"
            name = "20261101T020000000001Z"
            folder = repo / "snapshots" / name
            folder.mkdir(parents=True)
            (folder / "database.dump").write_bytes(b"test database dump")
            photos, links = [], []
            for month in ["01", "02"]:
                data = ("photo-" + month).encode()
                digest = hashlib.sha256(data).hexdigest()
                obj = inc.object_path(repo, digest)
                obj.parent.mkdir(parents=True, exist_ok=True)
                obj.write_bytes(data)
                row = {"employee_id": "EMP-20260101-000001", "employee_name": "=Unsafe Name",
                       "attendance_id": "ATT-2026" + month + "05-000001", "work_date": "2026-" + month + "-05",
                       "label": "masuk", "photo_id": "PHO-2026" + month + "05-000001",
                       "storage_key": "selfie_in/2026/" + month + "/05/example.jpg",
                       "content_type": "image/jpeg", "size_bytes": len(data)}
                photos.append({**row, "sha256": digest})
                links.append(row)
            manifest = {"version": 2, "snapshot_id": name,
                        "database_sha256": inc.backup.sha_file(folder / "database.dump"),
                        "photos": photos, "links": links}
            (folder / "snapshot.json").write_text(json.dumps(manifest))
            # February corruption should fail full verify, but not block January's independent export.
            inc.object_path(repo, photos[1]["sha256"]).write_bytes(b"corrupt")
            with self.assertRaises(ValueError):
                inc.verify(repo)
            output = root / "january"
            inc.export_month(repo, "latest", "2026-01", output)
            exported = json.loads((output / "manifest.json").read_text())
            self.assertEqual(len(exported["photos"]), 1)
            self.assertEqual(exported["photos"][0]["work_date"], "2026-01-05")
            self.assertIn("'=Unsafe Name", (output / "photos.csv").read_text(encoding="utf-8-sig"))
            inc.backup.verify_archive(output / "photos.tar.gz", exported["photos"])
            with self.assertRaises(ValueError):
                inc.export_month(repo, "latest", "2026-01", output)
            inc.export_month(repo, "latest", "2026-03", root / "empty-month")
            empty = json.loads((root / "empty-month" / "manifest.json").read_text())
            self.assertEqual(empty["photos"], [])

    def test_object_deduplication_and_corruption(self):
        with tempfile.TemporaryDirectory() as directory:
            repo = Path(directory)
            key = "selfie_in/2026/11/01/example.jpg"
            row = {"storage_key": key, "size_bytes": 5, "photo_id": "PHO-20261101-000001"}

            def stream():
                output = io.BytesIO()
                with tarfile.open(fileobj=output, mode="w") as archive:
                    item = tarfile.TarInfo(key)
                    item.size = 5
                    archive.addfile(item, io.BytesIO(b"photo"))
                output.seek(0)
                return output

            first = inc.store_new_photos(stream(), repo, {key: row})
            second = inc.store_new_photos(stream(), repo, {key: row})
            self.assertEqual(first, second)
            self.assertEqual(len(list((repo / "objects").glob("*/*"))), 1)
            self.assertEqual(inc.read_object(repo, first[key]), b"photo")
            path = inc.object_path(repo, first[key]["sha256"])
            path.write_bytes(b"wrong")
            with self.assertRaises(ValueError):
                inc.read_object(repo, first[key])

    def test_unsafe_keys_and_hashes(self):
        for key in ["../secret", "--checkpoint=exec", "x\n-C /etc", "foo/-C/etc", "a b.jpg"]:
            with self.assertRaises(ValueError):
                inc.check_key(key)
        with self.assertRaises(ValueError):
            inc.object_path(Path("/tmp/test"), "../secret")

    def test_rsync_pull_does_not_delete_and_uses_strict_ssh(self):
        with tempfile.TemporaryDirectory() as directory:
            config = {"ssh_host": "backup@example.test", "ssh_port": 2222,
                      "remote_repository": "/srv/backups/absensi", "local_repository": directory}
            with patch.object(inc.subprocess, "run") as run, patch.object(inc, "verify") as verify:
                verify.return_value = (Path(directory) / "snapshots" / "example", {"snapshot_id": "example", "photos": []})
                inc.pull(config)
                args = run.call_args.args[0]
                self.assertEqual(args[0], "rsync")
                self.assertFalse(any(a.startswith("--delete") for a in args))
                self.assertIn("StrictHostKeyChecking=yes", args[args.index("-e") + 1])
                self.assertIn("BatchMode=yes", args[args.index("-e") + 1])
                self.assertTrue((Path(directory) / "last-verified.json").exists())
            with patch.object(inc.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "rsync")):
                before = (Path(directory) / "last-verified.json").read_bytes()
                with self.assertRaises(subprocess.CalledProcessError):
                    inc.pull(config)
                self.assertEqual((Path(directory) / "last-verified.json").read_bytes(), before)
                with inc.exclusive_lock(Path(directory) / ".pull.lock"):
                    pass

    def test_ssh_shell_injection_rejected(self):
        config = {"ssh_host": "user@host; touch /tmp/bad", "remote_repository": "/backup", "local_repository": "/unused"}
        with self.assertRaises(ValueError):
            inc.pull(config)
        config["ssh_host"] = "user@host"
        config["remote_repository"] = "/backup;rm -rf x"
        with self.assertRaises(ValueError):
            inc.pull(config)

    def test_lock_prevents_overlap(self):
        with tempfile.TemporaryDirectory() as directory:
            lock = Path(directory) / ".lock"
            with inc.exclusive_lock(lock):
                with self.assertRaises(FileExistsError):
                    with inc.exclusive_lock(lock):
                        self.fail("lock overlap")
            with inc.exclusive_lock(lock):
                pass


if __name__ == "__main__":
    unittest.main()
