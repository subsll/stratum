# Contributing

Thanks for considering a contribution to Stratum. The project is deliberately
small, dependency-free and conservative about evidence integrity.

## Build and test

Requires Go 1.25 or newer.

```sh
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
make build
make demo
```

Every change should keep the full test suite and `go vet` green. Please add a
test for new detection signatures, EXIF parsing paths, chain behaviour, report
branches or CLI flags.

## Design rules

1. **No third-party Go dependencies** without a strong reason. A static,
   auditable binary is a feature.
2. **Never write to a source file.** Acquisition opens sources read-only.
3. **Do not record evidence you cannot verify.** A failed copy read-back is an
   error, not an item.
4. **The ledger is authoritative.** `case.json` is a convenience marker. Do not
   move trust into mutable metadata.
5. **Chain changes are format changes.** The field order, canonical JSON and
   hash rules are public and compatible with Subsoil. If you change them, update
   `docs/evidence-format.md`, the Python verifier and the Subsoil compatibility
   note, and bump `FormatVersion`.
6. **Reports are derivative.** Report generation must not mutate either ledger.
7. **Uncertainty is explicit.** Confidence, warnings, extension mismatches,
   reference-only mode, document-metadata limitations and partial verification
   belong in output, not in a comment.
8. **Bound untrusted input.** Evidence files are adversarial. Every parser must
   have size/count limits, bounds checks and panic-safe tests.
9. **Metadata is evidence about evidence.** Sidecars are read-only, bound to the
   item SHA-256, and their own SHA-256 is recorded in the chain. Do not treat a
   sidecar as authoritative if verification fails.

## Code layout

| Path | Purpose |
| --- | --- |
| `cmd/stratum` | thin `main` wrapper |
| `internal/app` | CLI dispatch, commands, flags, welcome/help |
| `internal/chain` | append-only hash-chained JSONL ledger |
| `internal/acquire` | walking, hashing, verified copy, ledger writes |
| `internal/filekind` | content-based file identification |
| `internal/exif` | JPEG/TIFF/PNG/WebP EXIF parser |
| `internal/docmeta` | PDF/OOXML/ODF/HTML/email document metadata parser |
| `internal/metadata` | metadata sidecar writer/reader/verifier |
| `internal/workspace` | case directory, case.json, metadata/, registry, lock |
| `internal/casestate` | replay of both ledgers into case state |
| `internal/custody` | custody scope/holder interpretation |
| `internal/verify` | chain, cross-link and item verification |
| `internal/report` | text/MD/HTML/JSON/CSV/manifest output |
| `internal/model` | shared data types |

## Pull requests

- Keep commits focused and explain the evidence-handling impact.
- Update `CHANGELOG.md` for user-visible changes.
- Do not include real evidence, personal data or case material in tests or
  issues. Use synthetic fixtures.
- For security issues, follow [SECURITY.md](SECURITY.md), not a public issue.

## Module path

The canonical module path is `github.com/subsll/stratum`. It matches the
repository URL, so `go install github.com/subsll/stratum/cmd/stratum@latest`
resolves correctly. If you publish a fork under another URL, rename the module
and all internal imports, then update the `-X .../internal/model.Version`
linker path in `Makefile`.
