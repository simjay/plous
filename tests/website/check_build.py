"""Validate every generated page, local asset, link and anchor without network access."""

from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit
import os
import json
import re
import sys

ROOT = Path(__file__).resolve().parents[2]
DIST = Path(os.environ.get("PLOUS_SITE_DIST", ROOT / "website/dist")).resolve()
BASE = "/" + os.environ.get("PLOUS_SITE_BASE", "/plous").strip("/")
BASE = BASE.rstrip("/") + "/"


class Page(HTMLParser):
    def __init__(self, source):
        super().__init__()
        self.ids = set()
        self.links = []
        self.titles = 0
        self.h1 = 0
        self.stack = []
        self.code_colors = {}
        self.feed(source)

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        key = attrs.get("id", "")
        if attrs.get("aria-label", "").endswith(" code example"):
            key = f"code-block-{len(self.code_colors)}"
            self.code_colors[key] = set()
        if tag not in {"meta", "link", "img", "input", "br", "hr"}:
            self.stack.append((tag, key))
        for _, ancestor in self.stack:
            if ancestor in self.code_colors:
                self.code_colors[ancestor].update(re.findall(r"(?:^|;)color:(var\(--[a-z-]+\)|#[a-f0-9]{6})", attrs.get("style", "")))
        if "id" in attrs:
            self.ids.add(attrs["id"])
        self.titles += tag == "title"
        self.h1 += tag == "h1"
        for attribute in ("href", "src"):
            if attrs.get(attribute):
                self.links.append(attrs[attribute])
        if tag == "img" and "alt" not in attrs:
            raise ValueError("Image lacks alternative text")

    def handle_endtag(self, tag):
        for index in range(len(self.stack) - 1, -1, -1):
            if self.stack[index][0] == tag:
                del self.stack[index:]
                break



def contrast(first, second):
    def luminance(value):
        value = value.lstrip("#")
        if len(value) == 3:
            value = "".join(char * 2 for char in value)
        channels = [int(value[i:i + 2], 16) / 255 for i in (0, 2, 4)]
        linear = [c / 12.92 if c <= 0.04045 else ((c + 0.055) / 1.055) ** 2.4 for c in channels]
        return sum(c * weight for c, weight in zip(linear, (0.2126, 0.7152, 0.0722)))
    a, b = sorted((luminance(first), luminance(second)))
    return (b + 0.05) / (a + 0.05)



def main():
    failures = []
    pages = {file: Page(file.read_text()) for file in DIST.rglob("*.html")}
    if not pages:
        failures.append("No website pages were built")
    homepage = (DIST / "index.html").read_text() if (DIST / "index.html").exists() else ""
    manifest = DIST / "versions.json"
    roots = [DIST / version["base"][len(BASE):] for version in json.loads(manifest.read_text())["versions"]] if manifest.exists() else [DIST]
    for root in roots:
        for asset in ("assets/app.js", "assets/app_bg.wasm", "assets/geist.ttf", "assets/geist-mono.ttf"):
            if not (root / asset).is_file():
                failures.append(f"Missing Rune runtime asset: {root / asset}")
        if not list(root.rglob("pagefind.js")):
            failures.append(f"Missing search index: {root}")
    if not manifest.exists() and 'data-rune-static' not in homepage:
        failures.append("Homepage was not exported by Rune")
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
            elif pathname:
                target = file.parent / pathname
            else:
                target = file
            target = target.resolve()
            if not target.is_relative_to(DIST.resolve()):
                failures.append(f"{file.relative_to(DIST)}: link leaves build: {href}")
                continue
            if target.is_dir():
                target /= "index.html"
            if not target.is_file():
                failures.append(f"{file.relative_to(DIST)}: missing target: {href}")
            elif url.fragment and target in pages and unquote(url.fragment) not in pages[target].ids:
                failures.append(f"{file.relative_to(DIST)}: missing anchor: {href}")
    for root in roots:
        if manifest.exists():
            routes = json.loads((root / "site.json").read_text())["routes"]
            targets = [(route, root / route / "index.html") for route in routes]
        else:
            targets = []
            for source in (ROOT / "docs").rglob("*.md"):
                relative = source.relative_to(ROOT / "docs")
                route = relative.with_suffix("")
                if route.name == "README":
                    route = route.parent
                targets.append((relative, root / "docs" / route / "index.html"))
        for name, target in targets:
            if target not in pages:
                failures.append(f"Missing documentation page: {root}: {name}")
            elif 'data-rune-static' not in target.read_text():
                failures.append(f"Documentation page was not exported by Rune: {root}: {name}")
    if not manifest.exists():
        examples = {
            "docs/getting-started/index.html": [("code-block-0", 1)],
            "docs/mcp/setup/index.html": [("code-block-0", 2), ("code-block-1", 2)],
            "docs/reference/configuration/index.html": [("code-block-1", 2)],
        }
        for route, blocks in examples.items():
            page = pages.get(DIST / route)
            for block, minimum in blocks:
                colors = page.code_colors.get(block, set()) if page else set()
                if len(colors) < minimum:
                    failures.append(f"{route}: {block} lacks syntax token colors")
                source = (DIST / route).read_text() if page else ""
                for theme in (r":root", r":root\[data-theme=dark\]"):
                    style = re.search(theme + r"\{([^}]+)\}", source)
                    palette = dict(re.findall(r"--([a-z-]+):(#[a-f0-9]{3,6})(?:;|$)", style[1])) if style else {}
                    for color in colors:
                        token = re.fullmatch(r"var\(--([a-z-]+)\)", color)
                        value = palette.get(token[1]) if token else color
                        if not value or "surface" not in palette or contrast(value, palette["surface"]) < 4.5:
                            failures.append(f"{route}: {block} token {color} has insufficient contrast in {theme}")

    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print(f"Verified {len(pages)} HTML pages: source coverage, titles, images, local links, anchors, code colors and search assets ({BASE}).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
