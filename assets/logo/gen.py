"""Generates the Plous logo set. Run: python3 gen.py

The mark is pi built from three blocks: a bar, and two tall blocks beneath it.
It reads as the letter and, for a deployment tool, as workloads sitting under a
control plane. Form, proportions, corner radii and palette are settled; this
script is the single source for every file in this directory.

Surface treatment ("depth"): continuous-curvature block corners, a soft drop
shadow lifting the blocks off the tile, a tinted rather than flat tile, and a
sheen across the top of each block.

Colour. Light: white-to-lavender tile, blocks #3B82F6 -> #7C3AED. Dark: near-black
tile, blocks lifted to #4F8DF7 -> #9066F0 so they keep the same hue identity while
holding contrast against the dark ground. The block gradient is declared in user
space across the glyph's bounding box, so all three blocks share one continuous
ramp; an objectBoundingBox gradient would restart it in each block.

Geometry: the blocks come from five numbers (glyph width, bar height, the gap
under the bar, block height, the gap between blocks) plus two corner radii, so a
change is a number rather than a redrawn path. Block height sits near 1.4x block
width, which gives the mark its vertical stance and keeps it from reading as a
dashboard grid.
"""
import math

import wordmark

S  = 256
N  = 5.2       # superellipse exponent — the tile
NB = 7.5       # superellipse exponent — the blocks

W, BH, GAP, KH, SPLIT = 168, 40, 11, 108, 14
RXB, RXK = 10, 13                        # approved corner radii
MARK_SCALE = 0.80                        # mark size in the lockup, relative to the icon
H   = BH + GAP + KH
TOP = 128 - H/2
X0  = 128 - W/2
KW  = (W - SPLIT) / 2
KY  = TOP + BH + GAP
PARTS = [(X0, TOP, W, BH, RXB), (X0, KY, KW, KH, RXK), (X0+KW+SPLIT, KY, KW, KH, RXK)]

THEMES = {
    "light": dict(g0="#3B82F6", g1="#7C3AED", t0="#FFFFFF", t1="#E8EBFF",
                  shadow="#1E1B4B", shadow_o=".22", sheen=".20", word="#4F46E5"),
    "dark":  dict(g0="#4F8DF7", g1="#9066F0", t0="#1A1D2E", t1="#0B0D17",
                  shadow="#000000", shadow_o=".45", sheen=".16", word="#C7D2FE"),
}
SOLID = "#4F46E5"          # single-colour stand-in for the gradient

def supell(cx, cy, w, h, n, pts=160):
    p = []
    for i in range(pts):
        t = 2*math.pi*i/pts
        c, s = math.cos(t), math.sin(t)
        p.append((cx + (w/2)*math.copysign(abs(c)**(2/n), c),
                  cy + (h/2)*math.copysign(abs(s)**(2/n), s)))
    return "M%.2f %.2f " % p[0] + " ".join("L%.2f %.2f" % q for q in p[1:]) + " Z"

SQ = supell(128, 128, S, S, N, 200)

def curved(fill):
    return "".join('\n    <path d="%s" fill="%s"/>'
                   % (supell(x+w/2, y+h/2, w, h, NB), fill) for x, y, w, h, _ in PARTS)

def square(fill):
    return "".join('\n  <rect x="%g" y="%g" width="%g" height="%g" rx="%g" fill="%s"/>'
                   % (x, y, w, h, rx, fill) for x, y, w, h, rx in PARTS)

def defs(th, tile=True, shadow=True):
    d = ('    <linearGradient id="g" gradientUnits="userSpaceOnUse" x1="%g" y1="%g" x2="%g" y2="%g">\n'
         '      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>\n'
         '    </linearGradient>\n' % (X0, TOP, X0+W, KY+KH, th["g0"], th["g1"]))
    d += ('    <linearGradient id="s" x1="0" y1="0" x2="0" y2="1">\n'
          '      <stop offset="0" stop-color="#FFFFFF" stop-opacity="%s"/>\n'
          '      <stop offset=".55" stop-color="#FFFFFF" stop-opacity="0"/>\n'
          '    </linearGradient>\n' % th["sheen"])
    if tile:
        d += ('    <linearGradient id="w" x1="0" y1="0" x2="0" y2="1">\n'
              '      <stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>\n'
              '    </linearGradient>\n' % (th["t0"], th["t1"]))
    if shadow:
        d += ('    <filter id="e" x="-20%%" y="-20%%" width="140%%" height="140%%">\n'
              '      <feDropShadow dx="0" dy="4" stdDeviation="5" flood-color="%s" '
              'flood-opacity="%s"/>\n    </filter>\n' % (th["shadow"], th["shadow_o"]))
    return d

