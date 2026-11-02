package filekind

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFormats(t *testing.T) {
	docx := zipBytes(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   "<w:document/>",
	})
	xlsx := zipBytes(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"xl/workbook.xml":     "<workbook/>",
	})
	odt := zipBytes(t, map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"content.xml": "<office/>",
	})
	epub := zipBytes(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": "<container/>",
	})

	cases := []struct {
		name     string
		ext      string
		data     []byte
		id       string
		category string
		match    bool
	}{
		{"pdf", "pdf", []byte("%PDF-1.7\n1 0 obj\n<<>>\nendobj\n"), "pdf", "document", true},
		{"jpeg", "jpg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, "jpeg", "image", true},
		{"png", "png", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), "png", "image", true},
		{"gif", "gif", []byte("GIF89a...."), "gif", "image", true},
		{"webp", "webp", append([]byte("RIFF\x00\x00\x00\x00WEBP"), make([]byte, 8)...), "webp", "image", true},
		{"tiff", "tif", []byte{'I', 'I', '*', 0, 8, 0, 0, 0, 0, 0}, "tiff", "image", true},
		{"html", "html", []byte("<!doctype html><html><body>hi</body></html>"), "html", "web", true},
		{"html-bom", "html", append([]byte{0xEF, 0xBB, 0xBF}, []byte("<!doctype html><html></html>")...), "html", "web", true},
		{"json", "json", []byte(`{"a":1,"b":[true,null]}`), "json", "text", true},
		{"csv", "csv", []byte("a,b,c\n1,2,3\n"), "csv", "text", true},
		{"text", "txt", []byte("hello world\n"), "text", "text", true},
		{"empty", "bin", nil, "empty", "other", true},
		{"sqlite", "sqlite", append([]byte("SQLite format 3\x00"), make([]byte, 32)...), "sqlite", "database", true},
		{"gzip", "gz", []byte{0x1F, 0x8B, 8, 0, 0, 0, 0, 0, 0, 0}, "gzip", "archive", true},
		{"7z", "7z", append([]byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}, make([]byte, 8)...), "7z", "archive", true},
		{"elf", "bin", []byte{0x7F, 'E', 'L', 'F', 2, 1, 1}, "elf", "executable", true},
		{"mp4", "mp4", append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, make([]byte, 16)...), "mp4", "video", true},
		{"heic", "heic", append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, make([]byte, 16)...), "heic", "image", true},
		{"mhtml", "mhtml", []byte("MIME-Version: 1.0\r\nContent-Type: multipart/related; boundary=x\r\nContent-Location: https://example.test/\r\n"), "mhtml", "web", true},
		{"eml", "eml", []byte("From: a@example.test\r\nMIME-Version: 1.0\r\nSubject: hi\r\n\r\nbody\r\n"), "eml", "email", true},
		{"warc", "warc", []byte("WARC/1.0\r\nWARC-Type: response\r\n\r\n"), "warc", "web", true},
		{"har", "har", []byte(`{"log":{"version":"1.2","entries":[]}}`), "har", "web", true},
		{"docx", "docx", docx, "docx", "document", true},
		{"xlsx", "xlsx", xlsx, "xlsx", "document", true},
		{"odt", "odt", odt, "odt", "document", true},
		{"epub", "epub", epub, "epub", "document", true},
		{"zip", "zip", zipBytes(t, map[string]string{"a.txt": "hello"}), "zip", "archive", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.name+"."+tc.ext)
			if err := os.WriteFile(path, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			ft, err := Detect(path, filepath.Base(path))
			if err != nil {
				t.Fatal(err)
			}
			if ft.ID != tc.id {
				t.Fatalf("id = %q, want %q (%+v)", ft.ID, tc.id, ft)
			}
			if tc.category != "" && ft.Category != tc.category {
				t.Fatalf("category = %q, want %q", ft.Category, tc.category)
			}
			if ft.ExtensionMatch != tc.match {
				t.Fatalf("extension match = %t, want %t (%+v)", ft.ExtensionMatch, tc.match, ft)
			}
			if ft.Confidence == "" || ft.MIME == "" {
				t.Fatalf("missing confidence or MIME: %+v", ft)
			}
		})
	}
}

func TestExtensionMismatchAndUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	pdf := []byte("%PDF-1.7\n")
	path := filepath.Join(dir, "disguised.jpg")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	ft, err := Detect(path, filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	if ft.ID != "pdf" || ft.ExtensionMatch {
		t.Fatalf("expected a PDF/extension mismatch, got %+v", ft)
	}

	path = filepath.Join(dir, "evidence.dat")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	ft, err = Detect(path, filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	if ft.ID != "pdf" || !ft.ExtensionMatch {
		t.Fatalf("unknown extensions should not be treated as a mismatch: %+v", ft)
	}
}

func TestShortAndRandomFilesDoNotPanic(t *testing.T) {
	dir := t.TempDir()
	for i, data := range [][]byte{
		{0xFF},
		{0xFF, 0xD8},
		{'P', 'K'},
		{'R', 'I', 'F', 'F'},
		bytes.Repeat([]byte{0x00}, 3),
		bytes.Repeat([]byte{0xAA}, 100),
	} {
		path := filepath.Join(dir, "f"+string(rune('a'+i)))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Detect(path, "f.bin"); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Detect(filepath.Join(t.TempDir(), "nope"), "nope"); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
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
	return buf.Bytes()
}
