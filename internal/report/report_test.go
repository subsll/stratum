package report_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subsll/stratum/internal/acquire"
	"github.com/subsll/stratum/internal/casestate"
	"github.com/subsll/stratum/internal/logx"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/report"
	"github.com/subsll/stratum/internal/verify"
	"github.com/subsll/stratum/internal/workspace"
)

func TestWriteAllFormats(t *testing.T) {
	ws, st, vrep := setupReportCase(t, true)
	beforeEvidence := fileHash(t, ws.EvidencePath)
	beforeCustody := fileHash(t, ws.CustodyPath)

	out, err := report.Write(ws, st, vrep, report.Options{Formats: report.DefaultFormats})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.txt", "report.md", "report.html", "report.json", "items.csv", "metadata.csv", "evidence.csv", "custody.csv", "MANIFEST.json", "SHA256SUMS"} {
		if _, err := os.Stat(filepath.Join(out.Dir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if got := fileHash(t, ws.EvidencePath); got != beforeEvidence {
		t.Fatal("report generation modified evidence.jsonl")
	}
	if got := fileHash(t, ws.CustodyPath); got != beforeCustody {
		t.Fatal("report generation modified custody.jsonl")
	}
	checkSums(t, out.Dir)

	text := readFile(t, filepath.Join(out.Dir, "report.txt"))
	for _, want := range []string{"STRATUM CASE REPORT", "CHAIN OF EVIDENCE", "CHAIN OF CUSTODY", "EVIDENCE INVENTORY", "E-0001"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text report missing %q", want)
		}
	}
	md := readFile(t, filepath.Join(out.Dir, "report.md"))
	for _, want := range []string{"## Chain of evidence", "## Chain of custody", "## Evidence inventory", "<code>"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown report missing %q", want)
		}
	}
	html := readFile(t, filepath.Join(out.Dir, "report.html"))
	if !strings.HasPrefix(html, "<!doctype html>") || !strings.Contains(html, "Stratum case report") {
		t.Fatal("html report is not a standalone document")
	}
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("html report did not escape the case name")
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("expected escaped case name in html")
	}
	if strings.Contains(md, "<script>alert(1)</script>") {
		t.Fatal("markdown report did not escape the case name")
	}
	if !strings.Contains(md, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatal("expected escaped case name in markdown")
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(out.Dir, "report.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["case"].(map[string]any)["case_id"] != ws.Case.CaseID {
		t.Fatal("json case id mismatch")
	}
	if len(doc["items"].([]any)) != 3 {
		t.Fatalf("json items: %d", len(doc["items"].([]any)))
	}
	if len(doc["evidence_records"].([]any)) == 0 || len(doc["custody_records"].([]any)) == 0 {
		t.Fatal("json report does not include raw records")
	}
	if doc["verification"].(map[string]any)["ok"] != true {
		t.Fatal("json verification is not ok")
	}
	summary := doc["summary"].(map[string]any)
	if summary["metadata_count"].(float64) != 3 || summary["document_metadata_count"].(float64) != 0 {
		t.Fatalf("summary metadata counts: %+v", summary)
	}

	manifestBytes := readFile(t, filepath.Join(out.Dir, "MANIFEST.json"))
	var manifest report.Manifest
	if err := json.Unmarshal([]byte(manifestBytes), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Evidence.RootSHA256 != vrep.Evidence.RootSHA256 || manifest.Custody.RootSHA256 != vrep.Custody.RootSHA256 {
		t.Fatal("manifest roots do not match verification roots")
	}
	if manifest.CaseFileSHA256 == "" || len(manifest.Reports) < 5 {
		t.Fatalf("manifest: %+v", manifest)
	}
	if manifest.Verification.OK != true || !manifest.Verification.Complete {
		t.Fatalf("manifest verification: %+v", manifest.Verification)
	}
	if manifest.Verification.MetadataChecked != 3 || manifest.Verification.MetadataOK != 3 {
		t.Fatalf("manifest metadata verification: %+v", manifest.Verification)
	}

	itemsCSV := readFile(t, filepath.Join(out.Dir, "items.csv"))
	if strings.Count(itemsCSV, "\n") != 4 {
		t.Fatalf("items.csv rows: %q", itemsCSV)
	}
}

func TestSingleFormatAndEscaping(t *testing.T) {
	ws, st, vrep := setupReportCase(t, true)
	out, err := report.Write(ws, st, vrep, report.Options{Formats: []string{"html"}, OutDir: filepath.Join(t.TempDir(), "delivery")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out.Dir, "report.html")); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"report.txt", "report.md", "report.json", "items.csv"} {
		if _, err := os.Stat(filepath.Join(out.Dir, absent)); err == nil {
			t.Fatalf("%s should not exist for an html-only report", absent)
		}
	}
	checkSums(t, out.Dir)
}

