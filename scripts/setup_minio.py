#!/usr/bin/env python3
"""Configure local MinIO without printing credentials or changing existing application secrets."""
import argparse
import os
from pathlib import Path
import re
import secrets
import tempfile

PROJECT = Path(__file__).resolve().parents[1]


def configure(path):
    path = Path(path).expanduser().resolve()
    new = not path.exists()
    text = (PROJECT / ".env.example").read_text() if new else path.read_text()
    values = {}
    for line in text.splitlines():
        if "=" in line and not line.lstrip().startswith("#"):
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip()
    if values.get("APP_ENV", "development") != "development":
        raise ValueError("Setup MinIO HTTP ini hanya untuk APP_ENV=development")
    app_key = values.get("MINIO_APP_ACCESS_KEY") or secrets.token_hex(10)
    app_secret = values.get("MINIO_APP_SECRET_KEY") or secrets.token_urlsafe(36)
    bucket = values.get("MINIO_BUCKET") or "absensi-development"
    prefix = values.get("MINIO_PREFIX", "development").strip("/")
    if not re.fullmatch(r"[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]", bucket) or ".." in bucket:
        raise ValueError("MINIO_BUCKET tidak valid")
    if prefix and (not re.fullmatch(r"[A-Za-z0-9/_-]+", prefix) or ".." in prefix):
        raise ValueError("MINIO_PREFIX tidak valid")
    settings = {
        "STORAGE_DRIVER": "s3", "S3_ENDPOINT": "http://minio:9000", "S3_REGION": "us-east-1",
        "S3_BUCKET": bucket, "S3_PREFIX": prefix, "S3_ACCESS_KEY_ID": app_key,
        "S3_SECRET_ACCESS_KEY": app_secret, "S3_FORCE_PATH_STYLE": "true",
        "S3_ALLOW_MINIO_HTTP": "true",
        "MINIO_IMAGE": values.get("MINIO_IMAGE") or "absensi-minio:local",
        "MINIO_MC_IMAGE": values.get("MINIO_MC_IMAGE") or "absensi-minio-mc:local",
        "MINIO_API_PORT": values.get("MINIO_API_PORT") or "9000",
        "MINIO_CONSOLE_PORT": values.get("MINIO_CONSOLE_PORT") or "9001",
        "MINIO_REGION": "us-east-1", "MINIO_BUCKET": bucket, "MINIO_PREFIX": prefix,
        "MINIO_ROOT_USER": values.get("MINIO_ROOT_USER") or "absensi-root",
        "MINIO_ROOT_PASSWORD": values.get("MINIO_ROOT_PASSWORD") or secrets.token_urlsafe(36),
        "MINIO_APP_ACCESS_KEY": app_key, "MINIO_APP_SECRET_KEY": app_secret,
    }
    if new:
        settings.update(JWT_SECRET=secrets.token_urlsafe(48), PHONE_HASH_SECRET=secrets.token_urlsafe(48))
    if settings["MINIO_ROOT_USER"] == app_key or settings["MINIO_ROOT_PASSWORD"] == app_secret:
        raise ValueError("Credentials aplikasi harus berbeda dari credentials root MinIO")
    result, seen = [], set()
    for line in text.splitlines():
        key = line.split("=", 1)[0].strip() if "=" in line and not line.lstrip().startswith("#") else None
        if key in settings:
            if key not in seen:
                result.append(key + "=" + settings[key])
                seen.add(key)
        else:
            result.append(line)
    result.extend(k + "=" + v for k, v in settings.items() if k not in seen)
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".env.minio-", dir=path.parent)
    try:
        with os.fdopen(fd, "w") as handle:
            handle.write("\n".join(result) + "\n")
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary, 0o600)
        os.replace(temporary, path)
    finally:
        Path(temporary).unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env-file", default=str(PROJECT / ".env"))
    args = parser.parse_args()
    configure(args.env_file)
    print("MinIO development configured. Credentials saved privately in .env; existing DB/JWT/phone secrets preserved.")


if __name__ == "__main__":
    main()
