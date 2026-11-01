#!/usr/bin/env python3
"""Nightly full DB snapshots + immutable incremental photo objects + SSH pull.

No application data is deleted. No credentials are embedded in configuration.
"""
import argparse
from contextlib import contextmanager
import csv
from datetime import datetime, timezone
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import subprocess
import sys
import tarfile
import tempfile

import backup

SNAPSHOT_RE = re.compile(r"\d{8}T\d{12}Z")
HASH_RE = re.compile(r"[a-f0-9]{64}")


@contextmanager
def exclusive_lock(path):
    fd = backup.acquire_lock(path)
    try:
        yield
    finally:
        backup.release_lock(fd)


def read_json(path):
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def check_key(key):
    backup.safe_path(key)
    if not re.fullmatch(r"[A-Za-z0-9_./-]+", key) or any(p.startswith("-") for p in key.split("/")):
        raise ValueError("Unsupported storage key")
    return key


def object_path(repo, digest):
    if not isinstance(digest, str) or not HASH_RE.fullmatch(digest):
        raise ValueError("Invalid photo checksum")
    path = repo / "objects" / digest[:2] / digest
    if path.is_symlink() or repo.resolve() not in path.resolve().parents:
        raise ValueError("Object path escapes repository")
    return path


def read_object(repo, row):
    path = object_path(repo, row["sha256"])
    if not 0 < row["size_bytes"] <= 5 * 1024 * 1024:
        raise ValueError("Invalid photo size")
    with open(path, "rb") as handle:
        data = handle.read(5 * 1024 * 1024 + 1)
    if len(data) != row["size_bytes"] or hashlib.sha256(data).hexdigest() != row["sha256"]:
        raise ValueError("Photo integrity check failed")
    return data


def snapshot(repo, name="latest"):
    if name == "latest":
        names = sorted(p.name for p in (repo / "snapshots").iterdir()
                       if p.is_dir() and not p.is_symlink() and SNAPSHOT_RE.fullmatch(p.name))
        if not names:
            raise ValueError("No complete snapshot available")
        name = names[-1]
    if not SNAPSHOT_RE.fullmatch(name):
        raise ValueError("Invalid snapshot ID")
    directory = repo / "snapshots" / name
    if directory.is_symlink() or repo.resolve() not in directory.resolve().parents:
        raise ValueError("Snapshot path escapes repository")
    manifest = read_json(directory / "snapshot.json")
    if manifest["version"] != 2 or manifest["snapshot_id"] != name:
        raise ValueError("Invalid snapshot manifest")
    return directory, manifest


def verify(repo, name="latest"):
    directory, manifest = snapshot(repo, name)
    if backup.sha_file(directory / "database.dump") != manifest["database_sha256"]:
        raise ValueError("Database checksum verification failed")
    seen = set()
    for row in manifest["photos"]:
        key = check_key(row["storage_key"])
        if key in seen:
            raise ValueError("Duplicate snapshot storage key")
        seen.add(key)
        read_object(repo, row)
    for row in manifest["links"]:
        if row["storage_key"] not in seen:
            raise ValueError("Monthly photo reference missing")
    return directory, manifest


def store_new_photos(stream, repo, expected):
    found = {}
    with tarfile.open(fileobj=stream, mode="r|*") as source:
        for item in source:
            if item.isdir():
                continue
            key = check_key(item.name.removeprefix("./"))
            if not item.isfile() or key not in expected or key in found:
                raise ValueError("Unexpected file/link in photo stream")
            if item.size != expected[key]["size_bytes"] or not 0 < item.size <= 5 * 1024 * 1024:
                raise ValueError("Photo size differs from database")
            data = source.extractfile(item).read(5 * 1024 * 1024 + 1)
            if len(data) != item.size:
                raise ValueError("Truncated photo stream")
            digest = hashlib.sha256(data).hexdigest()
            target = object_path(repo, digest)
            target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            if target.exists():
                read_object(repo, {"sha256": digest, "size_bytes": len(data)})
            else:
                fd, tmp = tempfile.mkstemp(prefix=".object-", dir=target.parent)
                try:
                    with os.fdopen(fd, "wb") as handle:
                        handle.write(data)
                        handle.flush()
                        os.fsync(handle.fileno())
                    os.replace(tmp, target)
                finally:
                    Path(tmp).unlink(missing_ok=True)
            found[key] = {**expected[key], "sha256": digest}
    if set(found) != set(expected):
        raise ValueError("New photo referenced by database is missing")
    return found


def all_links(compose):
    return compose.query("""
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
      ORDER BY a.work_date, e.employee_id, link.label
    ) r;
    """)


