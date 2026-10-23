"""Render a Homebrew formula from a verified release source archive."""

import argparse
import hashlib
from pathlib import Path
import re
import tarfile


def version_numbers(version):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("Expected MAJOR.MINOR.PATCH")
    return tuple(map(int, version.split(".")))


def render(tag, archive):
    if not tag.startswith("v"):
        raise ValueError("Expected a vMAJOR.MINOR.PATCH tag")
    version = tag[1:]
    version_numbers(version)
    with tarfile.open(archive, "r:gz") as source:
        versions = [member for member in source.getmembers()
                    if member.name.endswith("/cmd/plous/VERSION") and member.isfile()]
        if len(versions) != 1:
            raise ValueError("Source archive must contain exactly one cmd/plous/VERSION")
        embedded = source.extractfile(versions[0]).read().decode().strip()
        if embedded != version:
            raise ValueError("Release tag does not match the embedded version")
        root = versions[0].name.removesuffix("cmd/plous/VERSION")
        module = source.extractfile(root + "go.mod").read().decode()
        go_version = re.search(r"^go ([0-9]+\.[0-9]+)(?:\.[0-9]+)?$", module, re.MULTILINE)
        if not go_version:
            raise ValueError("Source go.mod must declare a Go version")
    checksum = hashlib.sha256(Path(archive).read_bytes()).hexdigest()
    template = Path(__file__).with_name("plous.rb.in").read_text()
    return (template.replace("@VERSION@", version).replace("@SHA256@", checksum)
            .replace("@GO_SERIES@", go_version[1]))


def already_published(tag, formula):
    if not formula.exists():
        return False
    versions = re.findall(
        r'^  url "https://github\.com/simjay/plous/archive/refs/tags/v([0-9]+\.[0-9]+\.[0-9]+)\.tar\.gz"$',
        formula.read_text(), re.MULTILINE)
    if len(versions) != 1:
        raise ValueError("Existing formula must have one Plous release source URL")
    return version_numbers(versions[0]) >= version_numbers(tag[1:])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("archive", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--current", type=Path)
    args = parser.parse_args()
    formula = render(args.tag, args.archive)
    if args.current and already_published(args.tag, args.current):
        print("The tap already contains this version or a newer version.")
        return
    args.output.write_text(formula)


if __name__ == "__main__":
    main()