def wrap(body, defs_, w=S, h=S):
    return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" '
            'role="img" aria-label="Plous">\n  <defs>\n%s  </defs>%s\n</svg>\n'
            % (w, h, w, h, defs_, body))

def icon(theme):
    th = THEMES[theme]
    body = ('\n  <path d="%s" fill="url(#w)"/>' % SQ
            + '\n  <g filter="url(#e)">%s%s\n  </g>' % (curved("url(#g)"), curved("url(#s)")))
    return wrap(body, defs(th))

def mark(theme):
    th = THEMES[theme]
    return wrap('\n  <g filter="url(#e)">%s%s\n  </g>' % (curved("url(#g)"), curved("url(#s)")),
                defs(th, tile=False))

def lockup(theme):
    """Mark + wordmark, on a tight canvas.

    The mark is scaled to MARK_SCALE so it sits at about 1.1x the wordmark's
    height. At full size it is 1.37x, which reads as the mark shouting over the
    word. Everything below is derived — the gap scales with the mark, the
    wordmark is optically centred on the mark's axis, and the margins match — so
    changing MARK_SCALE or the typeface re-lays the lockup out correctly.
    """
    th = THEMES[theme]
    x0, y0, x1, y1 = wordmark.BOUNDS
    s        = MARK_SCALE
    mark_w   = W * s
    mark_h   = (KY + KH - TOP) * s
    word_w   = x1 - x0
    content  = max(mark_h, y1 - y0)
    # margin also has to clear the drop shadow, which reaches ~19 units below the
    # blocks at full size (dy 4 plus three standard deviations of a 5px blur).
    margin   = max(0.12 * content, 19 * s + 4)
    height   = content + 2 * margin
    cy       = height / 2
    gap      = 0.22 * mark_w
    # scale the mark about its own centre, then set its left edge on the margin
    dx = margin - (128 - (W / 2) * s)
    tf = ('translate(%.2f,%.2f) translate(128,128) scale(%.4f) translate(-128,-128)'
          % (dx, cy - 128, s))
    width = margin + mark_w + gap + word_w + margin
    body = ('\n  <g transform="%s">' % tf
            + '\n  <g filter="url(#e)">%s%s\n  </g>' % (curved("url(#g)"), curved("url(#s)"))
            + '\n  </g>'
            + '\n  <g transform="translate(%.2f,%.2f)">'
              '\n    <path d="%s" fill="%s"/>\n  </g>'
              % (margin + mark_w + gap - x0, cy - (y0 + y1) / 2, wordmark.PATH, th["word"]))
    return wrap(body, defs(th, tile=False), w=round(width), h=round(height))

open("icon.svg", "w").write(icon("light"))
open("icon-dark.svg", "w").write(icon("dark"))
open("mark.svg", "w").write(mark("light"))
open("mark-dark.svg", "w").write(mark("dark"))
open("lockup.svg", "w").write(lockup("light"))
open("lockup-dark.svg", "w").write(lockup("dark"))

# Flat, untreated forms. No filters or gradients, so they survive print, embroidery,
# and the 16px favicon, where a drop shadow and a tint only turn to mud.
def plain(tile_fill, block_fill):
    return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="256" height="256" '
            'role="img" aria-label="Plous">\n  <path d="%s" fill="%s"/>%s\n</svg>\n'
            % (SQ, tile_fill, square(block_fill)))

open("icon-flat.svg", "w").write(plain("#FFFFFF", SOLID))
open("icon-flat-dark.svg", "w").write(plain("#0B0D17", "#7C93F5"))
open("icon-mono.svg", "w").write(plain("#FFFFFF", "currentColor"))

print("wrote icon, icon-dark, mark, mark-dark, lockup, lockup-dark, "
      "icon-flat, icon-flat-dark, icon-mono")