def export_command(info, container, work):
    env = dict(value.split("=", 1) for value in info.get("Config", {}).get("Env", []) if "=" in value)
    driver = env.get("STORAGE_DRIVER", "local")
    command = ["docker", "run", "--rm", "-i", "--read-only", "--cap-drop", "ALL",
               "--security-opt", "no-new-privileges:true"]
    if driver == "local":
        return command + ["--volumes-from", container + ":ro", "--network", "none", "--entrypoint", "tar", info["Image"],
                          "-C", backup.STORAGE, "-cf", "-", "-T", "-"]
    if driver != "s3":
        raise ValueError("Unsupported storage driver for backup")
    # Private temporary env file, never CLI credentials or snapshot data.
    selected = {k: v for k, v in env.items() if k.startswith("S3_") or k in {"STORAGE_DRIVER", "APP_ENV"}}
    if any("\n" in v or "\r" in v for v in selected.values()):
        raise ValueError("Invalid storage environment")
    envfile = work / "storage.env"
    envfile.write_text("".join(k + "=" + v + "\n" for k, v in selected.items()), encoding="utf-8")
    envfile.chmod(0o600)
    networks = info.get("NetworkSettings", {}).get("Networks", {})
    network = next(iter(networks), "bridge")
    return command + ["--network", network, "--env-file", str(envfile),
                      "--entrypoint", "/usr/local/bin/photoctl", info["Image"], "export"]


def nightly(config):
    repo = Path(config["repository"]).expanduser().resolve()
    for source in [Path(backup.STORAGE), backup.PROJECT / "storage"]:
        if repo == source or source in repo.parents:
            raise ValueError("Repository must be outside application storage")
    repo.mkdir(mode=0o700, parents=True, exist_ok=True)
    repo.chmod(0o700)
    (repo / "snapshots").mkdir(mode=0o700, exist_ok=True)
    compose = backup.Compose(config["compose_file"])
    with exclusive_lock(backup.PROJECT / ".backup.lock"), exclusive_lock(repo / ".nightly.lock"):
        old = {}
        if any(SNAPSHOT_RE.fullmatch(p.name) for p in (repo / "snapshots").iterdir()):
            _, previous = snapshot(repo)
            old = {p["storage_key"]: p for p in previous["photos"]}
        container = compose.run("ps", "-a", "-q", "app", text=True, stdout=subprocess.PIPE).stdout.strip()
        if not container or len(container.splitlines()) != 1:
            raise ValueError("Exactly one app container is required")
        inspect = subprocess.run(["docker", "inspect", container], check=True, text=True,
                                 stdout=subprocess.PIPE)
        info = json.loads(inspect.stdout)[0]
        runtime_env = dict(v.split("=", 1) for v in info.get("Config", {}).get("Env", []) if "=" in v)
        driver = runtime_env.get("STORAGE_DRIVER", "local")
        if driver not in {"local", "s3"}:
            raise ValueError("Unsupported storage driver for backup")
        running, stopped = info["State"]["Running"], False
        snapshot_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
        final = repo / "snapshots" / snapshot_id
        try:
            with tempfile.TemporaryDirectory(prefix=".snapshot-", dir=repo) as temp:
                work = Path(temp)
                if running:
                    print("Pausing API for consistent incremental backup…", flush=True)
                    stopped = True  # Also attempt restart if stop is interrupted.
                    compose.run("stop", "-t", "60", "app", stdout=subprocess.DEVNULL)
                photos = compose.query("SELECT row_to_json(r) FROM (SELECT photo_id, storage_key, content_type, size_bytes FROM photos ORDER BY storage_key) r;")
                links = all_links(compose)
                reused, pending = {}, {}
                for row in photos:
                    key = check_key(row["storage_key"])
                    previous = old.get(key)
                    if previous and all(previous[k] == row[k] for k in ["photo_id", "size_bytes", "content_type"]):
                        obj = object_path(repo, previous["sha256"])
                        if obj.is_file() and obj.stat().st_size == row["size_bytes"]:
                            reused[key] = {**row, "sha256": previous["sha256"]}
                            continue
                    pending[key] = row
                with open(work / "database.dump", "xb") as handle:
                    compose.run("exec", "-T", "db", "sh", "-c",
                                'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc --no-owner --no-acl', stdout=handle)
                # R2 objects are immutable conditional creates. Once DB metadata/dump
                # is frozen, resume API before downloading photos from the network.
                # No external bucket deletion/lifecycle may run during this copy.
                if driver == "s3" and stopped:
                    compose.run("start", "app", stdout=subprocess.DEVNULL)
                    stopped = False
                added = {}
                if pending:
                    # Feed filenames via a file: no shell interpolation, command-length limit,
                    # or deadlock between a full stdin pipe and streaming tar stdout.
                    keys = work / "photo-keys.txt"
                    keys.write_text("".join(k + "\n" for k in pending), encoding="utf-8")
                    command = export_command(info, container, work)
                    with open(keys, "rb") as paths:
                        proc = subprocess.Popen(command, stdin=paths, stdout=subprocess.PIPE)
                        try:
                            added = store_new_photos(proc.stdout, repo, pending)
                            if proc.wait() != 0:
                                raise ValueError("Incremental photo copy failed")
                        finally:
                            proc.stdout.close()
                            if proc.poll() is None:
                                proc.terminate()
                            proc.wait()
                    keys.unlink()
                    (work / "storage.env").unlink(missing_ok=True)
                if stopped:
                    compose.run("start", "app", stdout=subprocess.DEVNULL)
                    stopped = False
                catalog = {**reused, **added}
                if len(catalog) != len(photos) or any(r["storage_key"] not in catalog for r in links):
                    raise ValueError("Incomplete snapshot photo catalog")
                manifest = {"version": 2, "snapshot_id": snapshot_id,
                            "storage_backend": driver,
                            "created_at": datetime.now(timezone.utc).isoformat(),
                            "database_sha256": backup.sha_file(work / "database.dump"),
                            "photos": list(catalog.values()), "links": links,
                            "new_photos": len(added), "reused_photos": len(reused)}
                (work / "snapshot.json").write_text(json.dumps(manifest, ensure_ascii=False), encoding="utf-8")
                work.rename(final)
                print(f"Snapshot {snapshot_id}: {len(added)} new photos, {len(reused)} reused; full database saved.", flush=True)
                return manifest
        finally:
            if stopped and running:
                compose.run("start", "app", stdout=subprocess.DEVNULL)


