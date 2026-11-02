package report

// textTemplate is the plain-text report. It avoids backticks so it can live in
// a Go raw string; formatting is intentionally line-oriented rather than
// column-aligned so long paths and hashes stay readable.
const textTemplate = `STRATUM CASE REPORT
===================

Case ID      {{.Case.CaseID}}
Case name    {{oneLine .Case.Name}}
Examiner     {{.Case.Examiner.Display}}
Created      {{datetime .Case.Created}}
Status       {{if .Sealed}}SEALED{{else}}OPEN{{end}}{{if .SealedAt}} (sealed {{datetime .SealedAt}}){{end}}
Generated    {{datetime .GeneratedAt}} by {{.GeneratedBy.Display}}
Tool         {{.Tool}} {{.Version}}

{{if .Case.Description}}Description
-----------
{{oneLine .Case.Description}}

{{end}}INTEGRITY AT A GLANCE
---------------------
Evidence chain    {{if .EvidenceInfo.ChainOK}}OK{{else}}BROKEN OR INCOMPLETE{{end}} — {{.EvidenceInfo.Records}} record(s)
Custody chain     {{if .CustodyInfo.ChainOK}}OK{{else}}BROKEN OR INCOMPLETE{{end}} — {{.CustodyInfo.Records}} record(s)
Verification      {{if .Summary.VerificationOK}}PASSED{{else}}FAILED{{end}} — {{if .Summary.VerificationComplete}}complete (item bytes re-hashed){{else}}partial (see verification section){{end}}
Re-hashed         {{if .Summary.Rehashed}}yes{{else}}no{{end}}

SUMMARY
-------
Items             {{.Summary.ItemCount}} ({{.Summary.PreservedCount}} preserved, {{.Summary.ReferenceCount}} reference-only)
Total size        {{.Summary.TotalBytesHuman}} ({{.Summary.TotalBytes}} bytes)
Duplicates        {{.Summary.DuplicateCount}}
EXIF metadata     {{.Summary.EXIFCount}} item(s), {{.Summary.GPSCount}} with GPS coordinates
Metadata sidecars {{.Summary.MetadataCount}} item(s)
Document metadata {{.Summary.DocumentMetadataCount}} item(s)
Web captures      {{.Summary.WebCount}}
Images            {{.Summary.ImageCount}}
Documents         {{.Summary.DocumentCount}}
Acquisition issues {{.Summary.IssueCount}} ({{.Summary.ErrorCount}} error(s))

Types
{{range .Summary.Types}}  {{.Count}} x {{.Label}}
{{end}}{{if .Summary.Categories}}
Categories
{{range .Summary.Categories}}  {{.Count}} x {{.Label}}
{{end}}{{end}}{{if .Summary.Holders}}
Current custodians
{{range .Summary.Holders}}  {{.Count}} item(s): {{.Holder}}
{{end}}{{end}}
CHAIN OF EVIDENCE — WHAT AND WHEN
---------------------------------
{{if .EvidenceEvents}}{{range .EvidenceEvents}}#{{.Seq}}  {{datetime .Time}}  {{.Title}}
    {{oneLine .Subject}}{{if .Detail}} — {{oneLine .Detail}}{{end}}
    record sha256 {{.Hash}}{{if .ObjectSHA256}}
    object sha256 {{.ObjectSHA256}}{{end}}
{{end}}{{else}}No evidence records.
{{end}}
CHAIN OF CUSTODY — WHO
----------------------
{{if .CustodyEvents}}{{range .CustodyEvents}}#{{.Seq}}  {{datetime .Time}}  {{.Title}}
    {{dash .From}} -> {{dash .To}}   actor: {{dash .Actor}}
    items: {{.ItemsLabel}} ({{.ItemCount}}); anchor {{.EvidenceRoot}}
{{if .Method}}    method: {{oneLine .Method}}
{{end}}{{if .Purpose}}    purpose: {{oneLine .Purpose}}
{{end}}{{if .Location}}    location: {{oneLine .Location}}
{{end}}{{if .Note}}    note: {{oneLine .Note}}
{{end}}    record sha256 {{.Hash}}
{{end}}{{else}}No custody records.
{{end}}
EVIDENCE INVENTORY
------------------
{{if .Items}}{{range .Items}}{{.ID}}  {{oneLine .Name}}
    type:      {{.TypeLabel}}
    mode:      {{.ModeLabel}}{{if .CopyVerified}} (copy read-back verified){{end}}
    size:      {{.SizeHuman}} ({{.Size}} bytes)
    sha256:    {{.SHA256}}
    acquired:  {{datetime .AcquiredAt}} by {{.AcquiredBy.Display}}
    source:    {{oneLine .SourcePath}}
    stored:    {{if .StoredPath}}{{oneLine .StoredPath}}{{else}}— (reference only){{end}}
    metadata:  {{if .Metadata.Path}}{{oneLine .Metadata.Path}}
               sha256 {{.Metadata.SHA256}} ({{.Metadata.Size}} bytes){{else}}—{{if .Metadata.Error}} ({{oneLine .Metadata.Error}}){{end}}{{end}}
    holder:    {{dash .CurrentHolder}}
    stability: {{if .SourceStable}}stable during acquisition{{else}}source changed or disappeared during acquisition{{end}}
{{if .DuplicateOf}}    duplicate: same bytes as {{.DuplicateOf}}
{{end}}{{if .Origin}}    origin:    {{oneLine .Origin}}
{{end}}{{if .Note}}    note:      {{oneLine .Note}}
{{end}}{{if .FileType.Notes}}    type note: {{range .FileType.Notes}}{{oneLine .}} {{end}}
{{end}}{{if .EXIF.Present}}    EXIF:      {{.EXIFSummary}}
{{end}}{{end}}{{else}}No evidence items have been acquired.
{{end}}
DOCUMENT METADATA
-----------------
{{if .Items}}{{range .Items}}{{if .Document.Present}}{{.ID}}  {{oneLine .Name}}
    format:     {{dash .Document.Format}}
    title:      {{dash .Document.Title}}
    author:     {{dash .Document.Author}}
    subject:    {{dash .Document.Subject}}
    created:    {{dash .Document.Created}}
    modified:   {{dash .Document.Modified}}
    application: {{dash .Document.Application}}{{if .Document.AppVersion}} {{.Document.AppVersion}}{{end}}
    producer:   {{dash .Document.Producer}}
    company:    {{dash .Document.Company}}
{{if .Document.Fields}}    fields:{{range $k, $v := .Document.Fields}}
      {{$k}}: {{oneLine $v}}{{end}}
{{end}}{{if .Document.Notes}}    notes:      {{range .Document.Notes}}{{oneLine .}} {{end}}
{{end}}    sidecar:    {{if .Metadata.Path}}{{oneLine .Metadata.Path}} ({{.Metadata.SHA256}}){{else}}—{{end}}
{{end}}{{end}}{{else}}No items.
{{end}}
EXIF AND LOCATION METADATA
--------------------------
{{if .Items}}{{range .Items}}{{if .EXIF.Present}}{{.ID}}  {{oneLine .Name}}
    make/model: {{dash .EXIF.Make}} {{dash .EXIF.Model}}
    serial:     {{dash .EXIF.Serial}}
    lens:       {{dash .EXIF.Lens}}
    software:   {{dash .EXIF.Software}}
    original:   {{dash .EXIF.DateTimeOriginal}}{{if .EXIF.OffsetTime}} ({{.EXIF.OffsetTime}}){{end}}
    orientation {{.EXIF.Orientation}}; dimensions {{.EXIF.Width}}x{{.EXIF.Height}}
    GPS:        {{if .GPS}}{{.GPS}}{{else}}not present{{end}}
    XMP:        {{if .EXIF.XMP}}present{{else}}not detected{{end}}
{{if .EXIF.Error}}    parser:     {{oneLine .EXIF.Error}}
{{end}}{{end}}{{end}}{{else}}No items.
{{end}}{{range .Items}}{{if and (not .EXIF.Present) .EXIF.Capable}}{{.ID}}  {{oneLine .Name}}
    {{oneLine .EXIF.Note}}
{{end}}{{end}}
ACQUISITION ISSUES
------------------
{{if .Issues}}{{range .Issues}}{{datetime .Time}}  [{{.Kind}}]  {{oneLine .Path}}
    {{oneLine .Reason}}
{{end}}{{else}}No acquisition errors or skips were recorded.
{{end}}
WARNINGS
--------
{{if .Warnings}}{{range .Warnings}}- {{oneLine .}}
{{end}}{{else}}None.
{{end}}
INTEGRITY DETAIL
----------------
Evidence root SHA-256  {{.EvidenceInfo.RootSHA256}}
Evidence file SHA-256  {{.EvidenceInfo.FileSHA256}}
Custody root SHA-256   {{.CustodyInfo.RootSHA256}}
Custody file SHA-256   {{.CustodyInfo.FileSHA256}}
{{if .Verification}}case.json SHA-256     {{.Verification.CaseFileHash}}
verification            {{if .Verification.OK}}PASSED{{else}}FAILED{{end}}; {{if .Verification.Complete}}complete{{else}}partial{{end}}; rehashed={{.Verification.Rehashed}}
metadata sidecars       {{.Verification.MetadataChecked}} checked, {{.Verification.MetadataOK}} verified, {{len .Verification.MetadataMissing}} missing, {{len .Verification.MetadataModified}} modified, {{len .Verification.MetadataUnexpected}} unexpected
{{if .Verification.Errors}}Verification errors:
{{range .Verification.Errors}}  - {{oneLine .}}
{{end}}{{end}}{{if .Verification.Missing}}Missing items: {{range .Verification.Missing}}{{.}} {{end}}
{{end}}{{if .Verification.Modified}}Modified items:
{{range .Verification.Modified}}  {{.ID}} {{oneLine .Note}}
{{end}}{{end}}{{if .Verification.Unexpected}}Unexpected files:
{{range .Verification.Unexpected}}  {{.}}
{{end}}{{end}}{{if .Verification.Reference}}Reference-only items (partial verification):
{{range .Verification.Reference}}  {{.ID}}: {{oneLine .Note}}
{{end}}{{end}}{{if .Verification.MetadataMissing}}Metadata sidecars missing:
{{range .Verification.MetadataMissing}}  {{.}}
{{end}}{{end}}{{if .Verification.MetadataModified}}Metadata sidecars modified:
{{range .Verification.MetadataModified}}  {{.ID}} {{oneLine .Note}}
{{end}}{{end}}{{if .Verification.MetadataUnexpected}}Unexpected metadata files:
{{range .Verification.MetadataUnexpected}}  {{.}}
{{end}}{{end}}{{end}}
METHOD AND LIMITATIONS
----------------------
Stratum copies source files into the workspace without writing to the source,
hashes the exact bytes read, reads the preserved copy back, compares the
SHA-256, preserves the source modification time, and makes the copy read-only.
EXIF and document metadata are extracted into a JSON sidecar under metadata/,
whose SHA-256 is recorded in the evidence chain. Timestamps are UTC. The
evidence and custody ledgers are independent SHA-256 hash chains; every custody
record anchors an evidence-chain root.

A hash chain is tamper-evident, not tamper-proof: a person who can rewrite the
whole file can recompute every hash. Anchor the roots above outside this
workspace (signed manifest, email, or RFC 3161 timestamp) when that distinction
matters. A file-level acquisition does not capture deleted data, unallocated
space, alternate data streams, extended attributes or a full disk image.
Reference-only items have no preserved copy; verification is partial and
re-checking depends on the original source still being available.
Reading a source can update its access time. Reports contain sensitive paths
and may contain EXIF/GPS metadata; handle them accordingly.

ATTESTATION
-----------
This report was generated from the ledgers named above. Report files are covered
by SHA256SUMS and MANIFEST.json in this directory. To re-check the case, run:
  stratum verify --workspace <case-directory>
`

