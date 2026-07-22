#!/usr/bin/env bash
# Regenerates the embedded IEEE MAC address registry used for vendor lookups.
#
# The registry changes as new blocks are assigned, so refresh it occasionally
# and commit the result. Output: internal/scanner/oui.txt.gz
set -euo pipefail

SRC="https://standards-oui.ieee.org/oui/oui.csv"
OUT="internal/scanner/oui.txt.gz"
TMP="$(mktemp -t oui.XXXXXX)"
trap 'rm -f "$TMP"' EXIT

echo "Downloading $SRC"
curl -sfL --max-time 180 "$SRC" -o "$TMP"

echo "Building $OUT"
python3 - "$TMP" "$OUT" <<'PY'
import csv, gzip, re, sys

src, out = sys.argv[1], sys.argv[2]
rows = []
with open(src, newline='', encoding='utf-8', errors='replace') as fh:
    for r in csv.DictReader(fh):
        prefix = (r.get('Assignment') or '').strip().upper()
        name = ' '.join((r.get('Organization Name') or '').split())
        if not prefix or not name:
            continue
        if not re.fullmatch(r'[0-9A-F]{6,9}', prefix):
            continue
        if name.lower() in ('private', 'ieee registration authority'):
            continue
        rows.append((prefix, name))

rows.sort()
with gzip.open(out, 'wt', encoding='utf-8', compresslevel=9) as fh:
    fh.write('\n'.join(f'{p}\t{n}' for p, n in rows) + '\n')

print(f"  {len(rows)} entries")
PY

echo "Done. Commit $OUT to embed it in the binary."
