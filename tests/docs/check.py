"""Check repository Markdown without a website build or network access."""

from collections import Counter
from pathlib import Path
import re
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[2]
DOCS = ROOT / "docs"
LINK = re.compile(r"\]\(([^\s)]+)\)|(?:src|href|srcset)=\"([^\"]+)\"")


def prose(path):
    lines, fence = [], ""
    for line in path.read_text().splitlines():
        marker = re.match(r"^\s*(`{3,}|~{3,})", line)
        if marker:
            if not fence:
                fence = marker[1]
            elif marker[1][0] == fence[0] and len(marker[1]) >= len(fence):
                fence = ""
            lines.append("")
        else:
            lines.append("" if fence else line)
    return "\n".join(lines)


def anchors(text):
    result, seen = set(), Counter()
    for heading in re.findall(r"^#{1,6}\s+(.+)$", text, re.MULTILINE):
        heading = re.sub(r"\[([^]]+)\]\([^)]*\)", r"\1", heading)
        slug = re.sub(r"[^\w\- ]", "", heading.strip().lower()).replace(" ", "-")
        anchor = slug if not seen[slug] else f"{slug}-{seen[slug]}"
        while anchor in result:
            seen[slug] += 1
            anchor = f"{slug}-{seen[slug]}"
        result.add(anchor)
        seen[slug] += 1
    return result


def main():
    guides = set(DOCS.rglob("*.md"))
    sources = guides | {ROOT / "README.md", ROOT / "tests/README.md", ROOT / "benchmark/README.md", ROOT / "assets/logo/README.md"}
    content = {path: prose(path) for path in sources}
    headings = {path: anchors(text) for path, text in content.items()}
    edges, failures = {}, []
    for source in sorted(sources):
        text = content[source]
        if source in guides and len(re.findall(r"^#\s+", text, re.MULTILINE)) != 1:
            failures.append(f"{source.relative_to(ROOT)}: expected one page title")
        edges[source] = set()
        for match in LINK.finditer(text):
            href = match[1] or match[2]
            url = urlsplit(href)
            if url.scheme or url.netloc:
                continue
            target = (source.parent / unquote(url.path)).resolve() if url.path else source
            location = f"{source.relative_to(ROOT)}:{text[:match.start()].count(chr(10)) + 1}"
            if not target.is_relative_to(ROOT):
                failures.append(f"{location}: link leaves repository: {href}")
                continue
            if any(p.startswith('.') and not (i == 0 and p == '.github')
                   for i, p in enumerate(target.relative_to(ROOT).parts)):
                failures.append(f"{location}: link to local internals: {href}")
                continue
            if not target.exists():
                failures.append(f"{location}: missing target: {href}")
                continue
            if target.is_dir() and (target / "README.md").exists():
                target /= "README.md"
            edges[source].add(target)
            if url.fragment and target.suffix == ".md":
                if target not in headings:
                    headings[target] = anchors(prose(target))
                if unquote(url.fragment) not in headings[target]:
                    failures.append(f"{location}: missing heading: {href}")
    visited, pending = set(), [DOCS / "README.md"]
    while pending:
        page = pending.pop()
        if page not in visited:
            visited.add(page)
            pending.extend(edges.get(page, set()) & guides)
    for page in sorted(guides - visited):
        failures.append(f"{page.relative_to(ROOT)}: not reachable from docs/README.md")
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print(f"Checked {len(guides)} guides: titles, local links, headings, and index navigation.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
