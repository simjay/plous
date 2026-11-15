"""Validate generated pages, local assets, links, anchors, and version coverage offline."""

from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit
import json
import os
import sys

ROOT = Path(__file__).resolve().parents[2]
DIST = Path(os.environ.get("PLOUS_SITE_DIST", ROOT / "website/dist")).resolve()
BASE = "/" + os.environ.get("PLOUS_SITE_BASE", "/plous").strip("/")
BASE = BASE.rstrip("/") + "/"


class Page(HTMLParser):
    def __init__(self, source):
        super().__init__()
        self.source = source
        self.ids = set()
        self.links = []
        self.titles = 0
        self.h1 = 0
        self.feed(source)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if "id" in attrs:
            self.ids.add(attrs["id"])
        self.titles += tag == "title"
        self.h1 += tag == "h1"
        for attribute in ("href", "src", "poster"):
            if attrs.get(attribute):
                self.links.append(attrs[attribute])
        if tag == "img" and "alt" not in attrs:
            raise ValueError("Image lacks alternative text")


def check_links(pages, failures):
    for file, page in pages.items():
        if page.titles != 1 or page.h1 != 1:
            failures.append(f"{file.relative_to(DIST)}: expected one title and one h1")
        for href in page.links:
            url = urlsplit(href)
            if url.scheme or url.netloc:
                continue
            pathname = unquote(url.path)
            if pathname.startswith("/"):
                if not pathname.startswith(BASE):
                    failures.append(f"{file.relative_to(DIST)}: link outside deployment base: {href}")
                    continue
                target = DIST / pathname[len(BASE):]
            else:
                target = file.parent / pathname if pathname else file
            target = target.resolve()
            if not target.is_relative_to(DIST):
                failures.append(f"{file.relative_to(DIST)}: link leaves build: {href}")
                continue
            if target.is_dir():
                target /= "index.html"
            if not target.is_file():
                failures.append(f"{file.relative_to(DIST)}: missing target: {href}")
            elif url.fragment and target in pages and unquote(url.fragment) not in pages[target].ids:
                failures.append(f"{file.relative_to(DIST)}: missing anchor: {href}")


def current_routes():
    routes = {""}
    for source in (ROOT / "docs").rglob("*.md"):
        relative = source.relative_to(ROOT / "docs")
        if relative.parts[0] == "contributing":
            continue
        route = relative.parent if source.name == "README.md" else relative.with_suffix("")
        routes.add("docs/" if route == Path(".") else f"docs/{route.as_posix()}/")
    return routes


def check_vitepress(root, manifest, pages, failures):
    if any(route.startswith("docs/contributing/") for route in manifest["routes"]):
        failures.append(f"{root}: maintainer pages appear in the public route list")
    if not list((root / "assets").rglob("*localSearchIndex*.js")):
        failures.append(f"{root}: missing VitePress search index")
    for route in ("", "docs/getting-started/"):
        page = pages.get(root / route / "index.html")
        screenshot = manifest["base"] + "assets/plous-overview.png"
        if page and screenshot not in page.links:
            failures.append(f"{root / route}: missing the original workspace screenshot")
    for route, example, count in [
        ("docs/getting-started/", "panel-focus", 3),
        ("docs/mcp/setup/", "mcp-target", 2),
    ]:
        page = pages.get(root / route / "index.html")
        if page and not all(f"{example}-{role}-{index}" in page.ids
                            for role in ("tab", "panel") for index in range(count)):
            failures.append(f"{route}: missing tab or panel IDs")
    for route in ("docs/getting-started/", "docs/mcp/setup/", "docs/reference/configuration/"):
        page = pages.get(root / route / "index.html")
        if page and not all(term in page.source for term in ('class="copy"', '--shiki-light:', '--shiki-dark:')):
            failures.append(f"{route}: missing copy button or syntax colors")


def check_versions(roots, pages, failures):
    for root in roots:
        manifest = json.loads((root / "site.json").read_text())
        current_renderer = manifest.get("renderer") == "vitepress"
        if current_renderer:
            check_vitepress(root, manifest, pages, failures)
        else:
            if not (root / "assets/app_bg.wasm").is_file() or not (root / "pagefind/pagefind.js").is_file():
                failures.append(f"{root}: missing archived runtime or search assets")
        for route in manifest["routes"]:
            page = pages.get(root / route / "index.html")
            marker = 'id="app"' if current_renderer else 'data-rune-static'
            if not page or marker not in page.source:
                failures.append(f"{root}: missing rendered page for {route or '/'}")


def main():
    failures = []
    pages = {file: Page(file.read_text()) for file in DIST.rglob("*.html")}
    if not pages:
        failures.append("No website pages were built")
    versions = DIST / "versions.json"
    if versions.exists():
        roots = [DIST / version["base"][len(BASE):] for version in json.loads(versions.read_text())["versions"]]
    else:
        roots = [DIST]
        site = json.loads((DIST / "site.json").read_text())
        expected = current_routes()
        if site.get("renderer") != "vitepress" or set(site["routes"]) != expected:
            failures.append("The new build must cover exactly the public guide sources using VitePress")
    check_versions(roots, pages, failures)
    check_links(pages, failures)
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print(f"Verified {len(pages)} HTML pages, routes, bookmarks, code examples, and search assets ({BASE}).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
