# Plous logo

The mark forms π from three blocks: a horizontal bar above two vertical blocks.

Plous takes its name from the Greek word for voyage. The owner selected the plain π mark, indigo palette, small corner radii, and the depth treatment with a tinted tile, soft shadow, and sheen. Exo 2 600 is the selected wordmark. The TUI's semantic colors are separate from this identity.

`gen.py` generates the SVGs. Edit its geometry and palette, then regenerate them. Run these commands from `assets/logo/` in the repository checkout:

```sh
python3 gen.py                      # regenerate every SVG
sh export.sh                        # icon.svg      -> png/
sh export.sh icon-dark.svg dark     # icon-dark.svg -> png/dark/
```

SVG generation requires Python 3. PNG export currently requires macOS with Google Chrome installed in `/Applications`; `export.sh` uses that executable directly.

## Wordmark

Exo 2 600, set at 130px with -0.02 em tracking, **outlined to a path** in `wordmark.py`.
Exo 2 is licensed under the SIL Open Font License, which permits use in a logo. Source:
<https://fonts.google.com/specimen/Exo+2>.

The wordmark uses paths and needs no installed font. Keep the paths when embedding the SVG.

To change the typeface, download its TTF and regenerate the outlines:

```sh
python3 outline_wordmark.py path/to/Font.ttf "Family Name" 600   # rewrites wordmark.py
python3 gen.py
```

`outline_wordmark.py` requires `fontTools` and a TTF file.

## Files

| File | Use |
| --- | --- |
| `icon.svg` / `icon-dark.svg` | The app icon. Light tile and dark tile |
| `mark.svg` / `mark-dark.svg` | The glyph without its tile, for documents and anywhere beside text |
| `lockup.svg` / `lockup-dark.svg` | Mark and wordmark, horizontal. Tight canvas, so it fills the box you give it |
| `icon-flat.svg` / `icon-flat-dark.svg` | No gradients or filters. Print, embroidery, and the 16px favicon |
| `icon-mono.svg` | Blocks take the surrounding `currentColor` |
| `png/`, `png/dark/` | Raster exports at 16–1024px with transparent corners |

Pick the dark files by the background the mark sits on, not by the viewer's theme: `icon.svg` on
light, `icon-dark.svg` on dark. In HTML, `<picture>` with `prefers-color-scheme` does this.

## Lockup proportions

`MARK_SCALE` (0.80) makes the mark about 1.1 times the wordmark's height in the lockup.

The gap is 0.22 of the mark's width. The mark and wordmark are vertically centered, with equal
side margins and enough space for the shadow. The layout is recalculated when `MARK_SCALE`
or the typeface changes.

Set the SVG's display width to control the lockup's size.

## Palette

| | Blocks | Tile |
| --- | --- | --- |
| Light | `#3B82F6` → `#7C3AED` | `#FFFFFF` → `#E8EBFF` |
| Dark | `#4F8DF7` → `#9066F0` | `#1A1D2E` → `#0B0D17` |

The dark variant uses brighter blocks for contrast. The single-color version uses `#4F46E5`.

## Notes

The block gradient is declared with `gradientUnits="userSpaceOnUse"` across the glyph's bounding box
so all three blocks share one gradient. An `objectBoundingBox` gradient would restart in each block.

Trademark clearance remains outstanding before public release or a CNCF submission.
