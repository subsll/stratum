package docmeta

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPDFMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	pdf := "%PDF-1.7\n1 0 obj\n<< /Title (Quarterly Report) /Author (Jane Doe) /Subject (Review) /Keywords (a, b) /Creator (Writer) /Producer (PDFTool) /CreationDate (D:20240102030405Z) /ModDate (D:20240103040506+02'00') >>\nendobj\ntrailer<</Root 1 0 R>>"
	if err := os.WriteFile(path, []byte(pdf), 0o600); err != nil {
		t.Fatal(err)
	}
	d := Extract(path, "pdf")
	if !d.Present || d.Format != "pdf" {
		t.Fatalf("pdf: %+v", d)
	}
	if d.Title != "Quarterly Report" || d.Author != "Jane Doe" || d.Subject != "Review" {
		t.Fatalf("fields: %+v", d)
	}
	if d.Keywords != "a, b" || d.Creator != "Writer" || d.Producer != "PDFTool" {
		t.Fatalf("fields: %+v", d)
	}
	if d.Created != "D:20240102030405Z" || d.Modified != "D:20240103040506+02'00'" {
		t.Fatalf("dates: %q %q", d.Created, d.Modified)
	}
}

func TestPDFHexAndXMP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hex.pdf")
	pdf := "%PDF-1.7\n<< /Title <FEFF00480065006C006C006F> >>\n<?xpacket begin?><x:xmpmeta></x:xmpmeta>"
	if err := os.WriteFile(path, []byte(pdf), 0o600); err != nil {
		t.Fatal(err)
	}
	d := Extract(path, "pdf")
	if d.Title != "Hello" {
		t.Fatalf("title = %q", d.Title)
	}
	if !d.XMP {
		t.Fatal("XMP not detected")
	}
}

func TestOOXMLMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.docx")
	core := `<?xml version="1.0"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<dc:title>Case Notes</dc:title><dc:creator>Jane Doe</dc:creator><dc:subject>Review</dc:subject>
<dc:description>Summary</dc:description><cp:keywords>alpha, beta</cp:keywords>
<cp:lastModifiedBy>Bob</cp:lastModifiedBy><cp:revision>7</cp:revision>
<dcterms:created xsi:type="dcterms:W3CDTF">2024-01-02T03:04:05Z</dcterms:created>
<dcterms:modified xsi:type="dcterms:W3CDTF">2024-01-03T04:05:06Z</dcterms:modified>
<cp:category>Legal</cp:category></cp:coreProperties>`
	app := `<?xml version="1.0"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Word</Application><Company>Acme</Company><AppVersion>16.0</AppVersion><Pages>3</Pages><Words>1200</Words><Characters>7000</Characters></Properties>`
	writeZip(t, path, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<w:document/>",
		"docProps/core.xml":   core,
		"docProps/app.xml":    app,
	})
	d := Extract(path, "docx")
	if !d.Present || d.Format != "ooxml" {
		t.Fatalf("ooxml: %+v", d)
	}
	if d.Title != "Case Notes" || d.Author != "Jane Doe" || d.LastModifiedBy != "Bob" {
		t.Fatalf("core: %+v", d)
	}
	if d.Application != "Word" || d.Company != "Acme" || d.AppVersion != "16.0" || d.Pages != 3 || d.Words != 1200 {
		t.Fatalf("app: %+v", d)
	}
	if d.Created != "2024-01-02T03:04:05Z" || d.Modified != "2024-01-03T04:05:06Z" {
		t.Fatalf("dates: %q %q", d.Created, d.Modified)
	}
	if d.Fields["revision"] != "7" || d.Fields["category"] != "Legal" || d.Fields["characters"] != "7000" {
		t.Fatalf("extra fields: %+v", d.Fields)
	}
}

func TestODFMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.odt")
	meta := `<?xml version="1.0"?>
<office:document-meta xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:meta="urn:oasis:names:tc:opendocument:xmlns:meta:1.0">
<office:meta><dc:title>Minutes</dc:title><dc:creator>Jane</dc:creator>
<meta:generator>LibreOffice</meta:generator><meta:initial-creator>Jane</meta:initial-creator>
<meta:creation-date>2024-01-02T03:04:05</meta:creation-date><dc:date>2024-01-03T04:05:06</dc:date>
<meta:document-statistic meta:page-count="2" meta:word-count="42" meta:character-count="200"/>
</office:meta></office:document-meta>`
	writeZip(t, path, map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"meta.xml":    meta,
		"content.xml": "<office/>",
	})
	d := Extract(path, "odt")
	if !d.Present || d.Format != "odf" {
		t.Fatalf("odf: %+v", d)
	}
	if d.Title != "Minutes" || d.Author != "Jane" || d.Application != "LibreOffice" {
		t.Fatalf("fields: %+v", d)
	}
	if d.Pages != 2 || d.Words != 42 {
		t.Fatalf("stats: pages=%d words=%d", d.Pages, d.Words)
	}
	if d.Fields["initial_creator"] != "Jane" || d.Fields["statistic.character-count"] != "200" {
		t.Fatalf("fields: %+v", d.Fields)
	}
}

func TestHTMLMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	body := `<!doctype html><html><head><title>Acme &amp; Sons — Pricing</title>
<meta name="author" content="Jane Doe"><meta name="description" content="Price list">
<meta name="keywords" content="pricing, acme"><meta name="generator" content="CMS 2">
<meta property="og:site_name" content="Acme"><meta property="article:published_time" content="2024-01-02T03:04:05Z">
</head><body>hello</body></html>`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d := Extract(path, "html")
	if !d.Present || d.Format != "html" {
		t.Fatalf("html: %+v", d)
	}
	if d.Title != "Acme & Sons — Pricing" || d.Author != "Jane Doe" || d.Description != "Price list" {
		t.Fatalf("fields: %+v", d)
	}
	if d.Keywords != "pricing, acme" || d.Application != "CMS 2" || d.Created != "2024-01-02T03:04:05Z" {
		t.Fatalf("fields: %+v", d)
	}
	if d.Fields["og:site_name"] != "Acme" {
		t.Fatalf("fields: %+v", d.Fields)
	}
}

func TestEMLMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mail.eml")
	body := "From: Jane Doe <jane@example.test>\r\nTo: Bob <bob@example.test>\r\nSubject: Delivery\r\nDate: Tue, 02 Jan 2024 03:04:05 +0000\r\nMessage-ID: <abc@example.test>\r\nMIME-Version: 1.0\r\n\r\nbody\r\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d := Extract(path, "eml")
	if !d.Present || d.Title != "Delivery" || d.Author == "" || d.Created == "" {
		t.Fatalf("eml: %+v", d)
	}
	if d.Fields["to"] != "Bob <bob@example.test>" || d.Fields["message_id"] != "<abc@example.test>" {
		t.Fatalf("fields: %+v", d.Fields)
	}
}

func TestUnsupportedAndMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if d := Extract(path, "text"); d.Present || d.Error != "" {
		t.Fatalf("text: %+v", d)
	}
	// A broken ZIP should report an error but must not panic.
	if d := Extract(path, "docx"); d.Error == "" {
		t.Fatalf("expected zip error: %+v", d)
	}
	// Malformed PDF still parses without panic.
	bad := filepath.Join(t.TempDir(), "bad.pdf")
	if err := os.WriteFile(bad, []byte("%PDF-1.7\n/Title (unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := Extract(bad, "pdf")
	if d.Error != "" {
		t.Fatalf("unexpected pdf error: %s", d.Error)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// ODF requires mimetype first; keep deterministic order for that one.
	names := make([]string, 0, len(files))
	if _, ok := files["mimetype"]; ok {
		names = append(names, "mimetype")
	}
	for name := range files {
		if name != "mimetype" {
			names = append(names, name)
		}
	}
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(files[name])); err != nil {
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
