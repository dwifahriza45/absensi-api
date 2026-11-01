#!/usr/bin/env python3
"""Backup Docker Compose database + storage; export readable monthly photos.

Python standard library only. Does not delete source data or copy .env secrets.
"""
import argparse
import csv
import fcntl
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
import tarfile
import tempfile
from datetime import datetime, timezone

PROJECT = Path(__file__).resolve().parents[1]
STORAGE = "/app/storage"


def acquire_lock(path):
    """Kernel lock is automatically released on exit/crash (Linux/macOS)."""
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        try:
            fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as error:
            raise FileExistsError("Another backup/pull is already running") from error
        os.ftruncate(fd, 0)
        os.write(fd, str(os.getpid()).encode())
        return fd
    except BaseException:
        os.close(fd)
        raise


def release_lock(fd):
    # Keep the lock file: unlinking it races with other processes holding its inode.
    fcntl.flock(fd, fcntl.LOCK_UN)
    os.close(fd)


def safe_path(value):
    value = value.removeprefix("./")
    path = PurePosixPath(value)
    if (not value or path.is_absolute() or any(p in ("", ".", "..") for p in value.split("/"))
            or "\\" in value or ":" in value or "\x00" in value):
        raise ValueError("Invalid archive/storage path")
    return value


def slug(value):
    return re.sub(r"[^\w-]+", "-", value, flags=re.UNICODE).strip("-_")[:90] or "karyawan"


def month_bounds(value):
    if not re.fullmatch(r"\d{4}-\d{2}", value):
        raise ValueError("Bulan harus berformat YYYY-MM")
    start = datetime.strptime(value, "%Y-%m")
    end = datetime(start.year + (start.month == 12), start.month % 12 + 1, 1)
    return start.strftime("%Y-%m-%d"), end.strftime("%Y-%m-%d")


