// Package metadata builds, writes and verifies per-item metadata sidecars. A
// sidecar contains the item's EXIF and document metadata and is bound to the
// item's SHA-256. Its own SHA-256 is stored in the evidence chain.
package metadata

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/subsll/stratum/internal/docmeta"
	"github.com/subsll/stratum/internal/exif"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

// FormatVersion is the metadata sidecar format version.
const FormatVersion = 1

// PathFor returns the workspace-relative sidecar path for an item.
func PathFor(itemID, name string) string {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if strings.TrimSpace(base) == "" || base == "." || base == ".." {
		base = "file"
	}
	var b strings.Builder
	for _, r := range base {
		if r < 0x20 || r == 0x7F || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	clean := strings.TrimSpace(b.String())
	if clean == "" || clean == "." || clean == ".." {
		clean = "file"
	}
	runes := []rune(clean)
	if len(runes) > 80 {
		clean = string(runes[:80])
	}
	return filepath.ToSlash(filepath.Join(workspace.MetadataDir, itemID+"__"+clean+".json"))
}

// EnsureDir creates and validates metadata/ as a real directory.
func EnsureDir(ws *workspace.Workspace) error {
	dir := ws.MetadataPath
	if dir == "" {
		dir = filepath.Join(ws.Root, workspace.MetadataDir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is not a real directory", dir)
	}
	return os.Chmod(dir, 0o700)
}

// Write creates a metadata sidecar for item. replace permits replacing an
// existing regular sidecar (used by `stratum extract --force`).
func Write(ws *workspace.Workspace, item model.Item, exifData model.EXIF, doc model.Document, extractedAt string, replace bool) (model.MetadataRef, error) {
	if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.SHA256) == "" {
		return model.MetadataRef{}, fmt.Errorf("metadata: item id and SHA-256 are required")
	}
	if err := EnsureDir(ws); err != nil {
		return model.MetadataRef{}, err
	}
	if extractedAt == "" {
		extractedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	docFile := model.MetadataDocument{
		FormatVersion: FormatVersion,
		Tool:          model.ToolName,
		Version:       model.Version,
		ItemID:        item.ID,
		ItemName:      item.Name,
		ItemSHA256:    item.SHA256,
		ExtractedAt:   extractedAt,
		EXIF:          exifData,
		Document:      doc,
	}
	data, err := json.MarshalIndent(docFile, "", "  ")
	if err != nil {
		return model.MetadataRef{}, err
	}
	data = append(data, '\n')

	rel := PathFor(item.ID, item.Name)
	final, err := ws.ResolveStored(rel)
	if err != nil {
		return model.MetadataRef{}, err
	}
	exists := false
	if li, err := os.Lstat(final); err == nil {
		if !replace {
			return model.MetadataRef{}, fmt.Errorf("metadata sidecar already exists: %s", rel)
		}
		if li.Mode()&os.ModeSymlink != 0 || !li.Mode().IsRegular() {
			return model.MetadataRef{}, fmt.Errorf("refusing to replace non-regular metadata sidecar %s", rel)
		}
		exists = true
	} else if !os.IsNotExist(err) {
		return model.MetadataRef{}, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(final), ".incoming-meta-*")
	if err != nil {
		return model.MetadataRef{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return model.MetadataRef{}, err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return model.MetadataRef{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return model.MetadataRef{}, err
	}
	if err := tmp.Close(); err != nil {
		return model.MetadataRef{}, err
	}
	if err := os.Chmod(tmpName, 0o444); err != nil {
		return model.MetadataRef{}, err
	}
	// Only remove the old sidecar once the replacement is fully written.
	if exists {
		if err := os.Remove(final); err != nil {
			return model.MetadataRef{}, err
		}
	}
	if err := replaceFile(tmpName, final); err != nil {
		return model.MetadataRef{}, err
	}
	// Read back and verify the bytes before recording the hash.
	read, err := os.ReadFile(final)
	if err != nil {
		_ = os.Remove(final)
		return model.MetadataRef{}, err
	}
	sum := sha256.Sum256(read)
	want := sha256.Sum256(data)
	if len(read) != len(data) || sum != want {
		_ = os.Remove(final)
		return model.MetadataRef{}, fmt.Errorf("metadata sidecar read-back did not verify")
	}
	var check model.MetadataDocument
	if err := json.Unmarshal(read, &check); err != nil {
		_ = os.Remove(final)
		return model.MetadataRef{}, fmt.Errorf("metadata sidecar is not valid JSON: %w", err)
	}
	if check.ItemID != item.ID || check.ItemSHA256 != item.SHA256 {
		_ = os.Remove(final)
		return model.MetadataRef{}, fmt.Errorf("metadata sidecar binding check failed")
	}
	return model.MetadataRef{
		Path:        rel,
		SHA256:      hex.EncodeToString(sum[:]),
		Size:        int64(len(read)),
		ExtractedAt: extractedAt,
	}, nil
}

// ExtractAndWrite runs EXIF and document extraction from sourcePath and writes
// the sidecar. The item's SHA256 must already be set.
func ExtractAndWrite(ws *workspace.Workspace, item model.Item, sourcePath, extractedAt string, replace bool) (model.MetadataRef, model.EXIF, model.Document, error) {
	exifData := exif.Extract(sourcePath)
	doc := docmeta.Extract(sourcePath, item.FileType.ID)
	ref, err := Write(ws, item, exifData, doc, extractedAt, replace)
	return ref, exifData, doc, err
}

// Read loads a sidecar referenced by ref without verifying its hash. Use Verify
// for integrity checks.
func Read(ws *workspace.Workspace, ref model.MetadataRef) (*model.MetadataDocument, error) {
	if strings.TrimSpace(ref.Path) == "" {
		return nil, fmt.Errorf("no metadata sidecar recorded")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref.Path)))
	if clean != workspace.MetadataDir+"/"+filepath.Base(clean) || !strings.HasPrefix(clean, workspace.MetadataDir+"/") {
		return nil, fmt.Errorf("metadata path %q is outside %s/", ref.Path, workspace.MetadataDir)
	}
	full, err := ws.ResolveStored(ref.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("metadata sidecar is not a regular file")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	var doc model.MetadataDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("metadata sidecar is not valid JSON: %w", err)
	}
	return &doc, nil
}

// Verify checks a sidecar's path, hash, size, JSON structure and binding to the
// item, and confirms the sidecar's EXIF/document data matches the item record.
func Verify(ws *workspace.Workspace, item model.Item) (*model.MetadataDocument, error) {
	ref := item.Metadata
	if strings.TrimSpace(ref.Path) == "" {
		return nil, fmt.Errorf("no metadata sidecar recorded")
	}
	if strings.TrimSpace(ref.SHA256) == "" {
		return nil, fmt.Errorf("metadata sidecar has no recorded SHA-256")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref.Path)))
	if !strings.HasPrefix(clean, workspace.MetadataDir+"/") {
		return nil, fmt.Errorf("metadata path %q is outside %s/", ref.Path, workspace.MetadataDir)
	}
	full, err := ws.ResolveStored(ref.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("metadata sidecar is not a regular file")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if int64(len(data)) != ref.Size || actual != ref.SHA256 {
		return nil, fmt.Errorf("metadata sidecar hash mismatch (expected %d bytes/%s, found %d bytes/%s)", ref.Size, ref.SHA256, len(data), actual)
	}
	var doc model.MetadataDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("metadata sidecar is not valid JSON: %w", err)
	}
	if doc.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("metadata sidecar format version %d, want %d", doc.FormatVersion, FormatVersion)
	}
	if doc.ItemID != item.ID || doc.ItemName != item.Name || doc.ItemSHA256 != item.SHA256 {
		return nil, fmt.Errorf("metadata sidecar is not bound to this item")
	}
	if !sameJSON(doc.EXIF, item.EXIF) || !sameJSON(doc.Document, item.Document) {
		return nil, fmt.Errorf("metadata sidecar content differs from the item record")
	}
	return &doc, nil
}

// sameJSON compares two values through their canonical JSON representation so
// nil and empty maps/slices are treated consistently after a sidecar round-trip.
func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		if rerr := os.Remove(path); rerr == nil {
			if rerr := os.Rename(tmp, path); rerr == nil {
				return nil
			}
		}
	}
	return os.Rename(tmp, path)
}
