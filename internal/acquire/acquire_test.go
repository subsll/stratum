package acquire

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subsll/stratum/internal/casestate"
	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/logx"
	"github.com/subsll/stratum/internal/metadata"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/verify"
	"github.com/subsll/stratum/internal/workspace"
)

func TestAcquirePreserveAndVerify(t *testing.T) {
	ws, src := newCaseAndSource(t)
	write(t, filepath.Join(src, "a.txt"), "alpha\n")
	write(t, filepath.Join(src, "b.txt"), "alpha\n") // duplicate content
	write(t, filepath.Join(src, "page.html"), "<!doctype html><html></html>")
	write(t, filepath.Join(src, "photo.jpg"), string([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 16, 'J', 'F', 'I', 'F', 0, 0, 0, 0, 0, 0, 0xFF, 0xD9}))
	write(t, filepath.Join(src, "nested", "brief.pdf"), "%PDF-1.7\n")
	write(t, filepath.Join(src, "nested", "skip.tmp"), "temporary")
	before := hashTree(t, src)

	res, err := Run(ws, Request{
		Paths:     []string{src},
		Options:   Options{Copy: true, Exclude: []string{"*.tmp"}, Origin: "unit test"},
		Operator:  ws.Case.Examiner,
		ForceLock: false,
		Log:       logx.New(io.Discard, true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 5 {
		t.Fatalf("items = %d, want 5: %+v", len(res.Items), res.Items)
	}
	if res.Duplicates != 1 || res.Errors != 0 {
		t.Fatalf("duplicates=%d errors=%d issues=%+v", res.Duplicates, res.Errors, res.Issues)
	}
	if len(res.Issues) != 0 {
		t.Fatalf("issues: %+v", res.Issues)
	}
	if res.EvidenceInfo.Records == 0 || res.CustodyInfo.Records != 2 || !res.CustodyRecorded {
		t.Fatalf("ledger info: %+v %+v", res.EvidenceInfo, res.CustodyInfo)
	}
	seen := map[string]bool{}
	for i, it := range res.Items {
		wantID := "E-000" + string(rune('1'+i))
		if it.ID != wantID {
			t.Fatalf("item %d id = %s, want %s", i, it.ID, wantID)
		}
		seen[it.ID] = true
		if it.Mode != "preserved" || !it.CopyVerified || it.StoredPath == "" {
			t.Fatalf("item not preserved: %+v", it)
		}
		full, err := ws.ResolveStored(it.StoredPath)
		if err != nil {
			t.Fatal(err)
		}
		st, err := os.Stat(full)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o444 {
			t.Fatalf("%s mode = %v, want 0444", it.ID, st.Mode())
		}
		h, size, err := chain.HashFile(full)
		if err != nil || h != it.SHA256 || size != it.Size {
			t.Fatalf("%s copy hash: %q %d %v", it.ID, h, size, err)
		}
		if it.Origin != "unit test" {
			t.Fatalf("origin = %q", it.Origin)
		}
		if strings.HasSuffix(it.Name, ".tmp") {
			t.Fatalf("excluded file acquired: %s", it.Name)
		}
		if it.Metadata.Path == "" || it.Metadata.SHA256 == "" || it.Metadata.Size == 0 {
			t.Fatalf("%s has no metadata sidecar: %+v", it.ID, it.Metadata)
		}
		sidecar, err := ws.ResolveStored(it.Metadata.Path)
		if err != nil {
			t.Fatal(err)
		}
		sinfo, err := os.Stat(sidecar)
		if err != nil {
			t.Fatal(err)
		}
		if sinfo.Mode().Perm() != 0o444 {
			t.Fatalf("%s sidecar mode = %v", it.ID, sinfo.Mode())
		}
		if _, err := metadata.Verify(ws, it); err != nil {
			t.Fatalf("%s metadata verify: %v", it.ID, err)
		}
		if it.Name == "b.txt" && it.DuplicateOf != "E-0001" {
			t.Fatalf("duplicate = %q", it.DuplicateOf)
		}
	}
	if !seen["E-0001"] || !seen["E-0005"] {
		t.Fatalf("ids: %v", seen)
	}
	if got := hashTree(t, src); got != before {
		t.Fatal("source tree changed during acquisition")
	}

	st, err := casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Items) != 5 || !st.ChainsOK() {
		t.Fatalf("state: items=%d chains=%t", len(st.Items), st.ChainsOK())
	}
	if st.Holder("E-0001") != ws.Case.Examiner.Display() {
		t.Fatalf("holder = %q", st.Holder("E-0001"))
	}
	vrep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if !vrep.OK || !vrep.Complete || vrep.ItemsOK != 5 || vrep.MetadataChecked != 5 || vrep.MetadataOK != 5 {
		t.Fatalf("verification: %+v", vrep)
	}

	// A second acquisition reuses the next id and detects duplicates against
	// already-recorded items.
	write(t, filepath.Join(src, "more.txt"), "alpha\n")
	res2, err := Run(ws, Request{Paths: []string{filepath.Join(src, "more.txt")}, Options: Options{Copy: true}, Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Items) != 1 || res2.Items[0].ID != "E-0006" || res2.Items[0].DuplicateOf == "" {
		t.Fatalf("second batch: %+v", res2.Items)
	}
}

func TestReferenceModeAndSymlink(t *testing.T) {
	ws, src := newCaseAndSource(t)
	write(t, filepath.Join(src, "ref.txt"), "reference bytes\n")
	res, err := Run(ws, Request{
		Paths:    []string{filepath.Join(src, "ref.txt")},
		Options:  Options{Copy: false, Note: "kept on source media"},
		Operator: ws.Case.Examiner,
		Log:      logx.New(io.Discard, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("items: %+v", res.Items)
	}
	it := res.Items[0]
	if it.Mode != "reference" || it.StoredPath != "" || it.CopyVerified {
		t.Fatalf("reference item: %+v", it)
	}
	if it.Metadata.Path == "" || it.Metadata.SHA256 == "" {
		t.Fatalf("reference item has no metadata sidecar: %+v", it.Metadata)
	}
	st, err := casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	vrep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if !vrep.OK || vrep.Complete || len(vrep.Reference) != 1 {
		t.Fatalf("reference verification: %+v", vrep)
	}

	// Symlinks are recorded as skips, never followed.
	target := filepath.Join(src, "ref.txt")
	link := filepath.Join(src, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	res, err = Run(ws, Request{Paths: []string{link}, Options: Options{Copy: true}, Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 || len(res.Issues) != 1 || res.Issues[0].Kind != "skipped" {
		t.Fatalf("symlink: items=%v issues=%+v", res.Items, res.Issues)
	}
	if !strings.Contains(res.Issues[0].Reason, "symbolic link") {
		t.Fatalf("symlink reason: %q", res.Issues[0].Reason)
	}
}

func TestSealedAndWorkspaceSourceRejected(t *testing.T) {
	ws, src := newCaseAndSource(t)
	write(t, filepath.Join(src, "a.txt"), "alpha")
	if _, err := Run(ws, Request{Paths: []string{src}, Options: Options{Copy: true}, Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false)}); err != nil {
		t.Fatal(err)
	}
	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Add("case_sealed", ws.Case.CaseID, "stratum", model.CaseSealedData{Operator: ws.Case.Examiner, Note: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Close(); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(src, "late.txt"), "late")
	if _, err := Run(ws, Request{Paths: []string{filepath.Join(src, "late.txt")}, Options: Options{Copy: true}, Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false)}); !errors.Is(err, ErrSealed) {
		t.Fatalf("sealed error = %v", err)
	}

	// A source inside the workspace is rejected before a lock or record is made.
	if _, err := Run(ws, Request{Paths: []string{ws.ItemsPath}, Options: Options{Copy: true}, Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false)}); err == nil {
		t.Fatal("workspace source accepted")
	}
}

func TestLockIsHonoured(t *testing.T) {
	ws, src := newCaseAndSource(t)
	write(t, filepath.Join(src, "a.txt"), "alpha")
	lock, err := ws.AcquireLock(false, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if _, err := Run(ws, Request{Paths: []string{src}, Options: Options{Copy: true}, Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false)}); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected lock error, got %v", err)
	}
}

func newCaseAndSource(t *testing.T) (*workspace.Workspace, string) {
	t.Helper()
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir:      filepath.Join(t.TempDir(), "case"),
		Name:     "Acquire test",
		Examiner: model.Person{Name: "Jane Doe", Org: "Lab", Email: "jane@example.com"},
		Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ws, t.TempDir()
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hashTree(t *testing.T, root string) string {
	t.Helper()
	var b bytes.Buffer
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		h, size, err := chain.HashFile(path)
		if err != nil {
			return err
		}
		b.WriteString(rel + " " + h + " " + strings.Repeat("x", int(size%7)) + "\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