def sha_file(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for data in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(data)
    return digest.hexdigest()


class Compose:
    def __init__(self, filename):
        self.base = ["docker", "compose", "-f", str(Path(filename).resolve())]

    def run(self, *args, **kwargs):
        return subprocess.run(self.base + list(args), cwd=PROJECT, check=True, **kwargs)

    def query(self, sql):
        command = 'exec psql -X -v ON_ERROR_STOP=1 -qAt -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
        result = self.run("exec", "-T", "db", "sh", "-c", command,
                          input=sql, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        return [json.loads(line) for line in result.stdout.splitlines() if line.strip()]


def photo_rows(compose, month):
    start, end = month_bounds(month)  # Strict parse before interpolation; no arbitrary SQL input.
    sql = f"""
    SELECT row_to_json(r) FROM (
      SELECT e.employee_id, e.name AS employee_name, a.attendance_id,
             a.work_date::text AS work_date, link.label, p.photo_id,
             p.storage_key, p.content_type, p.size_bytes
      FROM attendances a JOIN employees e USING (employee_id)
      JOIN LATERAL (
        SELECT 'masuk' AS label, a.check_in_selfie_id AS photo_id
        UNION ALL SELECT 'pulang', a.check_out_selfie_id
        UNION ALL SELECT 'laporan-' || w.session_no, w.photo_id
                  FROM work_reports w WHERE w.attendance_id = a.attendance_id
      ) link ON link.photo_id IS NOT NULL
      JOIN photos p ON p.photo_id = link.photo_id
      WHERE a.work_date >= DATE '{start}' AND a.work_date < DATE '{end}'
      ORDER BY e.employee_id, a.work_date, link.label
    ) r;
    """
    return compose.query(sql)


def monthly_path(month, row):
    extension = {"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}.get(row["content_type"])
    if not extension or not re.fullmatch(r"masuk|pulang|laporan-[1-3]", row["label"]):
        raise ValueError("Unsupported photo type/label")
    employee = slug(row["employee_id"]) + "_" + slug(row["employee_name"])
    return safe_path(f"{month}/{employee}/{row['work_date']}/{row['label']}.{extension}")


def tar_info(name, size):
    item = tarfile.TarInfo(safe_path(name))
    item.size, item.mode, item.mtime = size, 0o600, 0
    return item


def export_storage(stream, original_path, monthly_archive, month, rows, expected):
    by_key = {}
    for row in rows:
        key = safe_path(row["storage_key"])
        by_key.setdefault(key, []).append(row)
    found, monthly_found, originals, monthly = set(), set(), [], []
    with tarfile.open(fileobj=stream, mode="r|*") as source, \
            tarfile.open(original_path, "w:gz") as original, \
            tarfile.open(monthly_archive, "w:gz") as readable:
        for item in source:
            if item.isdir():
                continue
            key = safe_path(item.name)
            if not item.isfile() or key in found:
                raise ValueError("Storage contains a link, special file, or duplicate path")
            if item.size > 5 * 1024 * 1024:
                raise ValueError("Storage contains a file larger than the photo limit")
            found.add(key)
            handle = source.extractfile(item)
            if handle is None:
                raise ValueError("Cannot read photo")
            data = handle.read()
            if len(data) != item.size:
                raise ValueError("Photo stream truncated")
            if isinstance(expected, dict) and key in expected and expected[key] != len(data):
                raise ValueError("Photo size differs from database metadata")
            digest = hashlib.sha256(data).hexdigest()
            original.addfile(tar_info(key, len(data)), io.BytesIO(data))
            originals.append({"path": key, "size": len(data), "sha256": digest})
            for row in by_key.get(key, []):
                if row["size_bytes"] != len(data):
                    raise ValueError("Photo size differs from database metadata")
                name = monthly_path(month, row)
                if name in monthly_found:
                    raise ValueError("Duplicate readable photo path")
                monthly_found.add(name)
                readable.addfile(tar_info(name, len(data)), io.BytesIO(data))
                monthly.append({**row, "path": name, "size": len(data), "sha256": digest})
    if not set(expected).issubset(found) or len(monthly) != len(rows):
        raise ValueError("Backup incomplete: photo referenced by database is missing")
    return originals, monthly


def verify_archive(path, expected):
    wanted = {safe_path(row["path"]): row for row in expected}
    if len(wanted) != len(expected):
        raise ValueError("Duplicate manifest entries")
    seen = set()
    with tarfile.open(path, "r|gz") as archive:
        for item in archive:
            key = safe_path(item.name)
            if not item.isfile() or key not in wanted or key in seen:
                raise ValueError("Archive contains unexpected file/link")
            row = wanted[key]
            if item.size != row["size"]:
                raise ValueError("Photo size verification failed")
            digest = hashlib.sha256()
            handle = archive.extractfile(item)
            for chunk in iter(lambda: handle.read(1024 * 1024), b""):
                digest.update(chunk)
            if digest.hexdigest() != row["sha256"]:
                raise ValueError("Photo checksum verification failed")
            seen.add(key)
    if seen != set(wanted):
        raise ValueError("Archive missing files")


def verify(directory):
    directory = Path(directory)
    with open(directory / "manifest.json", encoding="utf-8") as handle:
        manifest = json.load(handle)
    if manifest["version"] != 1:
        raise ValueError("Unsupported backup version")
    month_bounds(manifest["month"])
    for name, digest in manifest["checksums"].items():
        if name not in {"database.dump", "storage.tar.gz", "photos.tar.gz", "photos.csv"}:
            raise ValueError("Invalid backup component")
        if sha_file(directory / name) != digest:
            raise ValueError(f"Checksum mismatch: {name}")
    if set(manifest["checksums"]) != {"database.dump", "storage.tar.gz", "photos.tar.gz", "photos.csv"}:
        raise ValueError("Backup component missing")
    verify_archive(directory / "storage.tar.gz", manifest["storage"])
    verify_archive(directory / "photos.tar.gz", manifest["photos"])
    return manifest


def backup(args):
    month_bounds(args.month)
    target = Path(args.output).expanduser().resolve()
    for storage_path in [Path(STORAGE), PROJECT / "storage"]:
        if target == storage_path or storage_path in target.parents:
            raise ValueError("Output backup tidak boleh berada dalam storage aplikasi")
    if target.exists():
        raise ValueError("Output sudah ada; gunakan folder backup baru")
    target.parent.mkdir(parents=True, exist_ok=True)
    # Shared kernel lock prevents full and incremental backups from overlapping.
    lock = PROJECT / ".backup.lock"
    fd = acquire_lock(lock)
    compose = Compose(args.compose_file)
    running, stopped = False, False
    try:
        container = compose.run("ps", "-a", "-q", "app", stdout=subprocess.PIPE, text=True).stdout.strip()
        if not container or len(container.splitlines()) != 1:
            raise ValueError("Exactly one existing app container is required")
        inspected = subprocess.run(["docker", "inspect", container], check=True,
                                   stdout=subprocess.PIPE, text=True)
        info = json.loads(inspected.stdout)[0]
        environment = dict(v.split("=", 1) for v in info.get("Config", {}).get("Env", []) if "=" in v)
        if environment.get("STORAGE_DRIVER", "local") != "local":
            raise ValueError("S3/R2 storage requires backup_incremental.py nightly, then materialize for a full restore bundle")
        running = info["State"]["Running"]
        with tempfile.TemporaryDirectory(prefix=".backup-partial-", dir=target.parent) as tmp:
            work = Path(tmp)
            if running:
                print("Stopping API briefly for a consistent database/storage snapshot…", flush=True)
                compose.run("stop", "-t", "60", "app", stdout=subprocess.DEVNULL)
                stopped = True
            rows = photo_rows(compose, args.month)
            expected = {safe_path(row["storage_key"]): row["size_bytes"] for row in compose.query(
                "SELECT row_to_json(r) FROM (SELECT storage_key, size_bytes FROM photos) r;")}
            dump_cmd = 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc --no-owner --no-acl'
            with open(work / "database.dump", "xb") as handle:
                compose.run("exec", "-T", "db", "sh", "-c", dump_cmd, stdout=handle)
            proc = subprocess.Popen(["docker", "cp", f"{container}:{STORAGE}/.", "-"], stdout=subprocess.PIPE)
            try:
                originals, photos = export_storage(proc.stdout, work / "storage.tar.gz",
                                                  work / "photos.tar.gz", args.month, rows, expected)
                if proc.wait() != 0:
                    raise ValueError("docker cp storage failed")
            finally:
                proc.stdout.close()
                if proc.poll() is None:
                    proc.terminate()
                proc.wait()
            # Copy is now complete. Resume API before hashing/verifying the local archives.
            if stopped:
                compose.run("start", "app", stdout=subprocess.DEVNULL)
                stopped = False
            with open(work / "photos.csv", "w", newline="", encoding="utf-8-sig") as handle:
                columns = ["employee_id", "employee_name", "attendance_id", "work_date", "label",
                           "photo_id", "storage_key", "path", "content_type", "size", "sha256"]
                writer = csv.DictWriter(handle, fieldnames=columns, extrasaction="ignore")
                writer.writeheader()
                for row in photos:
                    # Excel formula injection protection for human-entered names.
                    writer.writerow({k: ("'" + v if isinstance(v, str) and v.lstrip().startswith(("=", "+", "-", "@")) else v)
                                     for k, v in row.items()})
            manifest = {"version": 1, "created_at": datetime.now(timezone.utc).isoformat(),
                        "month": args.month, "storage": originals, "photos": photos,
                        "checksums": {name: sha_file(work / name) for name in
                                      ["database.dump", "storage.tar.gz", "photos.tar.gz", "photos.csv"]}}
            (work / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2), encoding="utf-8")
            verify(work)
            for item in work.iterdir():
                item.chmod(0o600)
            if target.exists():
                raise ValueError("Output sudah ada; backup tidak akan menimpa folder lain")
            work.rename(target)
        print(f"Backup verified: {target}")
        print(f"Storage files: {len(originals)}; readable monthly photos: {len(photos)}")
        print("Copy the folder to a separate device, then run verify there. No source data was deleted.")
    finally:
        try:
            if stopped and running:
                compose.run("start", "app", stdout=subprocess.DEVNULL)
        finally:
            release_lock(fd)


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    create = sub.add_parser("backup", help="Pause API, copy DB + storage, resume API, verify")
    create.add_argument("--month", required=True, help="YYYY-MM for readable monthly export")
    create.add_argument("--output", required=True, help="New private directory, preferably on external disk")
    create.add_argument("--compose-file", default=str(PROJECT / "docker-compose.dev.yml"))
    check = sub.add_parser("verify", help="Offline integrity check; does not require Docker")
    check.add_argument("directory")
    args = parser.parse_args()
    try:
        if args.command == "backup":
            backup(args)
        else:
            manifest = verify(args.directory)
            print(f"Backup verified: {len(manifest['storage'])} storage files, {len(manifest['photos'])} monthly photos")
    except (ValueError, OSError, KeyError, tarfile.TarError, subprocess.SubprocessError) as error:
        # Never print subprocess environment, output, or credentials.
        detail = str(error) if isinstance(error, ValueError) else type(error).__name__
        print(f"Backup failed: {detail}. Check paths, Docker services, and backup integrity.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
