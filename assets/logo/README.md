# Plous logo

The Open course mark traces an open P-shaped route with a separate departure point. It represents the entire voyage and Plous helping the Kubernetes helmsman. Flat Kubernetes blue connects the identity to that ecosystem.

The long logo uses the mark as the `p` in custom Soft flow lettering, with the smoother Level ends `s`. Use it for README headers and other wide placements. The short logo is the same `p` on its own, for icons, avatars, and compact spaces.

`mark.svg` owns the `p` geometry and primary color. `wordmark.py` owns the custom `lous` paths and spacing. `gen.py` combines them to produce the other SVGs. The terminal welcome screen adapts the short logo to character cells; it and the app header use the same blue.

## Files

| File | Use |
| --- | --- |
| `mark.svg` | Short logo: blue p with a transparent background |
| `mark-mono.svg` | Transparent mark using `currentColor` |
| `icon.svg` / `icon-dark.svg` | Centered mark on a light or dark rounded tile |
| `lockup.svg` / `lockup-dark.svg` | Long logo: integrated plous wordmark for light or dark backgrounds |
| `png/`, `png/dark/` | Icon exports at 16, 32, 64, 128, 256, 512, and 1024 pixels |

Choose the icon and long logo by their background. The blue p is identical in both themes. For the monochrome SVG, set CSS `color` when embedding it inline, or set its root `color` attribute for a standalone file. An external `<img>` does not inherit the surrounding text color.

## Palette

| Element | Light background | Dark background |
| --- | --- | --- |
| Mark | `#326CE5` | `#326CE5` |
| Wordmark | `#172239` | `#E4EAF4` |
| Icon tile | `#F8FAFC` | `#0B1220` |

The mark uses a 12-unit rounded stroke and a point with a 6-unit radius on a 100-unit viewBox. Keep the open ends and separate point intact. The identity uses flat colors without gradients, shadows, or sheen.

## Generate assets

Run from the repository root:

```sh
python3 assets/logo/gen.py
sh assets/logo/export.sh
sh assets/logo/export.sh icon-dark.svg dark
```

SVG generation requires Python 3. PNG export requires macOS with Google Chrome installed in `/Applications`; the exporter uses a temporary browser profile.

## Wordmark and proportions

The lettering is custom vector geometry and needs no installed font. Every letter uses the same 12-unit rounded stroke as the p. The open o repeats the p's curved body; the s has horizontal ends and smooth rounded bends.

The main letter bodies share the same top and baseline. The l rises above them, and the p's departure point sits below them. The long logo uses a 375 by 124 viewBox, including clear space around the complete lettering. Preserve its paths and spacing, and scale the whole SVG proportionally.
