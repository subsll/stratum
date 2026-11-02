package chain

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendVerifyAndReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evidence.jsonl")

	r, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		if _, err := r.Add("test", "target", "unit", map[string]any{"n": i, "text": "hello"}); err != nil {
			t.Fatal(err)
		}
	}
	info, err := r.Close()
	if err != nil {
		t.Fatal(err)
	}
	if info.Records != 3 || !info.ChainOK || info.RootSHA256 == "" || info.FileSHA256 == "" {
		t.Fatalf("unexpected close info: %+v", info)
	}

	res, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Report.OK || res.Report.Records != 3 {
		t.Fatalf("verification failed: %+v", res.Report)
	}
	if res.Records[0].Seq != 1 || res.Records[0].PrevHash != "" {
		t.Fatalf("bad first record: %+v", res.Records[0])
	}
	if res.Records[2].PrevHash != res.Records[1].Hash {
		t.Fatal("prev hash not linked")
	}
	if res.Report.RootSHA256 != res.Records[2].Hash {
		t.Fatal("root hash is not the last record hash")
	}
	// File hash must cover the exact bytes.
	data, _ := os.ReadFile(path)
	sum := sha256.Sum256(data)
	if res.Report.FileSHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("file hash mismatch")
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatal("ledger must end with a newline")
	}

	// Reopen appends and preserves the existing chain.
	r2, err := OpenAppend(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r2.Add("test", "target", "unit", map[string]any{"n": 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := r2.Close(); err != nil {
		t.Fatal(err)
	}
	res, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Report.OK || res.Report.Records != 4 {
		t.Fatalf("reopened chain failed: %+v", res.Report)
	}
}

func TestTamperDetection(t *testing.T) {
	path := makeChain(t, 3)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(data), `"n":2`, `"n":20`, 1)
	if tampered == string(data) {
		t.Fatal("test fixture did not contain the expected token")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("tampered ledger verified")
	}
	if len(rep.Errors) == 0 || !strings.Contains(rep.Errors[0], "hash mismatch") {
		t.Fatalf("unexpected errors: %v", rep.Errors)
	}
	if _, err := OpenAppend(path); err == nil {
		t.Fatal("OpenAppend accepted a broken chain")
	}
}

func TestBlankLineAndMissingNewline(t *testing.T) {
	path := makeChain(t, 2)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append([]byte("\n"), data...), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, _ := VerifyFile(path)
	if rep.OK || !strings.Contains(strings.Join(rep.Errors, " "), "blank line") {
		t.Fatalf("blank line not detected: %+v", rep)
	}

	path = makeChain(t, 2)
	data, _ = os.ReadFile(path)
	if err := os.WriteFile(path, data[:len(data)-1], 0o600); err != nil {
		t.Fatal(err)
	}
	rep, _ = VerifyFile(path)
	if rep.OK || !strings.Contains(strings.Join(rep.Errors, " "), "newline") {
		t.Fatalf("truncated newline not detected: %+v", rep)
	}
}

func TestOpenAppendRejectsMissingNewline(t *testing.T) {
	path := makeChain(t, 2)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data[:len(data)-1], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAppend(path); err == nil || !strings.Contains(err.Error(), "newline") {
		t.Fatalf("OpenAppend error = %v", err)
	}
}

func TestNonCanonicalLineDetected(t *testing.T) {
	path := makeChain(t, 2)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Add harmless-looking whitespace after a comma. The JSON still parses, but
	// the line is not canonical.
	tampered := strings.Replace(string(data), `"type":"test"`, `"type": "test"`, 1)
	if tampered == string(data) {
		t.Fatal("fixture not modified")
	}
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || !strings.Contains(strings.Join(rep.Errors, " "), "non-canonical") {
		t.Fatalf("non-canonical line not detected: %+v", rep)
	}
}

func TestEmptyAndBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := VerifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.Records != 0 {
		t.Fatalf("empty ledger should not verify: %+v", rep)
	}

	path = filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(path, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err = VerifyFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatal("invalid JSON verified")
	}
}

func TestHashReaderAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	h, n, err := HashFile(path)
	if err != nil || n != 3 || h != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("hash=%q n=%d err=%v", h, n, err)
	}
}

func makeChain(t *testing.T, n int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "chain.jsonl")
	r, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		if _, err := r.Add("test", "target", "unit", map[string]any{"n": i}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
