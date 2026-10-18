#!/bin/sh
# Exports PNGs from an SVG in this directory.
#   sh export.sh                     -> icon.svg into png/
#   sh export.sh icon-dark.svg dark  -> icon-dark.svg into png/dark/
set -eu
SRC="${1:-icon.svg}"
SUB="${2:-}"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
DIR="$(cd "$(dirname "$0")" && pwd)"
OUT="$DIR/png${SUB:+/$SUB}"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT HUP INT TERM
mkdir -p "$OUT"
for S in 16 32 64 128 256 512 1024; do
  printf '<html><body style="margin:0"><img src="%s" width="%s" height="%s"></body></html>' \
    "file://$DIR/$SRC" "$S" "$S" > "$TMP/icon.html"
  "$CHROME" --headless --disable-gpu --allow-file-access-from-files \
    --user-data-dir="$TMP/profile" --no-first-run --no-default-browser-check \
    --disable-background-networking --hide-scrollbars \
    --default-background-color=00000000 \
    --screenshot="$OUT/icon-$S.png" --window-size="$S,$S" \
    "file://$TMP/icon.html" 2>/dev/null
done
echo "wrote $OUT"
