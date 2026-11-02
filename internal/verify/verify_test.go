package verify_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stratum/internal/acquire"
	"stratum/internal/casestate"
	"stratum/internal/logx"
	"stratum/internal/model"
	"stratum/internal/verify"
	"stratum/internal/workspace"
)

func TestVerifyHealthyAndTampered(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.OK || !rep.Complete || rep.ItemsOK != 2 || !rep.CrossLinksOK || rep.MetadataChecked != 2 || rep.MetadataOK != 2 {
			t.Fatalf("healthy verify: %+v", rep)
		}
		if rep.CaseFileHash == "" || rep.Evidence.FileSHA256 == "" || rep.Custody.FileSHA256 == "" {
			t.Fatalf("missing hashes: %+v", rep)
		}
	})

	t.Run("modified metadata sidecar", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		item := st.Items[0]
		full, err := ws.ResolveStored(item.Metadata.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(full, 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.MetadataModified) != 1 || rep.MetadataOK != 1 {
			t.Fatalf("metadata tamper verify: %+v", rep)
		}
	})

	t.Run("missing metadata sidecar", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		full, err := ws.ResolveStored(st.Items[0].Metadata.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(full); err != nil {
			t.Fatal(err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.MetadataMissing) != 1 {
			t.Fatalf("metadata missing verify: %+v", rep)
		}
	})

	t.Run("unexpected metadata file", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		if err := os.WriteFile(filepath.Join(ws.MetadataPath, "extra.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.MetadataUnexpected) != 1 {
			t.Fatalf("metadata unexpected verify: %+v", rep)
		}
	})

	t.Run("modified copy", func(t *testing.T) {
		ws, st, itemPath := setupCase(t)
		if err := os.Chmod(itemPath, 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(itemPath, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.WriteString("tampered")
		_ = f.Close()
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.Modified) != 1 || rep.ItemsOK != 1 {
			t.Fatalf("modified verify: %+v", rep)
		}
		if !strings.Contains(rep.Modified[0].Note, "bytes") {
			t.Fatalf("note: %q", rep.Modified[0].Note)
		}
	})

	t.Run("missing copy", func(t *testing.T) {
		ws, st, itemPath := setupCase(t)
		if err := os.Remove(itemPath); err != nil {
			t.Fatal(err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.Missing) != 1 {
			t.Fatalf("missing verify: %+v", rep)
		}
	})

	t.Run("symlinked copy", func(t *testing.T) {
		ws, st, itemPath := setupCase(t)
		other := filepath.Join(t.TempDir(), "other")
		if err := os.WriteFile(other, []byte("same or different"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(itemPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, itemPath); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.Modified) != 1 {
			t.Fatalf("symlink verify: %+v", rep)
		}
	})

	t.Run("unexpected file", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		if err := os.WriteFile(filepath.Join(ws.ItemsPath, "extra.bin"), []byte("extra"), 0o600); err != nil {
			t.Fatal(err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || len(rep.Unexpected) != 1 {
			t.Fatalf("unexpected verify: %+v", rep)
		}
	})

	t.Run("broken chain", func(t *testing.T) {
		ws, st, _ := setupCase(t)
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
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK || rep.ChainsOK {
			t.Fatalf("broken chain verify: %+v", rep)
		}
	})

	t.Run("no rehash", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		rep, err := verify.Run(ws, st, false)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.OK || rep.Complete || rep.Rehashed || rep.ItemsOK != 0 || rep.MetadataChecked != 0 {
			t.Fatalf("no rehash: %+v", rep)
		}
	})

	t.Run("case json drift", func(t *testing.T) {
		ws, st, _ := setupCase(t)
		data, err := os.ReadFile(ws.CasePath)
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), "Verify test", "Drifted", 1))
		if err := os.WriteFile(ws.CasePath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		ws, err = workspace.Open(ws.Root)
		if err != nil {
			t.Fatal(err)
		}
		st, err = casestate.Load(ws)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := verify.Run(ws, st, true)
		if err != nil {
			t.Fatal(err)
		}
		if !rep.OK {
			t.Fatalf("case drift should warn, not fail: %+v", rep)
		}
		found := false
		for _, w := range rep.Warnings {
			if strings.Contains(w, "differs") {
				found = true
			}
		}
		if !found {
			t.Fatalf("drift warning missing: %v", rep.Warnings)
		}
	})
}

func TestReferenceOnlyIsPartial(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir: filepath.Join(t.TempDir(), "case"), Name: "Reference test",
		Examiner: model.Person{Name: "Jane Doe"}, Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "ref.txt")
	if err := os.WriteFile(src, []byte("reference"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquire.Run(ws, acquire.Request{
		Paths: []string{src}, Options: acquire.Options{Copy: false},
		Operator: ws.Case.Examiner, Log: logx.New(io.Discard, false),
	}); err != nil {
		t.Fatal(err)
	}
	st, err := casestate.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := verify.Run(ws, st, true)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Complete || len(rep.Reference) != 1 || rep.MetadataChecked != 1 || rep.MetadataOK != 1 {
		t.Fatalf("reference verify: %+v", rep)
	}
	if rep.Reference[0].Note == "" {
		t.Fatal("reference note missing")
	}
}

func setupCase(t *testing.T) (*workspace.Workspace, *casestate.State, string) {
	t.Helper()
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := workspace.Create(workspace.CreateOptions{
		Dir: filepath.Join(t.TempDir(), "case"), Name: "Verify test",
		Examiner: model.Person{Name: "Jane Doe", Org: "Lab"}, Register: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	for name, content := range map[string]string{
		"a.txt": "alpha\n",
		"b.txt": "beta\n",
	} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
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
	itemPath, err := ws.ResolveStored(st.Items[0].StoredPath)
	if err != nil {
		t.Fatal(err)
	}
	return ws, st, itemPath
}
