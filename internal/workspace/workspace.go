// Package workspace manages a Stratum case directory: the case marker, the two
// ledgers, preserved items, reports, the workspace lock and the local registry.
package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/model"
)

const (
	CaseFile     = "case.json"
	EvidenceFile = "evidence.jsonl"
	CustodyFile  = "custody.jsonl"
	ItemsDir     = "items"
	MetadataDir  = "metadata"
	ReportsDir   = "reports"
	ReadmeFile   = "README.md"
	LockFile     = ".stratum.lock"
)

// Workspace is an opened case directory.
type Workspace struct {
	Root         string
	CasePath     string
	EvidencePath string
	CustodyPath  string
	ItemsPath    string
	MetadataPath string
	ReportsPath  string
	ReadmePath   string
	Case         model.Case
}

// CreateOptions controls workspace creation.
type CreateOptions struct {
	Dir         string // exact directory to use; empty means <cwd>/<case-id>-<slug>
	Name        string
	CaseID      string
	Description string
	Examiner    model.Person
	Register    bool
	NoReadme    bool
}

// Open reads a case directory. The case marker must exist.
func Open(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs = filepath.Clean(abs)
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}
	w := &Workspace{
		Root:         abs,
		CasePath:     filepath.Join(abs, CaseFile),
		EvidencePath: filepath.Join(abs, EvidenceFile),
		CustodyPath:  filepath.Join(abs, CustodyFile),
		ItemsPath:    filepath.Join(abs, ItemsDir),
		MetadataPath: filepath.Join(abs, MetadataDir),
		ReportsPath:  filepath.Join(abs, ReportsDir),
		ReadmePath:   filepath.Join(abs, ReadmeFile),
	}
	data, err := os.ReadFile(w.CasePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s is not a Stratum workspace (no %s)", abs, CaseFile)
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &w.Case); err != nil {
		return nil, fmt.Errorf("read %s: %w", w.CasePath, err)
	}
	if strings.TrimSpace(w.Case.CaseID) == "" || strings.TrimSpace(w.Case.Name) == "" {
		return nil, fmt.Errorf("%s is incomplete (case_id and name are required)", w.CasePath)
	}
	return w, nil
}

// Find searches start and its parents for a case marker.
func Find(start string) (*Workspace, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, CaseFile)); err == nil {
			return Open(abs)
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return nil, fmt.Errorf("no %s found in this directory or its parents", CaseFile)
}

