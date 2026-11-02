# Evidence and custody format

Stratum writes two append-only JSONL ledgers per case:

| File | Question | Record types |
| --- | --- | --- |
| `evidence.jsonl` | **What** was observed, hashed or done, and **when** | `case_created`, `acquisition_started`, `item_acquired`, `item_metadata`, `item_error`, `acquisition_finished`, `case_sealed`, `case_unsealed`, `verification` |
| `custody.jsonl` | **Who** held, collected, transferred, returned, sealed or verified it | `custody_open`, `custody_collect`, `custody_transfer`, `custody_return`, `custody_seal`, `custody_unseal`, `custody_verify`, `custody_metadata` |

The format is intentionally the same as [Subsoil](https://github.com/subsll/subsoil):
one compact JSON object per line, each line committing to its own contents and
the previous line's hash. Subsoil's verifier accepts a Stratum ledger
structurally (it does not interpret custody semantics).

## Goals

- Every file, hash, type and observation is individually hashed.
- Changing one byte of one record is detectable.
- Deleting, reordering, reformatting or adding records is detectable.
- Verification needs no key material, no database and no network access.
- Every custody record is anchored to an evidence-chain root.
- Extracted EXIF/document metadata is itself a JSON artefact with its own
  SHA-256, stored under `metadata/` and referenced from the evidence chain.
- The JSON is simple enough to verify with a short Python script.

## File format

UTF-8 text, one compact JSON object per line, newline terminated. Field order is
fixed:

```json
{"seq":3,"time":"2026-11-02T12:00:00.123456Z","type":"item_acquired","target":"E-0001","source":"fs","prev_hash":"...","data":{"id":"E-0001","name":"page.html","sha256":"..."},"hash":"..."}
```

| Field | Meaning |
| --- | --- |
| `seq` | 1-based record number; contiguous, no gaps |
| `time` | observation time, UTC, RFC 3339 with nanoseconds |
| `type` | record type |
| `target` | case ID for case/acquisition/custody events, item ID for `item_acquired`, path for `item_error` |
| `source` | `stratum` for tool events, `fs` for acquisition, `custody` for custody |
| `prev_hash` | hash of the previous record; omitted entirely for `seq` 1 |
| `data` | type-specific object, stored exactly as hashed |
| `hash` | lowercase hex SHA-256 of the canonical payload |

Blank lines are invalid. A final newline is required. Lines must be
canonical: no extra whitespace, no reordered keys, no duplicate keys. Stratum
rejects a ledger that parses as JSON but is not byte-for-byte canonical, because
otherwise inserting harmless whitespace would change the file without changing
the chain.

## Evidence record types

### `case_created` (seq 1)

```json
{
  "case_id": "CASE-20261102-46EC91",
  "name": "Smith v Acme",
  "slug": "smith-v-acme",
  "description": "optional",
  "examiner": {"name": "Jane Doe", "org": "Doe Investigations", "email": "jane@example.com"},
  "environment": {"tool": "stratum", "version": "1.1.0", "hostname": "...", "username": "...", "os": "linux", "arch": "amd64", "go_version": "go1.25.0"}
}
```

### `acquisition_started`

```json
{
  "operator": {"name": "Jane Doe", "org": "...", "email": "..."},
  "paths": ["/absolute/source/dir"],
  "cwd": "/where/the/command/ran",
  "options": {
    "copy": true,
    "exclude": ["*.tmp"],
    "origin": "client delivery",
    "note": "optional",
    "skip_hidden": false,
    "follow_symlinks": false
  },
  "environment": {"tool": "stratum", "...": "..."}
}
```

### `item_acquired`

The `data` object is a complete item record. It is self-contained so a report
row can be tied directly to an evidence record.

| Field | Meaning |
| --- | --- |
| `id` | `E-0001`-style item ID |
| `name` | original basename |
| `source_path` | absolute source path observed at acquisition |
| `stored_path` | workspace-relative path (`items/E-0001__name`), or empty for reference-only |
| `mode` | `preserved` or `reference` |
| `size` | number of bytes hashed |
| `sha256` | SHA-256 of exactly those bytes |
| `source_mtime` | source modification time, UTC |
| `source_mode` | source Unix mode string |
| `acquired_at` | acquisition timestamp, UTC |
| `acquired_by` | operator |
| `copy_verified` | true when the read-back hash matched |
| `source_stable` | false when size/mtime changed during acquisition |
| `duplicate_of` | first item ID with the same SHA-256, if any |
| `origin`, `note` | optional provenance |
| `file_type` | content-based identification: id, MIME, category, description, extension match, confidence, magic, notes |
| `exif` | EXIF presence and fields, including GPS strings |
| `document` | PDF/OOXML/ODF/HTML/email document metadata (title, author, dates, application, fields) |
| `metadata` | sidecar path, SHA-256, size and extraction time |

For a preserved item, the hash was computed while writing the temporary copy
and confirmed by reading the copy back. The source was never opened for writing.
For a reference item there is no preserved copy; the hash covers bytes read from
the source at acquisition time.

### `item_metadata`

Written by `stratum extract` when metadata is added or replaced for an existing
item. Acquisition normally records metadata directly in `item_acquired`.

```json
{
  "item_id": "E-0001",
  "operator": {"name": "Jane Doe"},
  "metadata": {
    "path": "metadata/E-0001__photo.jpg.json",
    "sha256": "38aa85e2...",
    "size": 746,
    "extracted_at": "2026-11-02T12:39:40.123456Z"
  },
  "exif": {"present": true, "make": "TestCam", "gps": {...}},
  "document": {"present": false}
}
```

### `item_error`

```json
{"path": "/source/link", "kind": "skipped", "reason": "symbolic link not followed; target: ..."}
```

`kind` is `error`, `skipped` or `warning`. A warning can accompany an
`item_acquired` record (for example, the source changed during acquisition).

### `acquisition_finished`

```json
{
  "operator": {"name": "Jane Doe"},
  "items_added": 12,
  "item_ids": ["E-0001", "..."],
  "issues": 1,
  "errors": 0,
  "bytes": 66423900,
  "duplicates": 1
}
```

### `case_sealed` / `case_unsealed`

```json
{"operator": {"name": "Jane Doe"}, "note": "evidence set closed", "item_count": 12, "bytes": 66423900}
{"operator": {"name": "Jane Doe"}, "reason": "late production received"}
```

### `verification`

Written only by `stratum verify --record`, after a successful check:

```json
{
  "operator": {"name": "Jane Doe"},
  "ok": true,
  "complete": true,
  "rehashed": true,
  "chains_ok": true,
  "cross_links_ok": true,
  "items_checked": 12,
  "items_ok": 12,
  "missing": 0,
  "modified": 0,
  "unexpected": 0,
  "reference_only": 0,
  "metadata_checked": 12,
  "metadata_ok": 12,
  "metadata_missing": 0,
  "metadata_modified": 0,
  "metadata_unexpected": 0
}
```

## Custody record types

Every custody record's `data` is:

```json
{
  "action": "transferred",
  "actor": {"name": "Jane Doe", "org": "Doe Investigations"},
  "from": "Jane Doe (Doe Investigations)",
  "to": {"name": "Bob Smith", "org": "Acme Legal", "email": "bob@example.com"},
  "method": "hand delivery",
  "purpose": "production to counsel",
  "location": "Acme Legal, Reception",
  "note": "sealed box, seal intact",
  "items": ["E-0001", "E-0002"],
  "item_count": 2,
  "evidence_root": "97d062bc0a56e678bdc27b5c38be9978e8d2b2162ca0aadff694d5bcf4faf456"
}
```

`items` is either an explicit list or `["*"]`, meaning every item acquired
before the event timestamp. `item_count` snapshots the scope size at event time.
`evidence_root` is the evidence-chain root (last record hash) at the moment the
custody action was recorded. This is the cross-anchor between the two ledgers.

Actions and record types:

| Record type | `action` | Holder change |
| --- | --- | --- |
| `custody_open` | `opened` | examiner becomes initial custodian |
| `custody_collect` | `collected` | operator becomes custodian of the listed new items |
| `custody_transfer` | `transferred` | `to` becomes custodian of the scope |
| `custody_return` | `returned` | `to` becomes custodian of the scope |
| `custody_seal` | `sealed` | none; case-level record |
| `custody_unseal` | `unsealed` | none; case-level record |
| `custody_verify` | `verified` | none; case-level record |
| `custody_metadata` | `metadata` | none; case-level record |

For case-level records (`sealed`, `unsealed`, `verified`, `metadata`), `to` is
an empty person object (`{"name":""}`) because no handover occurred, and `from`
summarises the current custodian or custodian groups. They are not handovers.

## Metadata sidecars

Every acquired item has a JSON sidecar under `metadata/`:

```json
{
  "format_version": 1,
  "tool": "stratum",
  "version": "1.1.0",
  "item_id": "E-0001",
  "item_name": "photo.jpg",
  "item_sha256": "3216f16b...",
  "extracted_at": "2026-11-02T12:21:39.123456Z",
  "exif": {
    "present": true,
    "container": "jpeg",
    "make": "TestCam",
    "model": "Model X",
    "gps": {"latitude": "51.5000000", "longitude": "-7.0000000", "altitude": "100"}
  },
  "document": {
    "present": false
  }
}
```

- The sidecar name is `metadata/<item-id>__<original-name>.json` (sanitised).
- It is written read-only (0444), fsynced and read back.
- Its SHA-256 and size are stored in `item_metadata.metadata` (or in the
  `item_acquired` data for automatic extraction).
- It is bound to the item by `item_sha256`; verification compares the sidecar's
  EXIF/document objects with the item record and checks the sidecar hash/size.
- A sidecar is a derivative of the item. If it is modified or missing, the
  evidence chain still verifies but `stratum verify` reports a metadata
  integrity failure.
- Existing items from older versions can be given sidecars with
  `stratum extract --all` (or re-extracted with `--force`).

Automatic extraction covers EXIF plus document metadata from PDF Info strings,
OOXML core/app properties, ODF `meta.xml`, HTML title/meta tags and RFC 822
email headers. It is best-effort; format parsers are bounded, panic-safe and
never execute content.

## Canonical payload

The hash is computed over exactly these bytes, with **no trailing newline**:

```text
{"seq":<int>,"time":"<RFC3339Nano>","type":"<type>","target":"<target>","source":"<source>","prev_hash":"<hash>","data":<data JSON>}
```

Rules:

1. Field order is exactly `seq`, `time`, `type`, `target`, `source`,
   `prev_hash`, `data`.
2. `prev_hash` is omitted entirely for the first record.
3. `data` is the JSON value exactly as stored: compact, object keys sorted
   lexicographically, HTML escaping disabled (`<`, `>`, `&` are literal), and
   **numbers are integers only**. GPS coordinates and altitude are decimal
   strings, not floats, so independent verifiers do not depend on language-
   specific float formatting.
4. `hash` is the lowercase hex SHA-256 of the UTF-8 bytes above.

Go's encoder escapes U+2028 and U+2029 even with HTML escaping disabled. If
those characters appear in user data, a re-implementation must escape them the
same way (`\u2028`, `\u2029`).

## Chain rules

- `seq` starts at 1 and increases by exactly 1 per record.
- `prev_hash` equals the previous record's `hash`.
- The **root hash** is the last record's `hash`.
- An empty ledger is invalid for verification.
- The file must end with a newline.
- Every line must be canonical JSON (see above).
- The **file hash** is SHA-256 of the exact file bytes; it changes if any byte
  changes, even if the per-record canonical hash still verifies.

## Cross-links and verification

`stratum verify` (and the verification section of every report) checks:

1. **Evidence chain**: every record's canonical hash, sequence and previous
   link.
2. **Custody chain**: the same checks.
3. **Cross-links**:
   - every custody record has a non-empty `evidence_root`;
   - each `evidence_root` is the hash of a record that exists in the evidence
     chain (so it was a real evidence state, not an invented value);
   - each explicit `items` entry names an acquired item;
   - each acquired item is covered by a custody collection/opening event;
   - each item has a current custodian.
4. **Item bytes** (unless `--no-rehash`): every preserved item is a regular
   file at the recorded path, has the recorded size and hashes to the recorded
   SHA-256; missing, modified, symlinked and unrecorded files are reported.
5. **Metadata sidecars** (unless `--no-rehash`): every recorded sidecar exists
   as a regular file, has the recorded size and SHA-256, is valid JSON, is
   bound to the item's SHA-256, and contains the same EXIF/document values as
   the item record. Missing, modified and unrecorded sidecars are reported.
6. **Case metadata**: `case.json` is hashed; drift from the chain is warned
   about, but the chain is authoritative.

`--record` appends a `verification` evidence record and a `custody_verify`
custody record after a successful check. Both are ordinary chain records, so
the verification itself is timestamped, attributable and tamper-evident.

## Verify with Subsoil

The chain format is compatible with Subsoil's verifier:

```sh
subsoil verify path/to/evidence.jsonl
subsoil verify path/to/custody.jsonl
```

Subsoil checks the generic hash chain and prints the root/file hashes. It does
not know which `data` fields are item records, so it cannot check cross-links or
re-hash evidence files. Use `stratum verify` for a full case check.

This compatibility was tested against Subsoil 1.3.2. A future Subsoil change to
its canonical format would need to be matched here.

## Verify with Python

This independent verifier reproduces the canonical payload. It works because
the ledgers contain only strings, integers, booleans, null, arrays and objects
(no floats).

```python
import hashlib, json, sys

path = sys.argv[1]
prev, seq = "", 0

for lineno, raw in enumerate(open(path, encoding="utf-8"), 1):
    if raw.endswith("\n"):
        raw = raw[:-1]
    if not raw.strip():
        raise SystemExit(f"blank line {lineno}")
    rec = json.loads(raw)
    seq += 1
    assert rec["seq"] == seq, f"sequence gap at {seq}"
    assert rec.get("prev_hash", "") == prev, f"broken link at {seq}"

    payload = {
        "seq": rec["seq"],
        "time": rec["time"],
        "type": rec["type"],
        "target": rec["target"],
        "source": rec["source"],
    }
    if rec.get("prev_hash"):
        payload["prev_hash"] = rec["prev_hash"]
    payload["data"] = rec["data"]

    canon = json.dumps(payload, separators=(",", ":"), ensure_ascii=False)
    digest = hashlib.sha256(canon.encode()).hexdigest()
    assert digest == rec["hash"], f"hash mismatch at {seq}"

    # The stored line = canonical payload + the hash field.
    expected_line = canon[:-1] + ',"hash":' + json.dumps(rec["hash"]) + "}"
    if expected_line != raw:
        raise SystemExit(f"non-canonical line {lineno}")

    prev = digest

print(f"OK: {seq} records, root {prev}")
```

Check the file hash too:

```python
import hashlib
got = hashlib.sha256(open("evidence.jsonl", "rb").read()).hexdigest()
manifest = json.load(open("MANIFEST.json"))
assert got == manifest["evidence"]["file_sha256"]
```

(If you do not have a manifest, compare with `SHA256SUMS` from the report
directory.)

## Anchoring

A self-contained hash chain is **tamper-evident, not tamper-proof**. A person
who can rewrite the entire file can recompute every hash. The chain proves the
ledger was not edited *after* you received it; it does not prove the ledger was
not fabricated.

Anchor at least the two root hashes and the ledger file hashes outside the
workspace:

- sign `SHA256SUMS` with a key the recipient trusts;
- send the roots in an email to the client and keep the reply;
- use an RFC 3161 timestamp authority;
- record the roots in a separate case-management system.

The generated `MANIFEST.json` contains both roots and report-file hashes, and
`SHA256SUMS` covers the whole report directory. Keep the original ledgers even
after generating reports.

## Custody semantics and wildcards

`["*"]` applies to items acquired **before** the event timestamp, not future
items. This is why every custody record also stores `item_count`: a historical
report can show how many items were in scope even after the case grows.

A transfer with explicit item IDs moves only those items. If selected items have
different current custodians, Stratum requires separate transfers rather than
recording a handover that did not happen.

Sealing is an evidence-ledger action plus a case-level custody record. It blocks
new acquisitions (`stratum add`) but does not block transfers, verification or
reports. `stratum unseal --reason` appends to both ledgers; there is no silent
unseal.
