#!/bin/sh
# Synthetic end-to-end demo. Creates a temporary case, acquires sample files,
# records a handover, seals the case, writes reports and verifies everything.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
BIN="$ROOT/dist/stratum"
if [ ! -x "$BIN" ]; then
    make -C "$ROOT" build
fi

DEMO=${STRATUM_DEMO_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/stratum-demo.XXXXXX")}
HOME_DIR="$DEMO/home"
CASE="$DEMO/case"
SRC="$DEMO/source"
export STRATUM_HOME="$HOME_DIR"
mkdir -p "$HOME_DIR" "$SRC/web" "$SRC/docs"

cat > "$SRC/web/pricing-page.html" <<'EOF'
<!doctype html>
<html><head><title>Acme pricing — captured page</title></head>
<body><h1>Pricing</h1><p>Demo capture. Not real evidence.</p></body></html>
EOF
printf '%%PDF-1.7\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%%%EOF\n' > "$SRC/docs/client-statement.pdf"
printf 'Interview notes from the synthetic demo.\n' > "$SRC/docs/interview-notes.txt"
cp "$SRC/docs/interview-notes.txt" "$SRC/docs/interview-notes-copy.txt"
# Synthetic JPEG with EXIF camera fields and GPS, for the metadata section.
cp "$ROOT/demo/fixtures/photo.jpg" "$SRC/web/photo.jpg"
cat > "$SRC/docs/metadata.json" <<'EOF'
{"source": "demo", "note": "synthetic evidence only"}
EOF

echo "Stratum demo"
echo "============"
echo

"$BIN" init "Synthetic demo case" \
    --dir "$CASE" --id DEMO-2026-001 \
    --examiner "Demo Examiner" --org "Demo Investigations" \
    --email "demo@example.invalid" \
    --description "Synthetic demo. Contains no real evidence." \
    --no-register
echo

"$BIN" add -w "$CASE" "$SRC" \
    --origin "synthetic demo handover" \
    --note "created by scripts/demo.sh"
echo

"$BIN" custody transfer -w "$CASE" \
    --to "Demo Recipient" --to-org "Demo Counsel" \
    --method "hand delivery" \
    --purpose "synthetic production demo"
echo

"$BIN" seal -w "$CASE" --note "demo evidence set closed"
echo

"$BIN" report -w "$CASE" -f html,md,json
echo

"$BIN" verify -w "$CASE"
echo

REPORT=$(ls -d "$CASE"/reports/*/ | tail -1)
echo "Demo case:    $CASE"
echo "HTML report:  ${REPORT}report.html"
echo
echo "Open the HTML report with your browser, or inspect:"
echo "  $BIN list -w \"$CASE\" --hash"
echo "  $BIN show -w \"$CASE\" E-0001"
echo "  $BIN verify -w \"$CASE\""
echo
echo "This demo uses synthetic files only."