// Resolve opens an explicit workspace, STRATUM_WORKSPACE, or the nearest case
// containing the current directory, in that order.
func Resolve(flag string) (*Workspace, error) {
	if strings.TrimSpace(flag) != "" {
		return Open(flag)
	}
	if env := strings.TrimSpace(os.Getenv("STRATUM_WORKSPACE")); env != "" {
		return Open(env)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Find(cwd)
}

// Create builds a new case. It returns non-fatal warnings (for example, a
// registry write failure) separately from an error.
func Create(opts CreateOptions) (*Workspace, []string, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, nil, errors.New("a case name is required")
	}
	if len([]rune(name)) > 200 {
		return nil, nil, errors.New("case name is too long (max 200 characters)")
	}
	if opts.Examiner.Empty() || strings.TrimSpace(opts.Examiner.Name) == "" {
		return nil, nil, errors.New("an examiner name is required so the chain of custody has a starting custodian")
	}

	caseID := strings.TrimSpace(opts.CaseID)
	if caseID == "" {
		caseID = generateCaseID()
	}
	if !validCaseID(caseID) {
		return nil, nil, fmt.Errorf("invalid case id %q (use 2-64 letters, digits, dots, underscores or hyphens)", caseID)
	}
	slug := Slug(name)
	root := strings.TrimSpace(opts.Dir)
	if root == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, nil, err
		}
		root = filepath.Join(cwd, caseID+"-"+slug)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	abs = filepath.Clean(abs)

	// Only create into a missing or empty directory.
	if entries, err := os.ReadDir(abs); err == nil {
		if len(entries) > 0 {
			return nil, nil, fmt.Errorf("%s already exists and is not empty", abs)
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, nil, err
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return nil, nil, err
	}
	for _, dir := range []string{ItemsDir, MetadataDir, ReportsDir} {
		p := filepath.Join(abs, dir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return nil, nil, err
		}
		if err := os.Chmod(p, 0o700); err != nil {
			return nil, nil, err
		}
	}

	w := &Workspace{
		Root:         abs,
		CasePath:     filepath.Join(abs, CaseFile),
		EvidencePath: filepath.Join(abs, EvidenceFile),
		CustodyPath:  filepath.Join(abs, CustodyFile),
		ItemsPath:    filepath.Join(abs, ItemsDir),
		MetadataPath: filepath.Join(abs, MetadataDir),
		ReportsPath:  filepath.Join(abs, ReportsDir),
		ReadmePath:   filepath.Join(abs, ReadmeFile),
	}

	env := Environment()
	ev, err := chain.OpenAppend(w.EvidencePath)
	if err != nil {
		return nil, nil, err
	}
	created, err := ev.Add("case_created", caseID, "stratum", model.CaseCreatedData{
		CaseID:      caseID,
		Name:        name,
		Slug:        slug,
		Description: strings.TrimSpace(opts.Description),
		Examiner:    opts.Examiner,
		Environment: env,
	})
	if err != nil {
		_, _ = ev.Close()
		return nil, nil, err
	}
	evInfo, err := ev.Close()
	if err != nil {
		return nil, nil, err
	}

	cu, err := chain.OpenAppend(w.CustodyPath)
	if err != nil {
		return nil, nil, err
	}
	if _, err := cu.Add("custody_open", caseID, "custody", model.CustodyData{
		Action:       "opened",
		Actor:        opts.Examiner,
		From:         "unassigned",
		To:           opts.Examiner,
		Purpose:      "case opened",
		Items:        []string{"*"},
		ItemCount:    0,
		EvidenceRoot: evInfo.RootSHA256,
	}); err != nil {
		_, _ = cu.Close()
		return nil, nil, err
	}
	if _, err := cu.Close(); err != nil {
		return nil, nil, err
	}

	w.Case = model.Case{
		FormatVersion: model.FormatVersion,
		CaseID:        caseID,
		Name:          name,
		Slug:          slug,
		Description:   strings.TrimSpace(opts.Description),
		Examiner:      opts.Examiner,
		Created:       created.Time,
		Tool:          model.ToolName,
		Version:       model.Version,
	}
	if err := writeJSON(w.CasePath, w.Case, 0o600); err != nil {
		return nil, nil, err
	}
	if !opts.NoReadme {
		if err := writeFile(w.ReadmePath, []byte(readme(w, env)), 0o600); err != nil {
			return nil, nil, err
		}
	}

	var warnings []string
	if opts.Register {
		entry := RegistryEntry{
			Path:    w.Root,
			CaseID:  w.Case.CaseID,
			Name:    w.Case.Name,
			Created: w.Case.Created,
		}
		if err := Register(entry); err != nil {
			warnings = append(warnings, "workspace created, but it could not be added to the registry: "+err.Error())
		}
	}
	return w, warnings, nil
}

// Environment records the current host and build.
func Environment() model.Environment {
	host, _ := os.Hostname()
	name := strings.TrimSpace(os.Getenv("USER"))
	if name == "" {
		name = strings.TrimSpace(os.Getenv("USERNAME"))
	}
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	return model.Environment{
		Tool:      model.ToolName,
		Version:   model.Version,
		Hostname:  host,
		Username:  name,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
	}
}

// ResolveStored returns the absolute path of a relative path inside the
// workspace and rejects traversal or absolute paths from untrusted records.
func (w *Workspace) ResolveStored(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", errors.New("empty stored path")
	}
	if filepath.IsAbs(rel) {
		return "", errors.New("stored path must be relative")
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("stored path %q escapes the workspace", rel)
	}
	full := filepath.Join(w.Root, clean)
	check, err := filepath.Rel(w.Root, full)
	if err != nil || check == ".." || strings.HasPrefix(check, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("stored path %q escapes the workspace", rel)
	}
	return full, nil
}

