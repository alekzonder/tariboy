#!/usr/bin/env python3
import argparse
import base64
import binascii
import hashlib
import json
import os
import re
import shutil
from datetime import datetime, timezone
from pathlib import Path


VERSION_RE = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+")
ARCHIVE_RE = re.compile(r"[A-Za-z0-9._-]+\.app\.tar\.gz")


def regular_file(path: Path, description: str) -> None:
    if not path.is_file() or path.is_symlink():
        raise ValueError(f"{description} is not a regular file: {path}")


def decode_signature(value: str) -> None:
    try:
        envelope = base64.b64decode(value, validate=True).decode("utf-8")
        lines = envelope.splitlines()
        packet = base64.b64decode(lines[1], validate=True)
        global_signature = base64.b64decode(lines[3], validate=True)
    except (binascii.Error, UnicodeDecodeError, IndexError) as error:
        raise ValueError("updater signature is not valid Tauri minisign encoding") from error
    if (
        len(lines) != 4
        or not lines[0].startswith("untrusted comment:")
        or len(packet) != 74
        or not lines[2].startswith("trusted comment:")
        or len(global_signature) != 64
    ):
        raise ValueError("updater signature is not valid Tauri minisign encoding")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("version")
    parser.add_argument("archive", type=Path)
    parser.add_argument("signature", type=Path)
    parser.add_argument("release_dir", type=Path)
    args = parser.parse_args()

    version = args.version
    archive = args.archive
    signature_path = args.signature
    release_dir = args.release_dir
    if not VERSION_RE.fullmatch(version):
        raise ValueError("version must be an exact stable numeric version")
    if not release_dir.is_dir() or release_dir.is_symlink():
        raise ValueError(f"release directory is not a directory: {release_dir}")
    regular_file(archive, "updater archive")
    if archive.stat().st_size == 0:
        raise ValueError("updater archive is empty")
    if not ARCHIVE_RE.fullmatch(archive.name):
        raise ValueError(f"unsafe updater archive basename: {archive.name}")
    regular_file(signature_path, "updater signature")
    if signature_path.name != archive.name + ".sig":
        raise ValueError("updater signature must be named after its archive")
    signature = signature_path.read_text(encoding="utf-8").strip()
    if not signature:
        raise ValueError("updater signature is empty")
    decode_signature(signature)

    metadata_path = release_dir / "release.json"
    checksums_path = release_dir / "SHA256SUMS"
    regular_file(metadata_path, "release metadata")
    regular_file(checksums_path, "release checksums")
    metadata = json.loads(metadata_path.read_text(encoding="utf-8"))
    if not isinstance(metadata, dict) or metadata.get("version") != version:
        raise ValueError("release metadata version does not match updater version")

    destination_archive = release_dir / archive.name
    destination_signature = release_dir / signature_path.name
    for destination in (destination_archive, destination_signature):
        if destination.exists() or destination.is_symlink():
            raise ValueError(f"release asset already exists: {destination.name}")

    checksum_lines = checksums_path.read_text(encoding="utf-8").splitlines()
    if not checksum_lines:
        raise ValueError("release checksums are empty")
    existing_names = {line.split(maxsplit=1)[-1].lstrip("*") for line in checksum_lines}
    if archive.name in existing_names or signature_path.name in existing_names:
        raise ValueError("release checksums already contain updater assets")

    url = f"https://github.com/alekzonder/tariboy/releases/download/v{version}/{archive.name}"
    manifest = {
        "version": version,
        "notes": f"Tariboy {version}",
        "pub_date": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "platforms": {"darwin-aarch64": {"url": url, "signature": signature}},
    }
    checksum_lines.extend(
        [
            f"{sha256(archive)}  {archive.name}",
            f"{sha256(signature_path)}  {signature_path.name}",
        ]
    )

    suffix = f".tmp.{os.getpid()}"
    staged_archive = release_dir / f".{archive.name}{suffix}"
    staged_signature = release_dir / f".{signature_path.name}{suffix}"
    staged_checksums = release_dir / f".SHA256SUMS{suffix}"
    staged_manifest = release_dir / f".latest.json{suffix}"
    staged = (staged_archive, staged_signature, staged_checksums, staged_manifest)
    try:
        shutil.copyfile(archive, staged_archive)
        shutil.copyfile(signature_path, staged_signature)
        staged_checksums.write_text("\n".join(checksum_lines) + "\n", encoding="utf-8")
        staged_manifest.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
        staged_archive.replace(destination_archive)
        staged_signature.replace(destination_signature)
        staged_checksums.replace(checksums_path)
        staged_manifest.replace(release_dir / "latest.json")
    finally:
        for path in staged:
            path.unlink(missing_ok=True)


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, json.JSONDecodeError) as error:
        raise SystemExit(f"error: {error}") from None
