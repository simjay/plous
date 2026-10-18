"""Generate the short logo and integrated wordmark from their shared geometry."""
from pathlib import Path
import xml.etree.ElementTree as ET

import wordmark

DIRECTORY = Path(__file__).resolve().parent
THEMES = {
    "light": {"tile": "#F8FAFC", "word": "#172239"},
    "dark": {"tile": "#0B1220", "word": "#E4EAF4"},
}

source = ET.parse(DIRECTORY / "mark.svg").getroot()
blue = source.attrib["color"]
geometry = []
for child in source:
    if child.tag.rsplit("}", 1)[-1] in {"title", "desc"}:
        continue
    for element in child.iter():
        element.tag = element.tag.rsplit("}", 1)[-1]
    geometry.append(ET.tostring(child, encoding="unicode").strip())
mark = "\n    ".join(geometry)


def wrap(body, width=100, height=100, color=None):
    color_attribute = f' color="{color}"' if color else ""
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width:g} {height:g}" '
        f'width="{width:g}" height="{height:g}"{color_attribute} '
        'role="img" aria-label="Plous">\n'
        f'  {body}\n</svg>\n'
    )


def icon(theme):
    tile = THEMES[theme]["tile"]
    return wrap(
        f'<rect width="100" height="100" rx="22" fill="{tile}"/>\n'
        f'  <g transform="translate(-4.5 0)">\n    {mark}\n  </g>',
        color=blue,
    )


def lockup(theme):
    letters = "\n      ".join(
        f'<path data-letter="{letter}" transform="translate({x} 0)" d="{path}"/>'
        for letter, x, path in wordmark.LETTERS
    )
    stroke = source.find("path").attrib["stroke-width"]
    return wrap(
        f'<g transform="translate({wordmark.ORIGIN[0]} {wordmark.ORIGIN[1]})">\n'
        f'    {mark}\n'
        f'    <g fill="none" stroke="{THEMES[theme]["word"]}" stroke-width="{stroke}" '
        f'stroke-linecap="round" stroke-linejoin="round">\n'
        f'      {letters}\n    </g>\n  </g>',
        width=wordmark.WIDTH, height=wordmark.HEIGHT, color=blue,
    )


outputs = {"mark-mono.svg": wrap(mark)}
for theme, suffix in (("light", ""), ("dark", "-dark")):
    outputs[f"icon{suffix}.svg"] = icon(theme)
    outputs[f"lockup{suffix}.svg"] = lockup(theme)
for name, content in outputs.items():
    (DIRECTORY / name).write_text(content, encoding="utf-8")
print("wrote " + ", ".join(outputs))
