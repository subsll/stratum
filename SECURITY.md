# Security policy

## Reporting a vulnerability

Please report security issues privately to the maintainer rather than opening a
public issue. Include:

- affected version (`stratum version`) and platform;
- a minimal reproducer, ideally with synthetic files;
- the impact you believe it has on evidence integrity or confidentiality;
- whether you are willing to be credited.

Do not include real case material, personal data or credentials.

## Scope and threat model

Stratum is a local evidence-acquisition and custody tool. It is designed to be
robust against malformed evidence files and to make accidental modification
detectable. It is **not** a sandbox, a malware analyser or a secure enclave.

In scope:

- crashes, panics, excessive memory/CPU or path traversal caused by malicious
  evidence files;
- writes outside the workspace or into source directories;
- bypasses of the read-only/verified-copy acquisition path;
- failures to detect modified, missing or unrecorded preserved items or
  metadata sidecars;
- canonical-encoding or chain-verification flaws;
- crashes, path traversal or injection through document-metadata parsers
  (PDF/OOXML/ODF/HTML/email);
- report injection (HTML/CSV/Markdown) through file names or metadata;
- incorrect custody/holder derivation that could misrepresent a handover.

Out of scope / known limitations:

- a local attacker who can rewrite the entire workspace and both ledgers
  consistently (a hash chain is tamper-evident, not tamper-proof);
- a compromised host, OS, clock, Go toolchain or binary;
- filesystem/journal/metadata loss outside Stratum's control;
- the absence of disk-imaging features (deleted data, unallocated space, full
  images, xattrs/ADS/resource forks);
- HEIF/AVIF EXIF extraction (flagged as capable, not parsed);
- PDF document metadata outside the scanned first/last 2 MiB, or ambiguous
  Info strings and HTML meta tags;
- legal admissibility or regulatory compliance.

## Integrity guidance

- Anchor the evidence/custody roots outside the workspace. Sign `SHA256SUMS` or
  send the roots to the client and keep the reply.
- Keep the original ledgers and preserved items together.
- Verify with re-hashing before relying on a report.
- Back up the workspace using a method that verifies restores.

## Supported versions

Security fixes are applied to the latest released minor version. There is no
long-term support commitment for pre-1.0 builds.
