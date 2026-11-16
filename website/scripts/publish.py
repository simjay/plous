"""Assemble development and checksum-verified release snapshots for GitHub Pages."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile


def release_version(tag):
    match = re.fullmatch(r"v([0-9]+)\.([0-9]+)\.([0-9]+)", tag)
    return tuple(map(int, match.groups())) if match else None


def unpack(archive, checksum, destination):
    expected = checksum.read_text().split()
    if len(expected) != 2 or expected[1] != archive.name:
        raise ValueError("Invalid documentation checksum file")
    if hashlib.sha256(archive.read_bytes()).hexdigest() != expected[0]:
        raise ValueError("Documentation archive checksum mismatch")
    with tarfile.open(archive) as source:
        members = source.getmembers()
        for member in members:
            target = (destination / member.name).resolve()
            if not target.is_relative_to(destination.resolve()) or not (member.isfile() or member.isdir()):
                raise ValueError("Unsafe documentation archive member")
        source.extractall(destination, members=members, filter="data")


def read_manifest(source):
    return json.loads((source / "site.json").read_text())


def publication_versions(development, releases, base):
    versions = {}
    for tag, source in sorted(releases, key=lambda pair: release_version(pair[0]), reverse=True):
        manifest = read_manifest(source)
        if manifest["version"] != tag or manifest["base"] != base + tag + "/":
            raise ValueError("Release documentation identity does not match its tag or deployment base")
        versions[tag] = (source, manifest)
    dev = read_manifest(development)
    if dev["version"] != "Development" or dev["base"] != base + "dev/":
        raise ValueError("Development documentation identity does not match deployment base")
    versions["dev"] = (development, dev)
    return versions


def build_latest(manifest, base, staging):
    checkout = "HEAD"
    if manifest["version"] != "Development":
        ref = manifest.get("revision") or f'refs/tags/{manifest["version"]}'
        subprocess.run(["git", "fetch", "--no-tags", "--depth=1", "origin", ref], check=True)
        checkout = "FETCH_HEAD"

    archive = staging / "source.tar"
    source = staging / "source"
    subprocess.run(["git", "archive", "-o", str(archive), checkout], check=True)
    with tarfile.open(archive) as packed:
        packed.extractall(source, filter="data")

    env = dict(os.environ, PLOUS_SITE_BASE=base, PLOUS_SITE_ROOT=base,
               PLOUS_DOCS_VERSION=manifest["version"], PLOUS_DOCS_SOURCE_REF=manifest["source"])
    env.pop("PLOUS_DOCS_REVISION", None)
    if manifest.get("revision"):
        env["PLOUS_DOCS_REVISION"] = manifest["revision"]
    subprocess.run(["npm", "ci", "--prefix", "website"], cwd=source, env=env, check=True)
    subprocess.run(["npm", "--prefix", "website", "run", "build"], cwd=source, env=env, check=True)
    return source / "website/dist"


def assemble(versions, root, output, base):
    manifests = (manifest for _, manifest in versions.values())
    latest = next(manifests)
    root_manifest = read_manifest(root)
    if root_manifest != {**latest, "base": base}:
        raise ValueError("Root documentation must match the latest version and use the deployment base")
    if output.exists():
        shutil.rmtree(output)
    shutil.copytree(root, output)
    for directory, (source, _) in versions.items():
        shutil.copytree(source, output / directory)
    catalog = {"latest": latest["version"], "versions": [root_manifest, *manifests]}
    (output / "versions.json").write_text(json.dumps(catalog, indent=2) + "\n")
    (output / ".nojekyll").touch()
    return latest["version"]


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def select_snapshot(release):
    tag = release["tag_name"]
    assets = {asset["name"]: asset for asset in release["assets"]}
    original = f"documentation-{tag}"
    if not {original + ".tar.gz", original + ".sha256"}.issubset(assets):
        raise ValueError(f"Incomplete documentation assets for {tag}")
    choices = [("", original, None)]
    pattern = re.compile(r"documentation-" + re.escape(tag) + r"-docs-([a-f0-9]{40})\.tar\.gz")
    for name, asset in assets.items():
        match = pattern.fullmatch(name)
        if match and name.removesuffix(".tar.gz") + ".sha256" in assets:
            choices.append((asset["created_at"], name.removesuffix(".tar.gz"), match[1]))
    _, stem, revision = max(choices)
    return stem, revision


def upload_correction(tag, directory):
    if release_version(tag) is None:
        raise ValueError("Expected vMAJOR.MINOR.PATCH")
    archives = list(directory.glob(f"documentation-{tag}-docs-*.tar.gz"))
    if len(archives) != 1:
        raise ValueError("Expected one correction archive")
    archive = archives[0]
    match = re.fullmatch(r"documentation-" + re.escape(tag) + r"-docs-([a-f0-9]{40})\.tar\.gz", archive.name)
    if not match:
        raise ValueError("Correction must identify its source commit")
    revision = match[1]
    checksum = directory / (archive.name.removesuffix(".tar.gz") + ".sha256")
    release = json.loads(gh("release", "view", tag, "--json", "isDraft,isPrerelease,assets"))
    if release["isDraft"] or release["isPrerelease"]:
        raise ValueError("Corrections require a published application release")
    names = {asset["name"] for asset in release["assets"]}
    if checksum.name in names and archive.name not in names:
        raise ValueError("Correction checksum exists without its archive")
    with tempfile.TemporaryDirectory() as temporary:
        root = Path(temporary)
        if archive.name in names:
            gh("release", "download", tag, "--dir", str(root), "--pattern", archive.name)
            archive = root / archive.name
            if checksum.name in names:
                gh("release", "download", tag, "--dir", str(root), "--pattern", checksum.name)
                checksum = root / checksum.name
            else:
                checksum = root / checksum.name
                checksum.write_text(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")
        destination = root / "site"
        destination.mkdir()
        unpack(archive, checksum, destination)
        manifest = json.loads((destination / "site.json").read_text())
        if manifest["version"] != tag or manifest.get("revision") != revision or manifest.get("source") != revision:
            raise ValueError("Correction source identity mismatch")
        for asset in (archive, checksum):
            if asset.name not in names:
                gh("release", "upload", tag, str(asset))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--upload-correction")
    parser.add_argument("--correction-directory", type=Path, default=Path("documentation-release"))
    parser.add_argument("--development", type=Path, default=Path("website/dist"))
    parser.add_argument("--output", type=Path, default=Path("website/published"))
    parser.add_argument("--base", default="/plous/")
    parser.add_argument("--repository", default="simjay/plous")
    args = parser.parse_args()
    if args.upload_correction:
        upload_correction(args.upload_correction, args.correction_directory)
        return
    base = "/" + args.base.strip("/") + "/" if args.base.strip("/") else "/"
    releases = json.loads(gh("api", "--paginate", "--slurp", f"repos/{args.repository}/releases?per_page=100"))
    with tempfile.TemporaryDirectory() as temporary:
        snapshots = []
        for release in [release for page in releases for release in page]:
            tag = release["tag_name"]
            if release["draft"] or release["prerelease"] or release_version(tag) is None:
                continue
            archive_name = f"documentation-{tag}.tar.gz"
            checksum_name = f"documentation-{tag}.sha256"
            assets = {asset["name"] for asset in release["assets"]}
            if archive_name not in assets and checksum_name not in assets:
                continue  # Releases before versioned website publication have only Markdown.
            stem, revision = select_snapshot(release)
            archive_name, checksum_name = stem + ".tar.gz", stem + ".sha256"
            folder = Path(temporary) / tag
            folder.mkdir()
            gh("release", "download", tag, "--repo", args.repository, "--dir", str(folder),
               "--pattern", archive_name, "--pattern", checksum_name)
            destination = folder / "site"
            destination.mkdir()
            unpack(folder / archive_name, folder / checksum_name, destination)
            manifest = json.loads((destination / "site.json").read_text())
            if revision and (manifest.get("revision") != revision or manifest.get("source") != revision):
                raise ValueError(f"Correction source identity mismatch for {tag}")
            snapshots.append((tag, destination))
        versions = publication_versions(args.development, snapshots, base)
        _, latest_manifest = next(iter(versions.values()))
        root = build_latest(latest_manifest, base, Path(temporary))
        latest = assemble(versions, root, args.output, base)
        print(f"Assembled {len(snapshots)} release snapshots and Development; latest is {latest}.")


if __name__ == "__main__":
    main()
