# Best practices for investigators

This is practical guidance, not legal advice. Evidentiary requirements,
disclosure duties, retention periods and privacy law vary by jurisdiction and
engagement. Agree the scope and authority in writing before collecting
anything.

## Before collection

- **Authority and scope.** Confirm in writing that the client can provide the
  material and that you are authorised to possess and review it. Record the
  engagement reference in the case description.
- **Plan the workspace location.** Use trusted, backed-up storage with enough
  capacity; evidence can be much larger than it first appears. Avoid syncing a
  live case directory to consumer cloud storage unless that is approved.
- **Synchronise the clock.** UTC timestamps are only as good as the system
  clock. Enable network time or check the clock before acquisition.
- **Identify the operator.** Use a full name, organisation and email in
  `--examiner`; do not rely on a shared login.
- **Use a write blocker or read-only mount** for original media where possible.
  Stratum never opens a source for writing, but reading can update access times
  and a mounted filesystem can still change metadata.
- **Decide what is in scope.** Record exclusions (`--exclude`) and hidden-file
  decisions in the acquisition note. Do not silently omit material.

## During acquisition

```sh
stratum add ./source \
  --origin "client handover, 2026-11-02, reception" \
  --note "two folders + one PDF; excludes *.tmp"
```

- **Record provenance now.** `--origin` and `--note` are written into every item
  record in the batch. Sources of web captures should include the URL and
  capture time if known.
- **Prefer preserved copies.** Use `--reference` only when a copy is impossible
  and document why. Reference-only items have no preserved copy; only the
  original source can be re-hashed.
- **Review metadata sidecars.** Each item gets a JSON sidecar under `metadata/`
  containing EXIF and document metadata, with its SHA-256 recorded in the
  evidence chain. Check that document titles/authors/dates make sense; PDF
  Info strings and HTML meta tags can be absent or misleading.
- **Do not follow symlinks.** Stratum records them as skipped. If a link is
  itself important evidence, record its target in a note or acquire the target
  separately.
- **Watch for source instability.** A warning that the source changed during
  acquisition means the recorded hash covers exactly the bytes read, not a
  stable file. Re-acquire from a quiescent copy if possible.
- **Do not rename or edit preserved files.** Their names are tied to item IDs and
  their hashes to the ledgers. Work from report copies.
- **Keep acquisitions small and documented.** A batch per delivery, device or
  location is easier to explain than one enormous unlabelled import.

## Storage and handling

- Keep the case directory together: `case.json`, both ledgers, `items/`,
  `reports/` and `README.md`. Do not separate the ledgers from their items.
- Back up using a method that preserves bytes and verifies restores. Test a
  restore before relying on it.
- Restrict file permissions. Stratum creates cases, ledgers and reports with
  owner-only permissions (`0700`/`0600`) and preserved copies `0444`, but your
  backup and sharing tools may be less careful.
- Encrypt at rest and in transit where the material is sensitive.
- Keep a copy offline or immutable. Verification detects modification; it
  cannot recover a deleted original.
- Record retention and destruction decisions in the case file. Do not destroy
  material subject to a hold.

## Chain of custody

The custody ledger answers "who had it, when, and how did it move?" Record
actions as they happen, not retrospectively.

1. **Open** the case with the examiner as the first custodian.
2. **Collect** source material (Stratum records this automatically).
3. **Transfer** with the recipient's name, organisation, method, purpose,
   location and a note about packaging/seal condition.
4. **Return** with the same detail.
5. **Seal** when the evidence set is complete.
6. **Unseal** only with a written reason; record the reason in the command.
7. **Verify** before disclosure and record the verification.

Useful details to put in `--note`:

- tracking number or courier reference;
- seal number and condition;
- package description and weight;
- who physically handed over and who received;
- whether the recipient signed a receipt (keep the receipt separately).

A custody record without a recipient acknowledgement proves what *you* recorded;
it does not prove the recipient agreed. Get a signed receipt or written
confirmation for transfers that matter.

## Verification and anchoring

- Run `stratum verify` before generating a final report and before disclosure.
- Use `--record` when you need an attributable, timestamped verification event
  in the ledger.
- Check the report's `MANIFEST.json` and `SHA256SUMS`.
- Anchor the roots outside the workspace: sign `SHA256SUMS`, email the roots to
  the client and keep the reply, or use an RFC 3161 timestamp.
- Re-verify after copying the case to another disk or transferring it.
- If verification fails, **stop**. Preserve the current state, do not rewrite
  the ledger, isolate the storage, and investigate. A broken chain is a finding,
  not something to repair by hand.

## Reports

- Re-hash by default (`stratum report`); use `--no-rehash` only when time or
  capacity requires it, and expect the report to say **partial**.
- Include the metadata section and `metadata.csv` when the receiving party
  needs document/EXIF information; the sidecars are referenced by hash.
- Review the acquisition issues and warnings sections before sending a report.
- Redact source paths, GPS coordinates, serial numbers and names that are not
  necessary for the recipient. The HTML report is self-contained and easy to
  redact; the JSON report is the complete audit copy, so consider sharing a
  redacted derivative rather than the raw JSON.
- Deliver reports with their `MANIFEST.json` and `SHA256SUMS`. The report alone
  does not prove the ledgers.
- Keep a copy of exactly what was delivered, and record the delivery as a
  custody action if it is a handover of evidence rather than a report.

## What Stratum does not do

| Area | Limitation |
| --- | --- |
| Disk imaging | File-level acquisition only; no deleted data, unallocated space, slack space or full partition image |
| Metadata | Records source mode and mtime; does not preserve/record ownership, ACLs, xattrs, resource forks or all timestamps |
| Source state | Reading can update source atime; a mounted source can change for other reasons |
| File type | Best-effort content identification; not a full format validator or malware analysis |
| EXIF | JPEG/TIFF/PNG/WebP; HEIF/AVIF flags capability but does not extract it |
| Document metadata | Best-effort PDF Info strings, OOXML/ODF properties, HTML meta/title and EML headers; not a full document parser |
| Metadata sidecars | Derivative artefacts; verified by hash and binding, but their content reflects the extractor at acquisition/extraction time |
| Chain | Tamper-evident, not tamper-proof without external anchoring |
| Legal | Not a substitute for legal advice, disclosure review, expert testimony or jurisdiction-specific procedure |

## Incident checklist

If something goes wrong:

1. **Stop using the workspace.** Do not run acquisition or hand-edit JSONL.
2. **Preserve the current directory** (including the lock file, if present) on
   separate media before troubleshooting.
3. **Run `stratum verify --no-rehash`** and save the output.
4. **If a preserved item or metadata sidecar is modified or missing**, record
   what you know, isolate the storage, and consider a fresh acquisition from the
   original source. Use `stratum extract --force` only for a metadata sidecar
   you can reconstruct from an unmodified item.
5. **If a ledger is broken**, do not recompute hashes or truncate records; that
   destroys the evidence of the failure. Treat the case as compromised unless
   you can establish a benign cause from an independently anchored copy.
6. **If the workspace is locked**, confirm no other Stratum process is running;
   only then use `--force-lock`. A stale lock older than 30 minutes is broken
   automatically.
7. **If case.json drifts**, do not edit it to match; investigate. The chain is
   authoritative and the report warns about drift.
8. **Preserve correspondence** about the incident and any remedial action.