const mdTemplate = `# Stratum case report — {{.Case.CaseID}}

**{{md .Case.Name}}**

{{if .Case.Description}}> {{md .Case.Description}}

{{end}}| Field | Value |
| --- | --- |
| Case ID | <code>{{.Case.CaseID}}</code> |
| Examiner | {{md .Case.Examiner.Display}} |
| Created (UTC) | {{datetime .Case.Created}} |
| Status | {{if .Sealed}}**SEALED**{{if .SealedAt}} ({{datetime .SealedAt}}){{end}}{{else}}OPEN{{end}} |
| Generated (UTC) | {{datetime .GeneratedAt}} |
| Generated by | {{md .GeneratedBy.Display}} |
| Tool | {{.Tool}} {{.Version}} |

## Integrity at a glance

| Check | Result |
| --- | --- |
| Evidence chain | {{if .EvidenceInfo.ChainOK}}**OK**{{else}}**BROKEN OR INCOMPLETE**{{end}} — {{.EvidenceInfo.Records}} record(s) |
| Custody chain | {{if .CustodyInfo.ChainOK}}**OK**{{else}}**BROKEN OR INCOMPLETE**{{end}} — {{.CustodyInfo.Records}} record(s) |
| Re-hash verification | {{if .Summary.VerificationOK}}**PASSED**{{else}}**FAILED**{{end}} — {{if .Summary.VerificationComplete}}complete{{else}}partial{{end}} |
| Item bytes re-hashed | {{if .Summary.Rehashed}}yes{{else}}no{{end}} |
| Metadata sidecars | {{if .Verification}}{{.Verification.MetadataOK}} of {{.Verification.MetadataChecked}} verified; {{len .Verification.MetadataMissing}} missing; {{len .Verification.MetadataModified}} modified; {{len .Verification.MetadataUnexpected}} unexpected{{else}}—{{end}} |

{{if .Summary.GPSCount}}> **Privacy note:** {{.Summary.GPSCount}} item(s) contain GPS coordinates. This report contains location metadata; redact it before public release.

{{end}}## Summary

| Metric | Value |
| --- | --- |
| Items | {{.Summary.ItemCount}} ({{.Summary.PreservedCount}} preserved, {{.Summary.ReferenceCount}} reference-only) |
| Total size | {{.Summary.TotalBytesHuman}} ({{.Summary.TotalBytes}} bytes) |
| Duplicates | {{.Summary.DuplicateCount}} |
| EXIF-bearing items | {{.Summary.EXIFCount}} |
| GPS-bearing items | {{.Summary.GPSCount}} |
| Metadata sidecars | {{.Summary.MetadataCount}} |
| Document metadata | {{.Summary.DocumentMetadataCount}} |
| Web captures | {{.Summary.WebCount}} |
| Images / documents | {{.Summary.ImageCount}} / {{.Summary.DocumentCount}} |
| Acquisition issues | {{.Summary.IssueCount}} ({{.Summary.ErrorCount}} error(s)) |

### Types

| Type | Count |
| --- | --- |
{{range .Summary.Types}}| {{md .Label}} | {{.Count}} |
{{end}}
{{if .Summary.Holders}}### Current custodians

| Custodian | Items |
| --- | --- |
{{range .Summary.Holders}}| {{md .Holder}} | {{.Count}} |
{{end}}{{end}}
## Chain of evidence — what and when

| # | UTC time | Event | Subject | Detail | Record SHA-256 |
| --- | --- | --- | --- | --- | --- |
{{range .EvidenceEvents}}| {{.Seq}} | {{datetime .Time}} | {{md .Title}} | {{md .Subject}} | {{md .Detail}} | <code>{{.Hash}}</code> |
{{end}}
## Chain of custody — who

| # | UTC time | Action | From | To | Actor | Method / purpose / note | Items | Anchor |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
{{range .CustodyEvents}}| {{.Seq}} | {{datetime .Time}} | {{md .Title}} | {{md .From}} | {{md .To}} | {{md .Actor}} | {{md .Method}}{{if .Purpose}} — {{md .Purpose}}{{end}}{{if .Location}}<br>{{md .Location}}{{end}}{{if .Note}}<br>{{md .Note}}{{end}} | {{md .ItemsLabel}} ({{.ItemCount}}) | <code>{{.EvidenceRoot}}</code> |
{{end}}
## Evidence inventory

| ID | File | Type | Size | Mode | SHA-256 | Holder |
| --- | --- | --- | --- | --- | --- | --- |
{{range .Items}}| {{.ID}} | {{md .Name}} | {{md .FileType.Description}}<br>{{md .FileType.MIME}} | {{.SizeHuman}} | {{.ModeLabel}} | <code>{{.SHA256}}</code> | {{md .CurrentHolder}} |
{{end}}
{{range .Items}}
### {{.ID}} — {{md .Name}}

| Field | Value |
| --- | --- |
| Source path | <code>{{md .SourcePath}}</code> |
| Stored path | {{if .StoredPath}}<code>{{md .StoredPath}}</code>{{else}}not preserved (reference only){{end}} |
| Metadata sidecar | {{if .Metadata.Path}}<code>{{md .Metadata.Path}}</code>{{else}}—{{end}} |
| Metadata SHA-256 | {{if .Metadata.SHA256}}<code>{{.Metadata.SHA256}}</code> ({{.Metadata.Size}} bytes){{else}}not recorded{{end}} |
| Acquired (UTC) | {{datetime .AcquiredAt}} by {{md .AcquiredBy.Display}} |
| Size | {{.SizeHuman}} ({{.Size}} bytes) |
| SHA-256 | <code>{{.SHA256}}</code> |
| File type | {{md .TypeLabel}} |
| Confidence | {{md .FileType.Confidence}} |
| Extension match | {{if .FileType.ExtensionMatch}}yes{{else}}**no** — extension .{{md .FileType.InputExtension}} does not match content{{end}} |
| Source stable | {{if .SourceStable}}yes{{else}}**no** — changed during acquisition{{end}} |
| Duplicate of | {{if .DuplicateOf}}{{.DuplicateOf}}{{else}}—{{end}} |
| Current holder | {{md .CurrentHolder}} |
{{if .Origin}}| Origin | {{md .Origin}} |
{{end}}{{if .Note}}| Note | {{md .Note}} |
{{end}}{{if .EXIF.Present}}| EXIF | {{md .EXIFSummary}} |
{{end}}{{if .GPS}}| GPS | {{.GPS}} |
{{end}}
{{if .FileType.Notes}}Type notes:

{{range .FileType.Notes}}- {{md .}}
{{end}}{{end}}
{{if .EXIF.Error}}EXIF parser note: {{md .EXIF.Error}}

{{end}}{{end}}
## Document metadata

{{if .Summary.DocumentMetadataCount}}| ID | Format | Title | Author | Created | Application | Sidecar SHA-256 |
| --- | --- | --- | --- | --- | --- | --- |
{{range .Items}}{{if .Document.Present}}| {{.ID}} | {{md .Document.Format}} | {{md .Document.Title}} | {{md .Document.Author}} | {{md .Document.Created}} | {{md .Document.Application}} | {{if .Metadata.SHA256}}<code>{{.Metadata.SHA256}}</code>{{else}}—{{end}} |
{{end}}{{end}}{{else}}No document metadata was found in the acquired files.

{{end}}
## EXIF and location metadata

{{if .Summary.EXIFCount}}| ID | File | Make / model | Serial | Original time | GPS | XMP |
| --- | --- | --- | --- | --- | --- | --- |
{{range .Items}}{{if .EXIF.Present}}| {{.ID}} | {{md .Name}} | {{md .EXIF.Make}} {{md .EXIF.Model}} | {{md .EXIF.Serial}} | {{md .EXIF.DateTimeOriginal}} | {{if .GPS}}{{.GPS}}{{else}}—{{end}} | {{if .EXIF.XMP}}yes{{else}}no{{end}} |
{{end}}{{end}}{{else}}No EXIF metadata was found in the acquired files.

{{end}}
## Acquisition issues

{{if .Issues}}| UTC time | Kind | Path | Reason |
| --- | --- | --- | --- |
{{range .Issues}}| {{datetime .Time}} | {{md .Kind}} | <code>{{md .Path}}</code> | {{md .Reason}} |
{{end}}{{else}}No acquisition errors or skips were recorded.

{{end}}
## Warnings

{{if .Warnings}}{{range .Warnings}}- {{md .}}
{{end}}{{else}}None.

{{end}}
## Integrity detail

| Field | SHA-256 |
| --- | --- |
| Evidence root | <code>{{.EvidenceInfo.RootSHA256}}</code> |
| Evidence file | <code>{{.EvidenceInfo.FileSHA256}}</code> |
| Custody root | <code>{{.CustodyInfo.RootSHA256}}</code> |
| Custody file | <code>{{.CustodyInfo.FileSHA256}}</code> |
{{if .Verification}}| case.json | <code>{{.Verification.CaseFileHash}}</code> |
{{end}}
{{if .Verification}}**Verification result:** {{if .Verification.OK}}PASSED{{else}}FAILED{{end}}; {{if .Verification.Complete}}complete{{else}}partial{{end}}; item bytes re-hashed: {{if .Verification.Rehashed}}yes{{else}}no{{end}}.

{{if .Verification.Errors}}**Verification errors:**

{{range .Verification.Errors}}- {{md .}}
{{end}}{{end}}{{if .Verification.Missing}}**Missing items:** {{range .Verification.Missing}}<code>{{.}}</code> {{end}}

{{end}}{{if .Verification.Modified}}**Modified items:**

| ID | Note |
| --- | --- |
{{range .Verification.Modified}}| {{.ID}} | {{md .Note}} |
{{end}}{{end}}{{if .Verification.Unexpected}}**Unexpected files:** {{range .Verification.Unexpected}}<code>{{md .}}</code> {{end}}

{{end}}{{if .Verification.Reference}}**Reference-only items (partial verification):**

| ID | Note |
| --- | --- |
{{range .Verification.Reference}}| {{.ID}} | {{md .Note}} |
{{end}}{{end}}{{if .Verification.MetadataMissing}}**Metadata sidecars missing:** {{range .Verification.MetadataMissing}}<code>{{.}}</code> {{end}}

{{end}}{{if .Verification.MetadataModified}}**Metadata sidecars modified:**

| ID | Note |
| --- | --- |
{{range .Verification.MetadataModified}}| {{.ID}} | {{md .Note}} |
{{end}}{{end}}{{if .Verification.MetadataUnexpected}}**Unexpected metadata files:** {{range .Verification.MetadataUnexpected}}<code>{{md .}}</code> {{end}}

{{end}}{{end}}
## Method and limitations

Stratum copies source files into the workspace without writing to the source, hashes the exact bytes read, reads the preserved copy back, compares the SHA-256, preserves the source modification time, and makes the copy read-only. EXIF and document metadata are extracted into a JSON sidecar under <code>metadata/</code>; the sidecar's SHA-256 is recorded in the evidence chain. Timestamps are UTC. The evidence and custody ledgers are independent SHA-256 hash chains; every custody record anchors an evidence-chain root.

A hash chain is **tamper-evident, not tamper-proof**: a person who can rewrite the whole file can recompute every hash. Anchor the roots above outside this workspace (signed manifest, email, or RFC 3161 timestamp) when that distinction matters. A file-level acquisition does not capture deleted data, unallocated space, alternate data streams, extended attributes or a full disk image. Reference-only items have no preserved copy; verification is partial. Reading a source can update its access time. Reports contain sensitive paths and may contain EXIF/GPS metadata.

## Attestation

This report was generated from the ledgers named above. Report files are covered by <code>SHA256SUMS</code> and <code>MANIFEST.json</code> in this directory.

To re-check the case:

~~~sh
stratum verify --workspace <case-directory>
~~~
`
