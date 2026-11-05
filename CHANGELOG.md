# Changelog

All notable changes to Stratum are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [1.2.0] — 2026-11-05

### Added

- `stratum compare <file>` checks a file's SHA-256 against a case's evidence
  ledger, or a standalone `evidence.jsonl`, and reports whether it was already
  captured or is new evidence. Supports `--hash`, `--json`, item ID/name
  context and sidecar validation.

## [1.1.1] — 2026-11-04

### Fixed

- Published the module as `github.com/subsll/stratum` with matching internal
  imports, so `go install github.com/subsll/stratum/cmd/stratum@latest` resolves
  correctly.
- Removed local Go module-cache artefacts that were accidentally included in
  the `v1.1.0` tag.

### Changed

- `go install` is now the recommended installation method in the README.
- Version bumped to 1.1.1.

## [1.1.0] — 2026-11-02

### Added

- Per-item JSON metadata sidecars under `metadata/`, containing EXIF and
  document metadata, bound to the item SHA-256 and hashed in the evidence chain.
- Document metadata extraction for PDF Info strings/XMP presence, OOXML
  `docProps/core.xml` and `docProps/app.xml`, ODF `meta.xml`, HTML title/meta
  tags and RFC 822 email headers.
- Automatic metadata sidecar creation during `stratum add`.
- `stratum extract [--all | <item-id>...] [--force]` to add or refresh metadata
  sidecars for existing items, with an `item_metadata` evidence record and a
  `custody_metadata` custody record.
- Metadata verification: sidecar size/hash, JSON validity, item binding and
  EXIF/document equality, plus unexpected/missing/modified sidecar detection.
- Metadata rows in text/Markdown/HTML reports, `metadata.csv`, JSON item
  metadata and manifest verification counts.
- `stratum show` displays the sidecar path/hash/status and document metadata.
- `docmeta` and `metadata` package tests, plus acquisition, verification, report
  and CLI tests for the new artefact.

### Changed

- Version bumped to 1.1.0.
- The workspace layout now includes `metadata/`; existing workspaces are
  upgraded lazily when metadata is extracted.

## [1.0.0] — 2026-11-02

### Added

- Case workspaces with `init`, discovery, local registry and workspace lock.
- Verified file acquisition from files and directories: read-only source,
  streaming SHA-256, copy read-back, fsync, timestamp preservation, read-only
  copies, duplicate detection, source-stability checks.
- Content-first file-type detection for documents, images, archives, web
  captures, email, databases, media, executables and raw/unknown data, with
  extension-mismatch warnings and confidence levels.
- EXIF extraction for JPEG, TIFF, PNG and WebP (make, model, serial, lens,
  timestamps, orientation, dimensions, GPS); HEIF/AVIF capability flag.
- Chain of evidence (`evidence.jsonl`) compatible with Subsoil's hash-chain
  format, including strict canonical JSON verification.
- Chain of custody (`custody.jsonl`) with collection, transfer, return, seal,
  unseal and verification records; every record anchors an evidence root.
- `seal` / documented `unseal`, with acquisition blocked while sealed.
- `verify`: both chains, cross-links, re-hashing of preserved items, missing/
  modified/unexpected/symlink detection, reference-only partiality, optional
  recorded verification.
- Reports: text, Markdown, standalone HTML, complete JSON, items/evidence/
  custody CSV, `MANIFEST.json` and `SHA256SUMS`.
- CLI with welcome screen, contextual help, JSON output for listing/verification
  and typed exit codes.
- Test suite covering chain tamper detection, strict canonical encoding, file
  types, EXIF (including malformed input), workspace lifecycle, locking,
  acquisition, verification, report escaping/checksums and the full CLI
  workflow.
- Documentation: evidence format, best practices, security policy and
  contribution guide.
