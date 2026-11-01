from pathlib import Path
import tempfile
import unittest

import setup_minio


def values(path):
    return dict(line.split("=", 1) for line in path.read_text().splitlines() if "=" in line and not line.startswith("#"))


class SetupMinioTest(unittest.TestCase):
    def test_preserves_application_secrets_and_is_idempotent(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / ".env"
            path.write_text("APP_ENV=development\nJWT_SECRET=keep-jwt\nPHONE_HASH_SECRET=keep-phone\nDB_PASSWORD=keep-db\nSTORAGE_DRIVER=local\n")
            setup_minio.configure(path)
            first = values(path)
            self.assertEqual(first["JWT_SECRET"], "keep-jwt")
            self.assertEqual(first["PHONE_HASH_SECRET"], "keep-phone")
            self.assertEqual(first["DB_PASSWORD"], "keep-db")
            self.assertEqual(first["S3_ACCESS_KEY_ID"], first["MINIO_APP_ACCESS_KEY"])
            self.assertEqual(first["S3_SECRET_ACCESS_KEY"], first["MINIO_APP_SECRET_KEY"])
            self.assertNotEqual(first["S3_SECRET_ACCESS_KEY"], first["MINIO_ROOT_PASSWORD"])
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            setup_minio.configure(path)
            self.assertEqual(first, values(path))

    def test_new_env_generates_secrets_and_production_is_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / ".env"
            setup_minio.configure(path)
            configured = values(path)
            self.assertGreaterEqual(len(configured["JWT_SECRET"]), 32)
            self.assertNotIn("ganti-", configured["PHONE_HASH_SECRET"])
            path.write_text("APP_ENV=production\n")
            with self.assertRaises(ValueError):
                setup_minio.configure(path)
            self.assertEqual(path.read_text(), "APP_ENV=production\n")


if __name__ == "__main__":
    unittest.main()
