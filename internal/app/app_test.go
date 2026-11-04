package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

func TestFullWorkflow(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("STRATUM_WORKSPACE", "")
	t.Setenv("STRATUM_EXAMINER", "")

	src := t.TempDir()
	writeAppFile(t, filepath.Join(src, "page.html"), "<!doctype html><html><title>Acme</title></html>")
	writeAppFile(t, filepath.Join(src, "brief.pdf"), "%PDF-1.7\n")
	writeAppFile(t, filepath.Join(src, "notes.txt"), "notes\n")
	writeAppFile(t, filepath.Join(src, "notes-copy.txt"), "notes\n")
	wsDir := filepath.Join(t.TempDir(), "case")

	code, out, errOut := runApp("init", "Smith v Acme", "--dir", wsDir, "--id", "CASE-E2E-001", "--examiner", "Jane Doe", "--org", "Doe Investigations", "--email", "jane@example.com")
	if code != 0 {
		t.Fatalf("init code=%d out=%s err=%s", code, out, errOut)
	}
	if !strings.Contains(out, "case workspace created") || !strings.Contains(out, "CASE-E2E-001") {
		t.Fatalf("init output: %s", out)
	}
	for _, name := range []string{"case.json", "evidence.jsonl", "custody.jsonl", "items", "reports", "README.md"} {
		if _, err := os.Stat(filepath.Join(wsDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	// Flags may follow positionals; add logs each item.
	code, out, errOut = runApp("add", src, "-w", wsDir, "--origin", "client delivery", "--exclude", "*.tmp")
	if code != 0 {
		t.Fatalf("add code=%d out=%s err=%s", code, out, errOut)
	}
	for _, want := range []string{"acquisition complete", "E-0001", "duplicate"} {
		if !strings.Contains(out, want) {
			t.Fatalf("add output missing %q: %s", want, out)
		}
	}

	code, out, _ = runApp("list", "-w", wsDir, "--hash")
	if code != 0 || !strings.Contains(out, "E-0004") || !strings.Contains(out, "sha256") {
		t.Fatalf("list code=%d out=%s", code, out)
	}
	code, out, _ = runApp("show", "-w", wsDir, "E-0001")
	if code != 0 || !strings.Contains(out, "PDF document") || !strings.Contains(out, "integrity   OK") {
		t.Fatalf("show code=%d out=%s", code, out)
	}

	code, out, errOut = runApp("custody", "transfer", "-w", wsDir, "--to", "Bob Smith", "--to-org", "Acme Legal", "--purpose", "production to counsel", "--method", "hand delivery")
	if code != 0 || !strings.Contains(out, "custody transferred recorded") {
		t.Fatalf("transfer code=%d out=%s err=%s", code, out, errOut)
	}
	code, out, _ = runApp("custody", "list", "-w", wsDir)
	if code != 0 || !strings.Contains(out, "Bob Smith (Acme Legal)") || !strings.Contains(out, "transferred") {
		t.Fatalf("custody list code=%d out=%s", code, out)
	}

	code, out, errOut = runApp("seal", "-w", wsDir, "--note", "evidence set closed")
	if code != 0 || !strings.Contains(out, "case sealed") {
		t.Fatalf("seal code=%d out=%s err=%s", code, out, errOut)
	}
	writeAppFile(t, filepath.Join(src, "late.txt"), "late\n")
	code, _, errOut = runApp("add", "-w", wsDir, filepath.Join(src, "late.txt"))
	if code != 1 || !strings.Contains(errOut, "sealed") {
		t.Fatalf("sealed add code=%d err=%s", code, errOut)
	}

	code, out, errOut = runApp("verify", "-w", wsDir)
	if code != 0 || !strings.Contains(out, "PASSED (complete)") {
		t.Fatalf("verify code=%d out=%s err=%s", code, out, errOut)
	}
	code, out, errOut = runApp("verify", "-w", wsDir, "--record", "--examiner", "Jane Doe")
	if code != 0 || !strings.Contains(out, "recorded     verification") {
		t.Fatalf("verify --record code=%d out=%s err=%s", code, out, errOut)
	}

	code, out, errOut = runApp("report", "-w", wsDir, "-f", "html,json", "--examiner", "Jane Doe")
	if code != 0 || !strings.Contains(out, "report written") {
		t.Fatalf("report code=%d out=%s err=%s", code, out, errOut)
	}
	reports, _ := filepath.Glob(filepath.Join(wsDir, "reports", "*"))
	if len(reports) != 1 {
		t.Fatalf("reports: %v", reports)
	}
	for _, name := range []string{"report.html", "report.json", "MANIFEST.json", "SHA256SUMS"} {
		if _, err := os.Stat(filepath.Join(reports[0], name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(reports[0], "report.txt")); err == nil {
		t.Fatal("html,json report unexpectedly contains report.txt")
	}

	code, out, errOut = runApp("workspaces", "--json")
	if code != 0 || !strings.Contains(out, "CASE-E2E-001") {
		t.Fatalf("workspaces code=%d out=%s err=%s", code, out, errOut)
	}
	var rows []workspaceRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || rows[0].Status != "ok" {
		t.Fatalf("workspaces json: %v %+v", err, rows)
	}

	code, out, errOut = runApp("unseal", "-w", wsDir, "--reason", "late production received")
	if code != 0 || !strings.Contains(out, "case unsealed") {
		t.Fatalf("unseal code=%d out=%s err=%s", code, out, errOut)
	}
	code, out, errOut = runApp("add", "-w", wsDir, filepath.Join(src, "late.txt"))
	if code != 0 || !strings.Contains(out, "E-0005") {
		t.Fatalf("post-unseal add code=%d out=%s err=%s", code, out, errOut)
	}

	// Tampering with a preserved copy is visible to verify, show and report.
	items, _ := filepath.Glob(filepath.Join(wsDir, "items", "E-0001__*"))
	if len(items) != 1 {
		t.Fatalf("items: %v", items)
	}
	if err := os.Chmod(items[0], 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(items[0], []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runApp("verify", "-w", wsDir)
	if code != 1 || !strings.Contains(out, "verdict     FAILED") {
		t.Fatalf("tampered verify code=%d out=%s", code, out)
	}
	code, _, _ = runApp("show", "-w", wsDir, "E-0001")
	if code != 1 {
		t.Fatalf("tampered show code=%d", code)
	}
	code, out, _ = runApp("report", "-w", wsDir, "-f", "text")
	if code != 1 || !strings.Contains(out, "report written") {
		t.Fatalf("tampered report code=%d out=%s", code, out)
	}
}

func TestStandaloneVerifyAndHelpers(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	dir := t.TempDir()
	ws, _, err := createTestWorkspace(t, dir)
	if err != nil {
		t.Fatal(err)
	}
	code, out, _ := runApp("verify", ws.EvidencePath)
	if code != 0 || !strings.Contains(out, "verdict     OK") {
		t.Fatalf("standalone verify code=%d out=%s", code, out)
	}
	if err := os.WriteFile(ws.EvidencePath+".sha256", []byte(strings.Repeat("0", 64)+"  evidence.jsonl\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runApp("verify", ws.EvidencePath)
	if code != 1 || !strings.Contains(out, "sidecar     MISMATCH") {
		t.Fatalf("sidecar verify code=%d out=%s", code, out)
	}
}

func TestCLIUsage(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	cases := []struct {
		args []string
		code int
		want string
		ok   bool
	}{
		{[]string{"version"}, 0, "stratum", true},
		{[]string{"help", "add"}, 0, "stratum add", true},
		{[]string{"--help"}, 0, "QUICK START", true},
		{[]string{"init", "--help"}, 0, "stratum init", true},
		{[]string{"not-a-command"}, 2, "unknown command", true},
		{[]string{"add"}, 2, "at least one source path", true},
		{[]string{"init"}, 2, "case name is required", true},
	}
	for _, tc := range cases {
		code, out, errOut := runApp(tc.args...)
		combined := out + errOut
		if code != tc.code {
			t.Errorf("%v: code=%d want %d (out=%s err=%s)", tc.args, code, tc.code, out, errOut)
			continue
		}
		if !strings.Contains(combined, tc.want) {
			t.Errorf("%v: output missing %q: %s", tc.args, tc.want, combined)
		}
	}
}

func createTestWorkspace(t *testing.T, dir string) (*workspace.Workspace, []string, error) {
	t.Helper()
	return workspace.Create(workspace.CreateOptions{
		Dir: dir, Name: "Standalone test", Examiner: model.Person{Name: "Jane Doe"}, Register: false,
	})
}

func runApp(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func writeAppFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRecordJSONIsPure(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	ws, _, err := createTestWorkspace(t, filepath.Join(t.TempDir(), "case"))
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runApp("verify", "-w", ws.Root, "--record", "--json", "--examiner", "Jane Doe")
	if code != 0 {
		t.Fatalf("verify --record --json code=%d err=%s", code, errOut)
	}
	var doc struct {
		Verification struct {
			OK bool `json:"ok"`
		} `json:"verification"`
		EvidenceRecord struct {
			Seq int `json:"seq"`
		} `json:"evidence_record"`
		CustodyRecord struct {
			Seq int `json:"seq"`
		} `json:"custody_record"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not pure JSON: %v\n%s", err, out)
	}
	if !doc.Verification.OK || doc.EvidenceRecord.Seq != 2 || doc.CustodyRecord.Seq != 2 {
		t.Fatalf("unexpected output: %+v", doc)
	}
}

func TestExtractBackfillAndVerify(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	src := t.TempDir()
	writeAppFile(t, filepath.Join(src, "report.pdf"), "%PDF-1.7\n1 0 obj<</Type/Catalog>>endobj\n")
	wsDir := filepath.Join(t.TempDir(), "case")
	if code, out, errOut := runApp("init", "Extract test", "--dir", wsDir, "--id", "CASE-EXTRACT-1", "--examiner", "Jane Doe"); code != 0 {
		t.Fatalf("init: %d %s %s", code, out, errOut)
	}
	if code, out, errOut := runApp("add", "-w", wsDir, src); code != 0 {
		t.Fatalf("add: %d %s %s", code, out, errOut)
	}
	sidecars, _ := filepath.Glob(filepath.Join(wsDir, "metadata", "E-0001__*.json"))
	if len(sidecars) != 1 {
		t.Fatalf("sidecars: %v", sidecars)
	}
	if err := os.Remove(sidecars[0]); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runApp("verify", "-w", wsDir)
	if code != 1 || !strings.Contains(out, "metadata") {
		t.Fatalf("verify after sidecar removal code=%d out=%s", code, out)
	}
	code, _, _ = runApp("extract", "-w", wsDir, "E-0001")
	if code != 1 {
		t.Fatalf("extract without --force code=%d", code)
	}
	code, out, errOut := runApp("extract", "-w", wsDir, "E-0001", "--force", "--examiner", "Jane Doe")
	if code != 0 || !strings.Contains(out, "metadata") {
		t.Fatalf("extract --force code=%d out=%s err=%s", code, out, errOut)
	}
	if code, out, errOut := runApp("verify", "-w", wsDir); code != 0 || !strings.Contains(out, "metadata    1 checked, 1 verified") {
		t.Fatalf("verify after extract code=%d out=%s err=%s", code, out, errOut)
	}
	code, out, _ = runApp("show", "-w", wsDir, "E-0001")
	if code != 0 || !strings.Contains(out, "meta status OK") {
		t.Fatalf("show after extract code=%d out=%s", code, out)
	}
}
