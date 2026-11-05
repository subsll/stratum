package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/model"
)

func TestCompareCommand(t *testing.T) {
	t.Setenv("STRATUM_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("STRATUM_WORKSPACE", "")
	t.Setenv("STRATUM_EXAMINER", "")

	ws, _, err := createTestWorkspace(t, filepath.Join(t.TempDir(), "case"))
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("already captured\n")
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])

	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Add("item_acquired", "E-0001", "fs", model.Item{
		ID: "E-0001", Name: "captured.txt", StoredPath: "items/E-0001__captured.txt",
		SHA256: digest,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ev.Close(); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(t.TempDir(), "captured.txt")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runApp("compare", file, "-w", ws.Root)
	if code != 0 || !strings.Contains(out, "ALREADY CAPTURED") || !strings.Contains(out, "E-0001") {
		t.Fatalf("captured code=%d out=%s err=%s", code, out, errOut)
	}

	code, out, errOut = runApp("compare", file, "--evidence", ws.EvidencePath)
	if code != 0 || !strings.Contains(out, "ALREADY CAPTURED") {
		t.Fatalf("standalone code=%d out=%s err=%s", code, out, errOut)
	}

	code, out, errOut = runApp("compare", "--hash", digest, "-w", ws.Root, "--json")
	if code != 0 {
		t.Fatalf("hash json code=%d err=%s", code, errOut)
	}
	var result compareOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("json: %v: %s", err, out)
	}
	if !result.Captured || result.CaseID == "" || len(result.Matches) != 1 {
		t.Fatalf("result = %+v", result)
	}

	newFile := filepath.Join(t.TempDir(), "new.txt")
	if err := os.WriteFile(newFile, []byte("new evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut = runApp("compare", newFile, "-w", ws.Root)
	if code != 1 || !strings.Contains(out, "NEW EVIDENCE") {
		t.Fatalf("new code=%d out=%s err=%s", code, out, errOut)
	}

	if err := os.WriteFile(ws.EvidencePath+".sha256", []byte(strings.Repeat("0", 64)+"  evidence.jsonl\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut = runApp("compare", file, "-w", ws.Root)
	if code != 2 || !strings.Contains(errOut, "sidecar MISMATCH") {
		t.Fatalf("sidecar code=%d err=%s", code, errOut)
	}
	if err := os.Remove(ws.EvidencePath + ".sha256"); err != nil {
		t.Fatal(err)
	}

	code, _, _ = runApp("compare", file, "-w", filepath.Join(t.TempDir(), "missing"))
	if code != 2 {
		t.Fatalf("missing workspace code=%d, want 2", code)
	}
}
