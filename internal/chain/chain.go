// Package chain implements an append-only, hash-chained JSONL ledger.
//
// It is deliberately compatible with the chain format used by Subsoil: each
// record commits to its own content and to the hash of the previous record.
// Changing, deleting or reordering any record breaks verification.
//
// A hash chain is tamper-evident, not tamper-proof. Anyone who can rewrite the
// whole file can recompute every hash. Anchor the final root hash somewhere the
// custodian does not control (a signed report, an email, an RFC 3161 timestamp)
// when that distinction matters.
package chain

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is one line of a ledger.
type Record struct {
	Seq      int             `json:"seq"`
	Time     string          `json:"time"`
	Type     string          `json:"type"`
	Target   string          `json:"target"`
	Source   string          `json:"source"`
	PrevHash string          `json:"prev_hash,omitempty"`
	Data     json.RawMessage `json:"data"`
	Hash     string          `json:"hash"`
}

// Info describes a closed ledger.
type Info struct {
	File       string `json:"file"`
	Records    int    `json:"records"`
	RootSHA256 string `json:"root_sha256"`
	FileSHA256 string `json:"file_sha256"`
	ChainOK    bool   `json:"chain_ok"`
}

// Report is the outcome of verifying a ledger.
type Report struct {
	File       string   `json:"file"`
	Records    int      `json:"records"`
	RootSHA256 string   `json:"root_sha256"`
	FileSHA256 string   `json:"file_sha256"`
	OK         bool     `json:"ok"`
	Errors     []string `json:"errors,omitempty"`
}

// LoadResult is a verified (or partially verified) ledger.
type LoadResult struct {
	Records []Record
	Report  *Report
}

type payload struct {
	Seq      int             `json:"seq"`
	Time     string          `json:"time"`
	Type     string          `json:"type"`
	Target   string          `json:"target"`
	Source   string          `json:"source"`
	PrevHash string          `json:"prev_hash,omitempty"`
	Data     json.RawMessage `json:"data"`
}

// Recorder appends records to a ledger. Add is safe for concurrent use.
type Recorder struct {
	mu     sync.Mutex
	f      *os.File
	path   string
	seq    int
	prev   string
	closed bool
	sync   bool
}

// OpenAppend opens an existing ledger for appending. It verifies the existing
// chain first and refuses to extend a corrupted file. An empty or missing file
// starts a new chain.
func OpenAppend(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	r := &Recorder{f: f, path: path, sync: true}
	if st.Size() == 0 {
		return r, nil
	}
	var lastByte [1]byte
	if _, err := f.ReadAt(lastByte[:], st.Size()-1); err != nil {
		_ = f.Close()
		return nil, err
	}
	if lastByte[0] != '\n' {
		_ = f.Close()
		return nil, fmt.Errorf("%s: refusing to append: file does not end with a newline", filepath.Base(path))
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, err
	}
	res, err := loadFrom(f, path)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !res.Report.OK {
		_ = f.Close()
		return nil, fmt.Errorf("%s: refusing to append to a broken chain: %s", filepath.Base(path), firstError(res.Report))
	}
	if len(res.Records) == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("%s: ledger is empty", path)
	}
	last := res.Records[len(res.Records)-1]
	r.seq = last.Seq
	r.prev = last.Hash
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		_ = f.Close()
		return nil, err
	}
	return r, nil
}

// New creates/truncates a ledger. Existing evidence must never be passed to
// New; it exists for tests and first-run setup where the file is known empty.
func New(path string) (*Recorder, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &Recorder{f: f, path: path, sync: true}, nil
}

// Path returns the ledger path.
func (r *Recorder) Path() string { return r.path }

// SetSync controls whether Close calls fsync. It is enabled by default.
func (r *Recorder) SetSync(v bool) {
	r.mu.Lock()
	r.sync = v
	r.mu.Unlock()
}

// Add appends a record and returns it, including its hash.
func (r *Recorder) Add(typ, target, source string, data any) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return Record{}, errors.New("chain: recorder is closed")
	}

	dataJSON, err := jsonMarshal(data)
	if err != nil {
		return Record{}, fmt.Errorf("chain: marshal data: %w", err)
	}

	rec := Record{
		Seq:      r.seq + 1,
		Time:     time.Now().UTC().Format(time.RFC3339Nano),
		Type:     typ,
		Target:   target,
		Source:   source,
		PrevHash: r.prev,
		Data:     dataJSON,
	}
	canon, err := canonical(rec)
	if err != nil {
		return Record{}, err
	}
	sum := sha256.Sum256(canon)
	rec.Hash = hex.EncodeToString(sum[:])

	line, err := jsonMarshal(rec)
	if err != nil {
		return Record{}, fmt.Errorf("chain: marshal record: %w", err)
	}
	line = append(line, '\n')
	if _, err := r.f.Write(line); err != nil {
		return Record{}, err
	}
	r.seq = rec.Seq
	r.prev = rec.Hash
	return rec, nil
}

// Flush forces written records to stable storage.
func (r *Recorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	return r.f.Sync()
}