def pull(config):
    host = config["ssh_host"]
    remote = config["remote_repository"]
    port = int(config.get("ssh_port", 22))
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]*@[A-Za-z0-9][A-Za-z0-9.-]*", host):
        raise ValueError("SSH host must be user@hostname or user@IPv4")
    if not re.fullmatch(r"/[A-Za-z0-9_./-]+", remote) or ".." in remote.split("/") or not 1 <= port <= 65535:
        raise ValueError("Remote repository must be an absolute path without spaces")
    repo = Path(config["local_repository"]).expanduser().resolve()
    repo.mkdir(mode=0o700, parents=True, exist_ok=True)
    repo.chmod(0o700)
    ssh = ["ssh", "-p", str(port), "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
           "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3"]
    if config.get("identity_file"):
        ssh += ["-i", str(Path(config["identity_file"]).expanduser())]
    with exclusive_lock(repo / ".pull.lock"):
        subprocess.run(["rsync", "-rtp", "--partial", "--delay-updates", "--timeout=120",
                        "--exclude=.snapshot-*", "--exclude=.object-*", "--exclude=*.lock",
                        "-e", shlex.join(ssh), host + ":" + remote.rstrip("/") + "/", str(repo) + "/"], check=True)
        directory, manifest = verify(repo)
        (repo / "last-verified.json").write_text(json.dumps({"snapshot_id": manifest["snapshot_id"],
              "verified_at": datetime.now(timezone.utc).isoformat()}), encoding="utf-8")
        print(f"Local backup verified: {directory.name}, {len(manifest['photos'])} photos. No remote/local files deleted.")