// Contains reports whether path is the workspace or is inside it.
func (w *Workspace) Contains(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(w.Root, abs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// Slug converts a case name into a filesystem-friendly fragment.
func Slug(s string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if len([]rune(b.String())) >= 48 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "case"
	}
	return out
}

var caseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)

func validCaseID(s string) bool { return caseIDPattern.MatchString(s) }

func generateCaseID() string {
	now := time.Now().UTC()
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("CASE-%s", now.Format("20060102-150405"))
	}
	return fmt.Sprintf("CASE-%s-%s", now.Format("20060102"), strings.ToUpper(hex.EncodeToString(b[:])))
}

func writeJSON(path string, v any, mode os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data, mode)
}

func writeFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replaceFile(tmpName, path)
}

// replaceFile renames tmp over path, falling back to remove-and-rename on
// platforms where os.Rename does not replace an existing file.
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

func readme(w *Workspace, env model.Environment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Case %s — %s\n\n", w.Case.CaseID, w.Case.Name)
	fmt.Fprintf(&b, "This directory is a **Stratum** evidence workspace. Keep `%s`, `%s`,\n", EvidenceFile, CustodyFile)
	fmt.Fprintf(&b, "`%s/`, `%s/`, `%s/` and this file together. Do not edit the JSONL ledgers.\n\n", ItemsDir, MetadataDir, ReportsDir)
	fmt.Fprintf(&b, "| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(&b, "| Case ID | `%s` |\n", w.Case.CaseID)
	fmt.Fprintf(&b, "| Case name | %s |\n", w.Case.Name)
	fmt.Fprintf(&b, "| Examiner | %s |\n", w.Case.Examiner.Display())
	fmt.Fprintf(&b, "| Created (UTC) | %s |\n", w.Case.Created)
	fmt.Fprintf(&b, "| Created on | %s (%s/%s) |\n", env.Hostname, env.OS, env.Arch)
	fmt.Fprintf(&b, "| Tool | Stratum %s |\n\n", model.Version)
	if w.Case.Description != "" {
		fmt.Fprintf(&b, "**Description:** %s\n\n", w.Case.Description)
	}
	b.WriteString("## Handling rules\n\n")
	b.WriteString("1. Acquire from copies or read-only media where possible; Stratum never writes to source files, but reading can update access times.\n")
	b.WriteString("2. Every preserved item is copied into `items/`, read back, SHA-256 hashed and set read-only.\n")
	b.WriteString("3. Extracted EXIF/document metadata is written to `metadata/` as a JSON sidecar; the sidecar's SHA-256 is recorded in the evidence chain.\n")
	b.WriteString("4. Every acquisition, seal, unseal and verification is appended to the hash-chained `evidence.jsonl`.\n")
	b.WriteString("5. Every collection and handover is appended to the hash-chained `custody.jsonl`.\n")
	b.WriteString("6. Seal the case when the evidence set is complete. Record any later transfer in the custody ledger.\n")
	b.WriteString("7. Verify before relying on a report, and anchor the root hashes below outside this workspace.\n\n")
	b.WriteString("## Commands\n\n")
	b.WriteString("```sh\n")
	b.WriteString("stratum add <path>... --origin \"where it came from\"\n")
	b.WriteString("stratum list\n")
	b.WriteString("stratum custody transfer --to \"Recipient\" --purpose \"production to counsel\"\n")
	b.WriteString("stratum seal --note \"evidence set closed\"\n")
	b.WriteString("stratum report\n")
	b.WriteString("stratum verify\n")
	b.WriteString("```\n\n")
	b.WriteString("Run `stratum help` for all commands. Treat reports as sensitive: they contain source paths and may contain EXIF/GPS metadata.\n")
	return b.String()
}
