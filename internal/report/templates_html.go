package report

// htmlTemplate is a self-contained, print-friendly report. It loads no
// JavaScript, fonts or images, so the file can be archived and opened offline.
const htmlTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Stratum case report — {{.Case.CaseID}}</title>
<style>
:root {
  --ink: #17211c;
  --muted: #5b6b62;
  --paper: #f5f2ea;
  --card: #fffdf8;
  --line: #d9d3c4;
  --accent: #2f6b4f;
  --accent-soft: #e4efe7;
  --ok: #1f6b43;
  --ok-soft: #e2f1e7;
  --warn: #8a5a00;
  --warn-soft: #fbf0d6;
  --bad: #8d2d2d;
  --bad-soft: #f8e3e1;
  --mono: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
}
* { box-sizing: border-box; }
html { scroll-behavior: smooth; }
body {
  margin: 0;
  color: var(--ink);
  background: var(--paper);
  font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
}
a { color: var(--accent); }
.wrap { max-width: 1180px; margin: 0 auto; padding: 28px 22px 64px; }
.masthead {
  background: var(--card);
  border: 1px solid var(--line);
  border-top: 7px solid var(--accent);
  border-radius: 10px;
  padding: 28px 30px;
  box-shadow: 0 1px 0 rgba(0,0,0,.03);
}
.brand {
  font-size: 12px; letter-spacing: .22em; font-weight: 800; color: var(--accent);
  text-transform: uppercase;
}
.brand span { color: var(--muted); font-weight: 600; letter-spacing: .12em; }
h1 { margin: 10px 0 4px; font-size: clamp(25px, 3.2vw, 40px); line-height: 1.15; letter-spacing: -.02em; }
.subtitle { color: var(--muted); margin: 0 0 18px; }
.pills { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 14px; }
.pill {
  display: inline-block; padding: 5px 10px; border-radius: 999px; font-size: 11px;
  font-weight: 800; letter-spacing: .08em; text-transform: uppercase;
  border: 1px solid var(--line); background: #f2eee4; color: var(--muted);
}
.pill.ok { color: var(--ok); background: var(--ok-soft); border-color: #b9dcc5; }
.pill.warn { color: var(--warn); background: var(--warn-soft); border-color: #ead7a5; }
.pill.bad { color: var(--bad); background: var(--bad-soft); border-color: #e7bcb7; }
.pill.sealed { color: #fff; background: var(--accent); border-color: var(--accent); }
.notice {
  margin: 18px 0 0; padding: 12px 14px; border-radius: 8px;
  border: 1px solid #ead7a5; background: var(--warn-soft); color: #5e4206;
}
.notice strong { color: #4b3504; }
.grid-cards { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin: 22px 0 6px; }
.card {
  background: var(--card); border: 1px solid var(--line); border-radius: 9px; padding: 16px 17px;
}
.card .value { font-size: 26px; font-weight: 800; letter-spacing: -.02em; line-height: 1.1; }
.card .label { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: .1em; margin-top: 6px; }
section { margin-top: 30px; }
h2 {
  font-size: 12px; letter-spacing: .17em; text-transform: uppercase; color: var(--accent);
  border-bottom: 1px solid var(--line); padding-bottom: 8px; margin: 0 0 14px;
}
h3 { font-size: 15px; margin: 22px 0 8px; }
p { margin: 10px 0; }
.muted { color: var(--muted); }
.table-wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: 9px; background: var(--card); }
table { width: 100%; border-collapse: collapse; font-size: 13px; }
th, td { text-align: left; vertical-align: top; padding: 9px 11px; border-bottom: 1px solid #e8e2d6; }
th { background: #efe9dc; font-size: 10.5px; letter-spacing: .08em; text-transform: uppercase; color: #4d5b53; white-space: nowrap; }
tr:last-child td { border-bottom: 0; }
tbody tr:nth-child(even) td { background: #fbf8f1; }
tbody td:first-child { white-space: nowrap; }
code, .hash { font-family: var(--mono); font-size: .78rem; word-break: break-all; }
.hash { color: #2c4234; }
.tag {
  display: inline-block; font-size: 10.5px; font-weight: 700; letter-spacing: .03em;
  padding: 2px 7px; border-radius: 5px; background: #eee8da; color: #47534b; white-space: nowrap;
}
.tag.ok { background: var(--ok-soft); color: var(--ok); }
.tag.warn { background: var(--warn-soft); color: var(--warn); }
.tag.bad { background: var(--bad-soft); color: var(--bad); }
.duo { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.panel { border: 1px solid var(--line); border-radius: 9px; background: var(--card); padding: 16px 18px; }
.panel h3 { margin-top: 0; }
ul.clean { margin: 8px 0; padding-left: 18px; }
ul.clean li { margin: 5px 0; }
dl.kv { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 7px 16px; margin: 0; }
dl.kv dt { color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .06em; padding-top: 1px; }
dl.kv dd { margin: 0; word-break: break-word; }
details {
  border: 1px solid var(--line); border-radius: 9px; background: var(--card);
  padding: 0; margin: 10px 0;
}
details > summary {
  cursor: pointer; padding: 12px 15px; font-weight: 700; list-style: none;
  display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap;
}
details > summary::-webkit-details-marker { display: none; }
details[open] > summary { border-bottom: 1px solid var(--line); }
.detail-body { padding: 14px 16px 16px; }
.footer {
  margin-top: 34px; padding-top: 18px; border-top: 1px solid var(--line);
  color: var(--muted); font-size: 12px;
}
@media (max-width: 800px) {
  .grid-cards { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .duo { grid-template-columns: 1fr; }
  .wrap { padding: 16px 12px 40px; }
  .masthead { padding: 20px; }
}
@media print {
  body { background: #fff; font-size: 11px; }
  .wrap { max-width: none; padding: 0; }
  .masthead, .card, .panel, details, .table-wrap { box-shadow: none; break-inside: avoid; }
  details { break-inside: avoid; }
  details > summary { cursor: default; }
  details:not([open]) .detail-body { display: block; }
  a { color: inherit; text-decoration: none; }
  section { break-inside: auto; }
  tr { break-inside: avoid; }
}
</style>
</head>
<body>
<div class="wrap">
  <header class="masthead">
    <div class="brand">Stratum <span>· case report</span></div>
    <h1>{{.Case.Name}}</h1>
    <p class="subtitle">Case <code>{{.Case.CaseID}}</code> · generated {{datetime .GeneratedAt}} by {{.GeneratedBy.Display}} · {{.Tool}} {{.Version}}</p>
    <div class="pills">
      <span class="pill {{if .Sealed}}sealed{{else}}warn{{end}}">{{if .Sealed}}Sealed{{else}}Open{{end}}</span>
      <span class="pill {{if .Summary.ChainOK}}ok{{else}}bad{{end}}">{{if .Summary.ChainOK}}Chains verified{{else}}Chain failure{{end}}</span>
      <span class="pill {{if .Summary.VerificationOK}}ok{{else}}bad{{end}}">{{if .Summary.VerificationOK}}Integrity checks passed{{else}}Integrity checks failed{{end}}</span>
      <span class="pill {{if .Summary.VerificationComplete}}ok{{else}}warn{{end}}">{{if .Summary.VerificationComplete}}Fully re-hashed{{else}}Partial verification{{end}}</span>
    </div>
    {{if .Summary.GPSCount}}<div class="notice"><strong>Privacy:</strong> {{.Summary.GPSCount}} item(s) contain GPS coordinates. This report contains location metadata; redact it before public release.</div>{{end}}
    {{if .Case.Description}}<p class="muted" style="margin-top:16px">{{oneLine .Case.Description}}</p>{{end}}
  </header>

  <div class="grid-cards">
    <div class="card"><div class="value">{{.Summary.ItemCount}}</div><div class="label">Evidence items</div></div>
    <div class="card"><div class="value">{{.Summary.TotalBytesHuman}}</div><div class="label">Total size</div></div>
    <div class="card"><div class="value">{{.Summary.EXIFCount}}</div><div class="label">EXIF-bearing</div></div>
    <div class="card"><div class="value">{{len .CustodyEvents}}</div><div class="label">Custody records</div></div>
  </div>

  <section id="case">
    <h2>Case and examiner</h2>
    <div class="duo">
      <div class="panel">
        <h3>Case</h3>
        <dl class="kv">
          <dt>Case ID</dt><dd><code>{{.Case.CaseID}}</code></dd>
          <dt>Name</dt><dd>{{.Case.Name}}</dd>
          <dt>Created</dt><dd>{{datetime .Case.Created}}</dd>
          <dt>Status</dt><dd>{{if .Sealed}}Sealed{{if .SealedAt}} at {{datetime .SealedAt}}{{end}}{{else}}Open{{end}}</dd>
          <dt>Description</dt><dd>{{if .Case.Description}}{{oneLine .Case.Description}}{{else}}—{{end}}</dd>
        </dl>
      </div>
      <div class="panel">
        <h3>Examiner and generation</h3>
        <dl class="kv">
          <dt>Examiner</dt><dd>{{.Case.Examiner.Display}}</dd>
          <dt>Generated by</dt><dd>{{.GeneratedBy.Display}}</dd>
          <dt>Generated</dt><dd>{{datetime .GeneratedAt}}</dd>
          <dt>Tool</dt><dd>{{.Tool}} {{.Version}}</dd>
          <dt>Items</dt><dd>{{.Summary.PreservedCount}} preserved, {{.Summary.ReferenceCount}} reference-only, {{.Summary.DuplicateCount}} duplicate(s)</dd>
          <dt>Metadata</dt><dd>{{.Summary.MetadataCount}} sidecar(s), {{.Summary.DocumentMetadataCount}} with document metadata</dd>
        </dl>
      </div>
    </div>
  </section>

  <section id="integrity">
    <h2>Integrity at a glance</h2>
    <div class="duo">
      <div class="panel">
        <h3>Hash chains</h3>
        <dl class="kv">
          <dt>Evidence chain</dt><dd><span class="tag {{if .EvidenceInfo.ChainOK}}ok{{else}}bad{{end}}">{{if .EvidenceInfo.ChainOK}}OK{{else}}BROKEN{{end}}</span> {{.EvidenceInfo.Records}} record(s)</dd>
          <dt>Evidence root</dt><dd><code class="hash">{{.EvidenceInfo.RootSHA256}}</code></dd>
          <dt>Evidence file</dt><dd><code class="hash">{{.EvidenceInfo.FileSHA256}}</code></dd>
          <dt>Custody chain</dt><dd><span class="tag {{if .CustodyInfo.ChainOK}}ok{{else}}bad{{end}}">{{if .CustodyInfo.ChainOK}}OK{{else}}BROKEN{{end}}</span> {{.CustodyInfo.Records}} record(s)</dd>
          <dt>Custody root</dt><dd><code class="hash">{{.CustodyInfo.RootSHA256}}</code></dd>
          <dt>Custody file</dt><dd><code class="hash">{{.CustodyInfo.FileSHA256}}</code></dd>
          {{if .Verification}}<dt>case.json</dt><dd><code class="hash">{{.Verification.CaseFileHash}}</code></dd>{{end}}
        </dl>
      </div>
      <div class="panel">
        <h3>Re-hash verification</h3>
        {{if .Verification}}
        <dl class="kv">
          <dt>Verdict</dt><dd><span class="tag {{if .Verification.OK}}ok{{else}}bad{{end}}">{{if .Verification.OK}}PASSED{{else}}FAILED{{end}}</span></dd>
          <dt>Completeness</dt><dd>{{if .Verification.Complete}}complete{{else}}partial{{end}}</dd>
          <dt>Chains</dt><dd>{{if .Verification.ChainsOK}}OK{{else}}FAILED{{end}}</dd>
          <dt>Cross-links</dt><dd>{{if .Verification.CrossLinksOK}}OK{{else}}FAILED{{end}}</dd>
          <dt>Bytes re-hashed</dt><dd>{{if .Verification.Rehashed}}yes{{else}}no{{end}}</dd>
          <dt>Items checked</dt><dd>{{.Verification.ItemsOK}} of {{.Verification.ItemCount}} verified</dd>
          <dt>Missing / modified</dt><dd>{{len .Verification.Missing}} / {{len .Verification.Modified}}</dd>
          <dt>Unexpected files</dt><dd>{{len .Verification.Unexpected}}</dd>
          <dt>Reference-only</dt><dd>{{len .Verification.Reference}}</dd>
          <dt>Metadata sidecars</dt><dd>{{.Verification.MetadataOK}} of {{.Verification.MetadataChecked}} verified<br><span class="muted">{{len .Verification.MetadataMissing}} missing, {{len .Verification.MetadataModified}} modified, {{len .Verification.MetadataUnexpected}} unexpected</span></dd>
          <dt>Checked</dt><dd>{{datetime .Verification.CheckedAt}}</dd>
        </dl>
        {{else}}<p class="muted">No verification result was supplied.</p>{{end}}
      </div>
    </div>
    {{if .Verification}}
      {{if .Verification.Errors}}<h3>Verification errors</h3><ul class="clean">{{range .Verification.Errors}}<li>{{.}}</li>{{end}}</ul>{{end}}
      {{if .Verification.Missing}}<h3>Missing items</h3><ul class="clean">{{range .Verification.Missing}}<li><code>{{.}}</code></li>{{end}}</ul>{{end}}
      {{if .Verification.Modified}}<h3>Modified items</h3><div class="table-wrap"><table><thead><tr><th>Item</th><th>Expected</th><th>Found</th><th>Note</th></tr></thead><tbody>{{range .Verification.Modified}}<tr><td>{{.ID}}</td><td><code class="hash">{{.ExpectedSHA256}}</code></td><td><code class="hash">{{.ActualSHA256}}</code></td><td>{{oneLine .Note}}</td></tr>{{end}}</tbody></table></div>{{end}}
      {{if .Verification.Unexpected}}<h3>Unexpected files in items/</h3><ul class="clean">{{range .Verification.Unexpected}}<li><code>{{.}}</code></li>{{end}}</ul>{{end}}
      {{if .Verification.Reference}}<h3>Reference-only items</h3><p class="muted">These items were hashed but not copied into the workspace, so their bytes cannot be re-verified from preserved evidence.</p><ul class="clean">{{range .Verification.Reference}}<li><code>{{.ID}}</code> — {{oneLine .Note}}</li>{{end}}</ul>{{end}}
      {{if .Verification.MetadataMissing}}<h3>Metadata sidecars missing</h3><p class="muted">These items have no verified metadata sidecar. For older cases, run <code>stratum extract --all</code>.</p><ul class="clean">{{range .Verification.MetadataMissing}}<li><code>{{.}}</code></li>{{end}}</ul>{{end}}
      {{if .Verification.MetadataModified}}<h3>Metadata sidecars modified</h3><div class="table-wrap"><table><thead><tr><th>Item</th><th>Expected SHA-256</th><th>Note</th></tr></thead><tbody>{{range .Verification.MetadataModified}}<tr><td>{{.ID}}</td><td><code class="hash">{{.ExpectedSHA256}}</code></td><td>{{oneLine .Note}}</td></tr>{{end}}</tbody></table></div>{{end}}
      {{if .Verification.MetadataUnexpected}}<h3>Unexpected files in metadata/</h3><ul class="clean">{{range .Verification.MetadataUnexpected}}<li><code>{{.}}</code></li>{{end}}</ul>{{end}}
    {{end}}
  </section>

  <section id="evidence">
    <h2>Chain of evidence — what and when</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th>#</th><th>UTC time</th><th>Event</th><th>Subject</th><th>Detail</th><th>Record SHA-256</th></tr></thead>
        <tbody>
        {{range .EvidenceEvents}}
          <tr>
            <td>{{.Seq}}</td>
            <td>{{datetime .Time}}</td>
            <td><strong>{{.Title}}</strong></td>
            <td>{{oneLine .Subject}}</td>
            <td>{{oneLine .Detail}}</td>
            <td><code class="hash">{{.Hash}}</code>{{if .ObjectSHA256}}<br><span class="muted">object</span> <code class="hash">{{.ObjectSHA256}}</code>{{end}}</td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
  </section>

  <section id="custody">
    <h2>Chain of custody — who</h2>
    {{if .CustodyEvents}}
    <div class="table-wrap">
      <table>
        <thead><tr><th>#</th><th>UTC time</th><th>Action</th><th>From → to</th><th>Actor</th><th>Method / purpose</th><th>Items</th><th>Anchor</th></tr></thead>
        <tbody>
        {{range .CustodyEvents}}
          <tr>
            <td>{{.Seq}}</td>
            <td>{{datetime .Time}}</td>
            <td><strong>{{.Title}}</strong></td>
            <td>{{.From}} → {{.To}}</td>
            <td>{{.Actor}}</td>
            <td>{{oneLine .Method}}{{if .Purpose}}<br><span class="muted">{{oneLine .Purpose}}</span>{{end}}{{if .Location}}<br><span class="muted">{{oneLine .Location}}</span>{{end}}{{if .Note}}<br><span class="muted">{{oneLine .Note}}</span>{{end}}</td>
            <td>{{.ItemsLabel}} <span class="muted">({{.ItemCount}})</span></td>
            <td><code class="hash">{{.EvidenceRoot}}</code><br><span class="muted">record</span> <code class="hash">{{.Hash}}</code></td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
    {{else}}<p class="muted">No custody records.</p>{{end}}
    {{if .Summary.Holders}}<h3>Current custodians</h3><ul class="clean">{{range .Summary.Holders}}<li><strong>{{.Holder}}</strong> — {{.Count}} item(s)</li>{{end}}</ul>{{end}}
  </section>

  <section id="inventory">
    <h2>Evidence inventory</h2>
    {{if .Items}}
    <div class="table-wrap">
      <table>
        <thead><tr><th>ID</th><th>File</th><th>Type</th><th>Size</th><th>Mode</th><th>SHA-256</th><th>Holder</th></tr></thead>
        <tbody>
        {{range .Items}}
          <tr>
            <td><strong>{{.ID}}</strong></td>
            <td>{{.Name}}{{if .Duplicate}}<br><span class="tag warn">duplicate of {{.DuplicateOf}}</span>{{end}}</td>
            <td>{{.FileType.Description}}<br><span class="muted"><code>{{.FileType.MIME}}</code></span>{{if not .FileType.ExtensionMatch}}<br><span class="tag bad">extension mismatch</span>{{end}}</td>
            <td>{{.SizeHuman}}</td>
            <td>{{.ModeLabel}}</td>
            <td><code class="hash">{{.SHA256}}</code></td>
            <td>{{.CurrentHolder}}</td>
          </tr>
        {{end}}
        </tbody>
      </table>
    </div>
    <p class="muted">Expand an item for source paths, EXIF details and notes.</p>
    {{range .Items}}
    <details>
      <summary><span class="tag">{{.ID}}</span> {{.Name}} <span class="muted">{{.TypeLabel}} · {{.SizeHuman}}</span></summary>
      <div class="detail-body">
        <dl class="kv">
          <dt>Source</dt><dd><code>{{.SourcePath}}</code></dd>
          <dt>Preserved</dt><dd>{{if .StoredPath}}<code>{{.StoredPath}}</code>{{else}}not preserved (reference only){{end}}</dd>
          <dt>Acquired</dt><dd>{{datetime .AcquiredAt}} by {{.AcquiredBy.Display}}</dd>
          <dt>Mode</dt><dd>{{.ModeLabel}}{{if .CopyVerified}} <span class="tag ok">read-back verified</span>{{end}}</dd>
          <dt>SHA-256</dt><dd><code class="hash">{{.SHA256}}</code></dd>
          <dt>Metadata sidecar</dt><dd>{{if .Metadata.Path}}<code>{{.Metadata.Path}}</code>{{else}}—{{end}}</dd>
          <dt>Metadata SHA-256</dt><dd>{{if .Metadata.SHA256}}<code class="hash">{{.Metadata.SHA256}}</code> <span class="muted">({{.Metadata.Size}} bytes)</span>{{else}}not recorded{{end}}</dd>
          <dt>Source stable</dt><dd>{{if .SourceStable}}yes{{else}}<span class="tag warn">changed during acquisition</span>{{end}}</dd>
          <dt>Confidence</dt><dd>{{.FileType.Confidence}}</dd>
          {{if .Origin}}<dt>Origin</dt><dd>{{oneLine .Origin}}</dd>{{end}}
          {{if .Note}}<dt>Note</dt><dd>{{oneLine .Note}}</dd>{{end}}
          {{if .DuplicateOf}}<dt>Duplicate</dt><dd>same bytes as {{.DuplicateOf}}</dd>{{end}}
          {{if .EXIF.Present}}<dt>EXIF</dt><dd>{{.EXIFSummary}}</dd>{{end}}
          {{if .GPS}}<dt>GPS</dt><dd>{{.GPS}}</dd>{{end}}
          {{if .Document.Present}}<dt>Document</dt><dd>{{.Document.Format}}{{if .Document.Title}} · {{.Document.Title}}{{end}}{{if .Document.Author}} · {{.Document.Author}}{{end}}{{if .Document.Created}} · {{.Document.Created}}{{end}}{{if .Document.Application}} · {{.Document.Application}}{{end}}</dd>{{end}}
        </dl>
        {{if .FileType.Notes}}<h3>Type notes</h3><ul class="clean">{{range .FileType.Notes}}<li>{{.}}</li>{{end}}</ul>{{end}}
        {{if .EXIF.Error}}<p class="muted">EXIF parser note: {{oneLine .EXIF.Error}}</p>{{end}}
        {{if .EXIF.Capable}}<p class="muted">{{oneLine .EXIF.Note}}</p>{{end}}
      </div>
    </details>
    {{end}}
    {{else}}<p class="muted">No evidence items have been acquired.</p>{{end}}
  </section>

  {{if .Summary.DocumentMetadataCount}}
  <section id="document-metadata">
    <h2>Document metadata</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th>ID</th><th>Format</th><th>Title</th><th>Author</th><th>Created</th><th>Application</th><th>Sidecar SHA-256</th></tr></thead>
        <tbody>
        {{range .Items}}{{if .Document.Present}}
          <tr>
            <td>{{.ID}}</td>
            <td>{{.Document.Format}}</td>
            <td>{{.Document.Title}}</td>
            <td>{{.Document.Author}}</td>
            <td>{{.Document.Created}}{{if .Document.Modified}}<br><span class="muted">modified {{.Document.Modified}}</span>{{end}}</td>
            <td>{{.Document.Application}}{{if .Document.AppVersion}} {{.Document.AppVersion}}{{end}}</td>
            <td>{{if .Metadata.SHA256}}<code class="hash">{{.Metadata.SHA256}}</code>{{else}}—{{end}}</td>
          </tr>
        {{end}}{{end}}
        </tbody>
      </table>
    </div>
    <p class="muted">Document metadata is extracted from PDF Info strings, OOXML/ODF properties, HTML meta/title tags and RFC 822 email headers. Each sidecar is a JSON file under <code>metadata/</code>; its SHA-256 is recorded in the evidence chain.</p>
  </section>
  {{end}}

  {{if .Summary.EXIFCount}}
  <section id="exif">
    <h2>EXIF and location metadata</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th>ID</th><th>Make / model</th><th>Serial</th><th>Original time</th><th>GPS</th><th>XMP</th></tr></thead>
        <tbody>
        {{range .Items}}{{if .EXIF.Present}}
          <tr>
            <td>{{.ID}}</td>
            <td>{{.EXIF.Make}} {{.EXIF.Model}}</td>
            <td>{{.EXIF.Serial}}</td>
            <td>{{.EXIF.DateTimeOriginal}}{{if .EXIF.OffsetTime}} {{.EXIF.OffsetTime}}{{end}}</td>
            <td>{{if .GPS}}{{.GPS}}{{else}}—{{end}}</td>
            <td>{{if .EXIF.XMP}}present{{else}}not detected{{end}}</td>
          </tr>
        {{end}}{{end}}
        </tbody>
      </table>
    </div>
    <p class="muted">EXIF is parsed from JPEG, TIFF, PNG and WebP files. HEIF/AVIF containers are flagged as EXIF-capable but are not parsed by this build.</p>
  </section>
  {{end}}

  {{if .Issues}}
  <section id="issues">
    <h2>Acquisition issues</h2>
    <div class="table-wrap">
      <table>
        <thead><tr><th>UTC time</th><th>Kind</th><th>Path</th><th>Reason</th></tr></thead>
        <tbody>
        {{range .Issues}}
          <tr><td>{{datetime .Time}}</td><td><span class="tag {{if eq .Kind "error"}}bad{{else}}warn{{end}}">{{.Kind}}</span></td><td><code>{{.Path}}</code></td><td>{{oneLine .Reason}}</td></tr>
        {{end}}
        </tbody>
      </table>
    </div>
  </section>
  {{end}}

  {{if .Warnings}}
  <section id="warnings">
    <h2>Warnings</h2>
    <ul class="clean">{{range .Warnings}}<li>{{.}}</li>{{end}}</ul>
  </section>
  {{end}}

  <section id="method">
    <h2>Method and limitations</h2>
    <div class="panel">
      <p>Stratum copies source files into the workspace without writing to the source, hashes the exact bytes read, reads the preserved copy back, compares the SHA-256, preserves the source modification time, and makes the copy read-only. EXIF and document metadata are extracted into a JSON sidecar under <code>metadata/</code>; the sidecar's SHA-256 is recorded in the evidence chain. Timestamps are UTC. The evidence and custody ledgers are independent SHA-256 hash chains; every custody record anchors an evidence-chain root.</p>
      <p>A hash chain is <strong>tamper-evident, not tamper-proof</strong>: a person who can rewrite the whole file can recompute every hash. Anchor the roots above outside this workspace (signed manifest, email, or RFC 3161 timestamp) when that distinction matters.</p>
      <p>A file-level acquisition does not capture deleted data, unallocated space, alternate data streams, extended attributes or a full disk image. Reference-only items have no preserved copy; verification is partial. Reading a source can update its access time. Reports contain sensitive paths and may contain EXIF/GPS metadata.</p>
    </div>
  </section>

  <section id="attestation">
    <h2>Attestation</h2>
    <div class="panel">
      <p>This report was generated from the ledgers named above. Every file in this report directory is covered by <code>SHA256SUMS</code> and <code>MANIFEST.json</code>.</p>
      <dl class="kv">
        <dt>Evidence root</dt><dd><code class="hash">{{.EvidenceInfo.RootSHA256}}</code></dd>
        <dt>Custody root</dt><dd><code class="hash">{{.CustodyInfo.RootSHA256}}</code></dd>
        <dt>Report generated</dt><dd>{{datetime .GeneratedAt}}</dd>
        <dt>Generated by</dt><dd>{{.GeneratedBy.Display}}</dd>
      </dl>
      <p class="muted">Re-check with <code>stratum verify --workspace &lt;case-directory&gt;</code>.</p>
    </div>
  </section>

  <footer class="footer">
    Stratum {{.Version}} · {{datetime .GeneratedAt}} · case {{.Case.CaseID}} · this report may contain sensitive evidence metadata.
  </footer>
</div>
</body>
</html>
`
