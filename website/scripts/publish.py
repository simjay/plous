"""Assemble development and checksum-verified release snapshots for GitHub Pages."""

import argparse
import hashlib
import html
import json
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


def redirect(destination):
    escaped = html.escape(destination, quote=True)
    script = json.dumps(destination).replace("<", "\\u003c")
    return ('<!doctype html><html lang="en"><head><meta charset="utf-8">'
            '<meta name="viewport" content="width=device-width"><title>Plous documentation</title>'
            f'<link rel="canonical" href="{escaped}"><meta name="robots" content="noindex">'
            f'<noscript><meta http-equiv="refresh" content="0;url={escaped}"></noscript>'
            f'<script>location.replace({script}+location.search+location.hash)</script>'
            f'</head><body><h1>Plous documentation</h1><a href="{escaped}">Open documentation</a></body></html>')


def assemble(development, releases, output, base):
    if output.exists():
        shutil.rmtree(output)
    output.mkdir(parents=True)
    shutil.copytree(development, output / "dev")
    versions = []
    for tag, source in sorted(releases, key=lambda pair: release_version(pair[0]), reverse=True):
        manifest = json.loads((source / "site.json").read_text())
        if manifest["version"] != tag or manifest["base"] != base + tag + "/":
            raise ValueError("Release documentation identity does not match its tag or deployment base")
        shutil.copytree(source, output / tag)
        versions.append(manifest)
    dev = json.loads((development / "site.json").read_text())
    if dev["version"] != "Development" or dev["base"] != base + "dev/":
        raise ValueError("Development documentation identity does not match deployment base")
    latest = versions[0] if versions else dev
    versions.append(dev)
    (output / "versions.json").write_text(json.dumps({"latest": latest["version"], "versions": versions}, indent=2) + "\n")
    source = output / (latest["version"] if latest is not dev else "dev")
    for page in source.rglob("*.html"):
        relative = page.relative_to(source)
        target = output / relative
        if target.exists():
            raise ValueError("Alias overlaps a version directory")
        target.parent.mkdir(parents=True, exist_ok=True)
        route = relative.as_posix().removesuffix("index.html")
        target.write_text(redirect(latest["base"] + route))
    (output / "404.html").write_text((source / "404.html").read_text())
    (output / ".nojekyll").touch()
    (output / "sitemap.xml").write_text((source / "sitemap.xml").read_text())
    return latest["version"]


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--development", type=Path, default=Path("website/dist"))
    parser.add_argument("--output", type=Path, default=Path("website/published"))
    parser.add_argument("--base", default="/plous/")
    parser.add_argument("--repository", default="simjay/plous")
    args = parser.parse_args()
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
            if not {archive_name, checksum_name}.issubset(assets):
                raise ValueError(f"Incomplete documentation assets for {tag}")
            folder = Path(temporary) / tag
            folder.mkdir()
            gh("release", "download", tag, "--repo", args.repository, "--dir", str(folder),
               "--pattern", archive_name, "--pattern", checksum_name)
            destination = folder / "site"
            destination.mkdir()
            unpack(folder / archive_name, folder / checksum_name, destination)
            snapshots.append((tag, destination))
        latest = assemble(args.development, snapshots, args.output, base)
        print(f"Assembled {len(snapshots)} release snapshots and Development; latest is {latest}.")


if __name__ == "__main__":
    main()