// Close flushes, closes and summarises the ledger. It is safe to call twice.
func (r *Recorder) Close() (Info, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info := Info{
		File:       r.path,
		Records:    r.seq,
		RootSHA256: r.prev,
		ChainOK:    true,
	}
	if r.closed {
		info.FileSHA256 = fileHash(r.path)
		return info, nil
	}
	if r.sync {
		if err := r.f.Sync(); err != nil {
			r.closed = true
			_ = r.f.Close()
			return info, err
		}
	}
	if err := r.f.Close(); err != nil {
		r.closed = true
		return info, err
	}
	r.closed = true
	info.FileSHA256 = fileHash(r.path)
	return info, nil
}

// Load reads and verifies a ledger. A verification failure is reported in
// Report.OK; only I/O errors are returned as an error.
func Load(path string) (*LoadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	res, err := loadFrom(f, path)
	if err != nil {
		return nil, err
	}
	// A truncated final newline still parses, but it means the bytes were
	// modified outside the writer. Treat it as a chain error.
	if st, statErr := f.Stat(); statErr == nil && st.Size() > 0 {
		var last [1]byte
		if _, readErr := f.ReadAt(last[:], st.Size()-1); readErr == nil && last[0] != '\n' {
			res.Report.failf("file does not end with a newline")
		}
	}
	if res.Report.FileSHA256 == "" {
		res.Report.FileSHA256 = fileHash(path)
	}
	return res, nil
}

// VerifyFile verifies a ledger and returns its report.
func VerifyFile(path string) (*Report, error) {
	res, err := Load(path)
	if err != nil {
		return nil, err
	}
	return res.Report, nil
}

func loadFrom(r io.Reader, name string) (*LoadResult, error) {
	rep := &Report{File: name, OK: true}
	res := &LoadResult{Report: rep}

	h := sha256.New()
	sc := bufio.NewScanner(io.TeeReader(r, h))
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)

	var prev string
	seq := 0
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := append([]byte(nil), sc.Bytes()...)
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			rep.failf("line %d: blank line", lineNo)
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			rep.failf("line %d: not valid JSON: %v", lineNo, err)
			continue
		}
		if canonicalLine, err := jsonMarshal(rec); err == nil && !bytes.Equal(raw, canonicalLine) {
			rep.failf("line %d: non-canonical encoding (whitespace, key order or duplicate keys)", lineNo)
		}
		seq++
		if rec.Seq != seq {
			rep.failf("line %d: sequence is %d, expected %d", lineNo, rec.Seq, seq)
		}
		if _, err := time.Parse(time.RFC3339Nano, rec.Time); err != nil {
			rep.failf("line %d: time %q is not RFC 3339 with nanoseconds", lineNo, rec.Time)
		}
		if rec.Type == "" {
			rep.failf("line %d: record type is empty", lineNo)
		}
		if rec.PrevHash != prev {
			rep.failf("line %d: previous hash %s does not match %s", lineNo, short(rec.PrevHash), short(prev))
		}
		if !json.Valid(rec.Data) {
			rep.failf("line %d: data is not valid JSON", lineNo)
		}
		canon, err := canonical(rec)
		if err != nil {
			rep.failf("line %d: cannot canonicalise: %v", lineNo, err)
			prev = rec.Hash
			res.Records = append(res.Records, rec)
			continue
		}
		sum := sha256.Sum256(canon)
		want := hex.EncodeToString(sum[:])
		if rec.Hash != want {
			rep.failf("line %d: hash mismatch (record says %s, content hashes to %s)", lineNo, short(rec.Hash), short(want))
		}
		prev = rec.Hash
		res.Records = append(res.Records, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	rep.Records = len(res.Records)
	rep.RootSHA256 = prev
	rep.FileSHA256 = hex.EncodeToString(h.Sum(nil))
	if rep.Records == 0 {
		rep.failf("log contains no records")
	}
	return res, nil
}

func canonical(rec Record) ([]byte, error) {
	p := payload{
		Seq:      rec.Seq,
		Time:     rec.Time,
		Type:     rec.Type,
		Target:   rec.Target,
		Source:   rec.Source,
		PrevHash: rec.PrevHash,
		Data:     rec.Data,
	}
	return jsonMarshal(p)
}

// jsonMarshal produces compact JSON with HTML escaping disabled. encoding/json
// sorts map keys, so the same value always produces the same bytes. The encoder
// still escapes U+2028 and U+2029; that is part of this format, as it is for
// Subsoil's chain.
func jsonMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (r *Report) failf(format string, args ...any) {
	r.OK = false
	if len(r.Errors) < 50 {
		r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
	}
}

func short(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "..."
}

func firstError(rep *Report) string {
	if len(rep.Errors) == 0 {
		return "verification failed"
	}
	return rep.Errors[0]
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashReader returns the lowercase hex SHA-256 of r. It is used by acquisition
// and verification code.
func HashReader(r io.Reader) (string, int64, error) {
	return HashReaderBuffer(r, nil)
}

// HashReaderBuffer is HashReader with a caller-supplied buffer.
func HashReaderBuffer(r io.Reader, buf []byte) (string, int64, error) {
	if len(buf) == 0 {
		buf = make([]byte, 1024*1024)
	}
	h := sha256.New()
	n, err := io.CopyBuffer(h, r, buf)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// HashFile returns the lowercase hex SHA-256 and size of a file.
func HashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	return HashReader(f)
}
