package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/model"
)

func TestCreateOpenAndRegistry(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("STRATUM_HOME", home)
	t.Setenv("STRATUM_WORKSPACE", "")
	dir := filepath.Join(t.TempDir(), "case")
	examiner := model.Person{Name: "Jane Doe", Org: "Doe Investigations", Email: "jane@example.com"}
	ws, warnings, err := Create(CreateOptions{
		Dir: dir, Name: "Smith v Acme", CaseID: "CASE-TEST-001",
		Description: "test case", Examiner: examiner, Register: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	if ws.Root != dir || ws.Case.CaseID != "CASE-TEST-001" || ws.Case.Examiner.Name != "Jane Doe" {
		t.Fatalf("workspace: %+v", ws)
	}
	for _, p := range []string{ws.CasePath, ws.EvidencePath, ws.CustodyPath, ws.ReadmePath, ws.ItemsPath, ws.MetadataPath, ws.ReportsPath} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing %s: %v", p, err)
		}
	}
	if st, _ := os.Stat(ws.ItemsPath); st.Mode().Perm()&0o700 != 0o700 {
		t.Fatalf("items dir mode = %v", st.Mode())
	}
	if st, _ := os.Stat(ws.CasePath); st.Mode().Perm()&0o600 != 0o600 {
		t.Fatalf("case.json mode = %v", st.Mode())
	}
	data, err := os.ReadFile(ws.ReadmePath)
	if err != nil || !strings.Contains(string(data), "CASE-TEST-001") {
		t.Fatalf("README: %v %q", err, string(data))
	}
	for _, path := range []string{ws.EvidencePath, ws.CustodyPath} {
		rep, err := chain.VerifyFile(path)
		if err != nil || !rep.OK || rep.Records != 1 {
			t.Fatalf("%s: %+v %v", path, rep, err)
		}
	}

	opened, err := Open(dir)
	if err != nil || opened.Case.Name != "Smith v Acme" {
		t.Fatalf("Open: %+v %v", opened, err)
	}
	nested := filepath.Join(dir, "items", "deep")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil || found.Root != dir {
		t.Fatalf("Find: %+v %v", found, err)
	}
	t.Setenv("STRATUM_WORKSPACE", dir)
	resolved, err := Resolve("")
	if err != nil || resolved.Root != dir {
		t.Fatalf("Resolve env: %+v %v", resolved, err)
	}

	entries, err := ListRegistry()
	if err != nil || len(entries) != 1 || entries[0].CaseID != "CASE-TEST-001" {
		t.Fatalf("registry: %+v %v", entries, err)
	}
	// Registering the same path updates rather than duplicates.
	if err := Register(RegistryEntry{Path: dir, CaseID: "CASE-TEST-001", Name: "Updated"}); err != nil {
		t.Fatal(err)
	}
	entries, _ = ListRegistry()
	if len(entries) != 1 || entries[0].Name != "Updated" {
		t.Fatalf("registry dedupe: %+v", entries)
	}
	raw, _ := os.ReadFile(filepath.Join(home, "workspaces.json"))
	var reg Registry
	if err := json.Unmarshal(raw, &reg); err != nil || len(reg.Workspaces) != 1 {
		t.Fatalf("registry json: %v %+v", err, reg)
	}
}

func TestCreateValidation(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	dir := filepath.Join(t.TempDir(), "case")
	if _, _, err := Create(CreateOptions{Dir: dir, Name: "x"}); err == nil {
		t.Fatal("expected an examiner requirement")
	}
	if _, _, err := Create(CreateOptions{Dir: dir, Name: "", Examiner: model.Person{Name: "A"}}); err == nil {
		t.Fatal("expected a name requirement")
	}
	if _, _, err := Create(CreateOptions{Dir: dir, Name: "x", CaseID: "bad/id", Examiner: model.Person{Name: "A"}}); err == nil {
		t.Fatal("expected an invalid id error")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "occupied"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Create(CreateOptions{Dir: dir, Name: "x", Examiner: model.Person{Name: "A"}}); err == nil {
		t.Fatal("expected a non-empty directory error")
	}
}

func TestSlugAndStoredPaths(t *testing.T) {
	cases := map[string]string{
		"Smith v Acme":          "smith-v-acme",
		"  Multiple   spaces  ": "multiple-spaces",
		"UPPER_case":            "upper-case",
		"":                      "case",
		"!!!":                   "case",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	dir := filepath.Join(t.TempDir(), "case")
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := Create(CreateOptions{Dir: dir, Name: "x", Examiner: model.Person{Name: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	full, err := ws.ResolveStored("items/E-0001__a.txt")
	if err != nil || !strings.HasPrefix(full, ws.ItemsPath) {
		t.Fatalf("resolve: %q %v", full, err)
	}
	for _, bad := range []string{"", "..", "../evil", "/etc/passwd", "items/../../evil"} {
		if _, err := ws.ResolveStored(bad); err == nil {
			t.Errorf("ResolveStored(%q) accepted", bad)
		}
	}
}

func TestLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case")
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := Create(CreateOptions{Dir: dir, Name: "x", Examiner: model.Person{Name: "A"}})
	if err != nil {
		t.Fatal(err)
	}
	l1, err := ws.AcquireLock(false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AcquireLock(false, "test"); err == nil {
		t.Fatal("second lock acquired")
	}
	if err := l1.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := ws.AcquireLock(false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := l2.Release(); err != nil {
		t.Fatal(err)
	}

	// A stale lock can be reclaimed without force.
	lockPath := filepath.Join(dir, LockFile)
	if err := os.WriteFile(lockPath, []byte(`{"token":"old","pid":999999,"time":"2020-01-01T00:00:00Z"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	l3, err := ws.AcquireLock(false, "test")
	if err != nil {
		t.Fatalf("stale lock: %v", err)
	}
	if err := l3.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestEnvironmentHasHost(t *testing.T) {
	env := Environment()
	if env.Tool != model.ToolName || env.OS == "" || env.Arch == "" {
		t.Fatalf("environment: %+v", env)
	}
}