def materialize(repo, name, month, target):
    """Create the v1 complete restore bundle on demand, not every night."""
    backup.month_bounds(month)
    directory, manifest = verify(repo, name)
    target = target.expanduser().resolve()
    if target.exists():
        raise ValueError("Output already exists")
    target.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".materialize-", dir=target.parent) as tmp:
        work = Path(tmp)
        shutil.copyfile(directory / "database.dump", work / "database.dump")
        originals, monthly, used_names = [], [], set()
        catalog = {r["storage_key"]: r for r in manifest["photos"]}
        with tarfile.open(work / "storage.tar.gz", "w:gz") as archive:
            for row in manifest["photos"]:
                key = check_key(row["storage_key"])
                data = read_object(repo, row)
                archive.addfile(backup.tar_info(key, len(data)), io.BytesIO(data))
                originals.append({"path": key, "size": len(data), "sha256": row["sha256"]})
        with tarfile.open(work / "photos.tar.gz", "w:gz") as archive:
            for row in manifest["links"]:
                if not row["work_date"].startswith(month + "-"):
                    continue
                obj = catalog[row["storage_key"]]
                path = backup.monthly_path(month, row)
                if path in used_names:
                    raise ValueError("Duplicate monthly photo path")
                used_names.add(path)
                data = read_object(repo, obj)
                archive.addfile(backup.tar_info(path, len(data)), io.BytesIO(data))
                monthly.append({**row, "path": path, "size": len(data), "sha256": obj["sha256"]})
        with open(work / "photos.csv", "w", encoding="utf-8-sig", newline="") as handle:
            columns = ["employee_id", "employee_name", "attendance_id", "work_date", "label", "photo_id",
                       "storage_key", "path", "content_type", "size", "sha256"]
            writer = csv.DictWriter(handle, fieldnames=columns, extrasaction="ignore")
            writer.writeheader()
            for row in monthly:
                writer.writerow({k: ("'" + v if isinstance(v, str) and v.lstrip().startswith(("=", "+", "-", "@")) else v)
                                 for k, v in row.items()})
        result = {"version": 1, "month": month, "snapshot_id": directory.name,
                  "created_at": datetime.now(timezone.utc).isoformat(), "storage": originals, "photos": monthly,
                  "checksums": {n: backup.sha_file(work / n) for n in
                                ["database.dump", "storage.tar.gz", "photos.tar.gz", "photos.csv"]}}
        (work / "manifest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        backup.verify(work)
        if target.exists():
            raise ValueError("Output already exists")
        work.rename(target)
    print(f"Restore bundle and readable monthly archive verified: {target}")


def export_month(repo, name, month, target):
    """Only the requested month; no full-year photo archive is built."""
    backup.month_bounds(month)
    directory, manifest = snapshot(repo, name)
    if backup.sha_file(directory / "database.dump") != manifest["database_sha256"]:
        raise ValueError("Database checksum verification failed")
    target = target.expanduser().resolve()
    if target.exists():
        raise ValueError("Output already exists")
    target.parent.mkdir(parents=True, exist_ok=True)
    catalog = {p["storage_key"]: p for p in manifest["photos"]}
    rows, names = [], set()
    with tempfile.TemporaryDirectory(prefix=".monthly-", dir=target.parent) as tmp:
        work = Path(tmp)
        with tarfile.open(work / "photos.tar.gz", "w:gz") as archive:
            for row in manifest["links"]:
                if not row["work_date"].startswith(month + "-"):
                    continue
                obj = catalog[check_key(row["storage_key"])]
                path = backup.monthly_path(month, row)
                if path in names:
                    raise ValueError("Duplicate monthly photo path")
                names.add(path)
                data = read_object(repo, obj)
                archive.addfile(backup.tar_info(path, len(data)), io.BytesIO(data))
                rows.append({**row, "path": path, "size": len(data), "sha256": obj["sha256"]})
        with open(work / "photos.csv", "w", encoding="utf-8-sig", newline="") as handle:
            columns = ["employee_id", "employee_name", "attendance_id", "work_date", "label", "photo_id",
                       "storage_key", "path", "content_type", "size", "sha256"]
            writer = csv.DictWriter(handle, fieldnames=columns, extrasaction="ignore")
            writer.writeheader()
            for row in rows:
                writer.writerow({k: ("'" + v if isinstance(v, str) and v.lstrip().startswith(("=", "+", "-", "@")) else v)
                                 for k, v in row.items()})
        result = {"version": "monthly-1", "month": month, "snapshot_id": directory.name,
                  "photos": rows, "checksums": {n: backup.sha_file(work / n) for n in ["photos.tar.gz", "photos.csv"]}}
        (work / "manifest.json").write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        backup.verify_archive(work / "photos.tar.gz", rows)
        if target.exists():
            raise ValueError("Output already exists")
        work.rename(target)
    print(f"Readable monthly archive verified: {target}; {len(rows)} photos")


def main():
    os.umask(0o077)
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ["nightly", "pull"]:
        p = sub.add_parser(name)
        p.add_argument("--config", required=True)
    for name in ["verify", "materialize", "export-month"]:
        p = sub.add_parser(name)
        p.add_argument("--repository", required=True)
        p.add_argument("--snapshot", default="latest")
        if name in ["materialize", "export-month"]:
            p.add_argument("--month", required=True)
            p.add_argument("--output", required=True)
    args = parser.parse_args()
    try:
        if args.command == "nightly":
            nightly(read_json(args.config))
        elif args.command == "pull":
            pull(read_json(args.config))
        elif args.command == "verify":
            _, manifest = verify(Path(args.repository).expanduser().resolve(), args.snapshot)
            print(f"Snapshot {manifest['snapshot_id']} verified: {len(manifest['photos'])} photos")
        elif args.command == "materialize":
            materialize(Path(args.repository).expanduser().resolve(), args.snapshot, args.month, Path(args.output))
        else:
            export_month(Path(args.repository).expanduser().resolve(), args.snapshot, args.month, Path(args.output))
    except KeyboardInterrupt:
        print("Backup interrupted; check app status and retry.", file=sys.stderr)
        return 130
    except (ValueError, OSError, KeyError, tarfile.TarError, subprocess.SubprocessError) as error:
        detail = str(error) if isinstance(error, ValueError) else type(error).__name__
        print(f"Backup failed: {detail}. No verified-success marker written for this attempt.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    def interrupt(_signum, _frame):
        raise KeyboardInterrupt
    signal.signal(signal.SIGTERM, interrupt)
    sys.exit(main())
