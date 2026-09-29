#!/bin/sh
# Exports PNGs from an SVG in this directory.
#   sh export.sh                      -> icon.svg      into png/
#   sh export.sh icon-dark.svg  dark  -> icon-dark.svg into png/dark/
set -e
SRC="${1:-icon.svg}"
SUB="${2:-}"
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
DIR="$(cd "$(dirname "$0")" && pwd)"
OUT="$DIR/png${SUB:+/$SUB}"
mkdir -p "$OUT"
for S in 16 32 64 128 256 512 1024; do
  printf '<html><body style="margin:0"><img src="%s" width="%s" height="%s"></body></html>' \
    "file://$DIR/$SRC" "$S" "$S" > "/tmp/plous-icon-$S.html"
  "$CHROME" --headless --disable-gpu --allow-file-access-from-files \
    --default-background-color=00000000 \
    --screenshot="$OUT/icon-$S.png" --window-size="$S,$S" \
    "file:///tmp/plous-icon-$S.html" 2>/dev/null
  rm -f "/tmp/plous-icon-$S.html"
done
echo "wrote $OUT"
