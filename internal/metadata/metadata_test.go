package metadata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stratum/internal/metadata"
	"stratum/internal/model"
	"stratum/internal/workspace"
)

func TestWriteReadVerifyAndTamper(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir: filepath.Join(t.TempDir(), "case"), Name: "Metadata test",
		Examiner: model.Person{Name: "Jane Doe"}, Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := model.Item{
		ID: "E-0001", Name: "report.pdf", SHA256: strings.Repeat("a", 64), Size: 123,
		FileType: model.FileType{ID: "pdf"},
		EXIF:     model.EXIF{Present: true, Make: "Cam"},
		Document: model.Document{Present: true, Format: "pdf", Title: "Report", Author: "Jane"},
	}
	ref, err := metadata.Write(ws, item, item.EXIF, item.Document, "2026-11-02T12:00:00Z", false)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Path == "" || ref.SHA256 == "" || ref.Size == 0 {
		t.Fatalf("ref: %+v", ref)
	}
	item.Metadata = ref
	full, err := ws.ResolveStored(ref.Path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(full)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o444 {
		t.Fatalf("sidecar mode = %v", st.Mode())
	}
	doc, err := metadata.Read(ws, ref)
	if err != nil {
		t.Fatal(err)
	}
	if doc.ItemID != item.ID || doc.ItemSHA256 != item.SHA256 || doc.Document.Title != "Report" || doc.EXIF.Make != "Cam" {
		t.Fatalf("sidecar: %+v", doc)
	}
	if _, err := metadata.Verify(ws, item); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// The sidecar is bound to the item hash.
	other := item
	other.SHA256 = strings.Repeat("b", 64)
	if _, err := metadata.Verify(ws, other); err == nil {
		t.Fatal("sidecar verified for a different item hash")
	}

	// Tampering is detected.
	if err := os.Chmod(full, 0o644); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(full)
	data = []byte(strings.Replace(string(data), `"title": "Report"`, `"title": "Tampered"`, 1))
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := metadata.Verify(ws, item); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("tamper error = %v", err)
	}

	// Replacement is explicit.
	if _, err := metadata.Write(ws, item, item.EXIF, item.Document, "2026-11-02T12:00:01Z", false); err == nil {
		t.Fatal("write without replace succeeded over an existing sidecar")
	}
	if _, err := os.Stat(full); err != nil {
		t.Fatal("sidecar missing after refused replace")
	}
}

func TestSidecarJSONShape(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir: filepath.Join(t.TempDir(), "case"), Name: "Metadata test",
		Examiner: model.Person{Name: "Jane Doe"}, Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	item := model.Item{ID: "E-0002", Name: "notes.txt", SHA256: strings.Repeat("c", 64), Size: 9}
	ref, err := metadata.Write(ws, item, model.EXIF{}, model.Document{}, "2026-11-02T12:00:00Z", false)
	if err != nil {
		t.Fatal(err)
	}
	item.Metadata = ref
	full, _ := ws.ResolveStored(ref.Path)
	data, _ := os.ReadFile(full)
	var doc model.MetadataDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.FormatVersion != metadata.FormatVersion || doc.Tool != model.ToolName || doc.ItemSHA256 != item.SHA256 {
		t.Fatalf("sidecar: %+v", doc)
	}
	if !strings.HasSuffix(ref.Path, ".json") || !strings.HasPrefix(ref.Path, "metadata/") {
		t.Fatalf("path: %s", ref.Path)
	}
}