func TestEmptyCaseReport(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir: filepath.Join(t.TempDir(), "case"), Name: "Empty case",
		Examiner: model.Person{Name: "Jane Doe"}, Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	st, err := casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	vrep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	out, err := report.Write(ws, st, vrep, report.Options{Formats: report.DefaultFormats})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.txt", "report.md", "report.html", "report.json"} {
		body := readFile(t, filepath.Join(out.Dir, name))
		if strings.Contains(body, "<no value>") || strings.Contains(body, "template:") {
			t.Fatalf("%s contains a template error", name)
		}
	}
	if !strings.Contains(readFile(t, filepath.Join(out.Dir, "report.html")), "No evidence items have been acquired") {
		t.Fatal("empty HTML report lacks the empty-state message")
	}
}

func TestNormalizeFormats(t *testing.T) {
	got, err := report.NormalizeFormats([]string{"markdown,txt", "HTML", "json"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"md", "text", "html", "json"}
	if len(got) != len(want) {
		t.Fatalf("formats = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("formats = %v, want %v", got, want)
		}
	}
	if _, err := report.NormalizeFormats([]string{"pdf"}); err == nil {
		t.Fatal("unknown format accepted")
	}
}

func setupReportCase(t *testing.T, exif bool) (*workspace.Workspace, *casestate.State, *verify.Report) {
	t.Helper()
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir:      filepath.Join(t.TempDir(), "case"),
		Name:     "Acme <script>alert(1)</script>",
		Examiner: model.Person{Name: "Jane Doe", Org: "Lab", Email: "jane@example.com"},
		Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("alpha\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "b.html"), []byte("<!doctype html><html></html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if exif {
		if err := os.WriteFile(filepath.Join(src, "photo.jpg"), jpegWithMake(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := acquire.Run(ws, acquire.Request{
		Paths: []string{src}, Options: acquire.Options{Copy: true, Origin: "unit test"},
		Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false),
	}); err != nil {
		t.Fatal(err)
	}
	st, err := casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	vrep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if !vrep.OK {
		t.Fatalf("setup verification: %+v", vrep)
	}
	return ws, st, vrep
}

func jpegWithMake() []byte {
	// A minimal little-endian TIFF: IFD0 with one ASCII Make tag.
	tiff := []byte{'I', 'I', '*', 0, 8, 0, 0, 0}
	tiff = append(tiff, 1, 0) // one entry
	tiff = append(tiff, 0x0F, 0x01, 2, 0, 4, 0, 0, 0, 26, 0, 0, 0)
	tiff = append(tiff, 0, 0, 0, 0) // next IFD
	tiff = append(tiff, 'C', 'a', 'm', 0)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	jpeg = append(jpeg, payload...)
	jpeg = append(jpeg, 0xFF, 0xD9)
	return jpeg
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func checkSums(t *testing.T, dir string) {
	t.Helper()
	data := readFile(t, filepath.Join(dir, "SHA256SUMS"))
	if strings.TrimSpace(data) == "" {
		t.Fatal("empty SHA256SUMS")
	}
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("bad checksum line %q", line)
		}
		got := fileHash(t, filepath.Join(dir, fields[1]))
		if got != fields[0] {
			t.Fatalf("%s checksum mismatch", fields[1])
		}
	}
}

func TestReportRendersBrokenChain(t *testing.T) {
	ws, st, vrep := setupReportCase(t, true)
	data, err := os.ReadFile(ws.EvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"name":"a.txt"`, `"name":"evil.txt"`, 1))
	if err := os.WriteFile(ws.EvidencePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	st, err = casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	vrep, err = verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if vrep.OK {
		t.Fatal("tampered chain verified")
	}
	out, err := report.Write(ws, st, vrep, report.Options{Formats: report.DefaultFormats})
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, filepath.Join(out.Dir, "report.txt"))
	if !strings.Contains(text, "Verification errors:") || !strings.Contains(text, "FAILED") {
		t.Fatalf("text report hides verification failure:\n%s", text)
	}
	md := readFile(t, filepath.Join(out.Dir, "report.md"))
	if !strings.Contains(md, "Verification errors") || !strings.Contains(md, "FAILED") {
		t.Fatalf("markdown report hides verification failure:\n%s", md)
	}
	html := readFile(t, filepath.Join(out.Dir, "report.html"))
	if !strings.Contains(html, "Verification errors") || !strings.Contains(html, "FAILED") {
		t.Fatal("html report hides verification failure")
	}
}

func TestDocumentMetadataReport(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir: filepath.Join(t.TempDir(), "case"), Name: "Document metadata case",
		Examiner: model.Person{Name: "Jane Doe"}, Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "doc.docx")
	writeDocx(t, src)
	if _, err := acquire.Run(ws, acquire.Request{
		Paths: []string{src}, Options: acquire.Options{Copy: true},
		Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false),
	}); err != nil {
		t.Fatal(err)
	}
	st, err := casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	vrep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if !vrep.OK || vrep.MetadataChecked != 1 || vrep.MetadataOK != 1 {
		t.Fatalf("verification: %+v", vrep)
	}
	out, err := report.Write(ws, st, vrep, report.Options{Formats: report.DefaultFormats})
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, filepath.Join(out.Dir, "report.txt"))
	if !strings.Contains(text, "DOCUMENT METADATA") || !strings.Contains(text, "Case Notes") {
		t.Fatalf("text report missing document metadata:\n%s", text)
	}
	html := readFile(t, filepath.Join(out.Dir, "report.html"))
	if !strings.Contains(html, "Document metadata") || !strings.Contains(html, "Case Notes") {
		t.Fatal("html report missing document metadata")
	}
	metaCSV := readFile(t, filepath.Join(out.Dir, "metadata.csv"))
	if !strings.Contains(metaCSV, "Case Notes") || !strings.Contains(metaCSV, "metadata/E-0001__doc.docx.json") {
		t.Fatalf("metadata.csv missing values:\n%s", metaCSV)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(out.Dir, "report.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	item := doc["items"].([]any)[0].(map[string]any)
	if item["document"].(map[string]any)["title"] != "Case Notes" {
		t.Fatalf("json item document: %+v", item["document"])
	}
	if item["metadata"].(map[string]any)["sha256"] == "" {
		t.Fatal("json item metadata hash missing")
	}
}

func writeDocx(t *testing.T, path string) {
	t.Helper()
	core := `<?xml version="1.0"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>Case Notes</dc:title><dc:creator>Jane Doe</dc:creator><dc:subject>Review</dc:subject><cp:lastModifiedBy>Bob</cp:lastModifiedBy><dcterms:created xsi:type="dcterms:W3CDTF">2024-01-02T03:04:05Z</dcterms:created></cp:coreProperties>`
	app := `<?xml version="1.0"?><Properties><Application>Word</Application><Company>Acme</Company><Pages>3</Pages><Words>1200</Words></Properties>`
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<w:document/>",
		"docProps/core.xml":   core,
		"docProps/app.xml":    app,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReportRendersMetadataFailure(t *testing.T) {
	ws, st, _ := setupReportCase(t, true)
	full, err := ws.ResolveStored(st.Items[0].Metadata.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(full); err != nil {
		t.Fatal(err)
	}
	st, err = casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	vrep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if vrep.OK || len(vrep.MetadataMissing) != 1 {
		t.Fatalf("verification: %+v", vrep)
	}
	out, err := report.Write(ws, st, vrep, report.Options{Formats: report.DefaultFormats})
	if err != nil {
		t.Fatal(err)
	}
	if text := readFile(t, filepath.Join(out.Dir, "report.txt")); !strings.Contains(text, "Metadata sidecars missing") {
		t.Fatalf("text metadata failure missing:\n%s", text)
	}
	if md := readFile(t, filepath.Join(out.Dir, "report.md")); !strings.Contains(md, "Metadata sidecars missing") {
		t.Fatalf("markdown metadata failure missing:\n%s", md)
	}
	if html := readFile(t, filepath.Join(out.Dir, "report.html")); !strings.Contains(html, "Metadata sidecars missing") {
		t.Fatal("html metadata failure missing")
	}
}
