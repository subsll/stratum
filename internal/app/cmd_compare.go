package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/compare"
	"github.com/subsll/stratum/internal/workspace"
)

type compareOutput struct {
	Workspace  string          `json:"workspace,omitempty"`
	CaseID     string          `json:"case_id,omitempty"`
	CaseName   string          `json:"case_name,omitempty"`
	Evidence   string          `json:"evidence"`
	File       string          `json:"file,omitempty"`
	Size       int64           `json:"size,omitempty"`
	SHA256     string          `json:"sha256"`
	RootSHA256 string          `json:"root_sha256,omitempty"`
	Records    int             `json:"records"`
	ChainOK    bool            `json:"chain_ok"`
	Sidecar    string          `json:"sidecar,omitempty"`
	Captured   bool            `json:"captured"`
	Matches    []compare.Match `json:"matches"`
}

func (a *App) cmdCompare(args []string) int {
	fs := a.newFlagSet("compare", "stratum compare <file> [--workspace CASE | --evidence evidence.jsonl] [--json]")
	var workspacePath, evidencePath, digest string
	var asJSON bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.StringVar(&evidencePath, "evidence", "", "standalone evidence.jsonl")
	fs.StringVar(&digest, "hash", "", "compare this SHA-256 instead of hashing a file")
	fs.BoolVar(&asJSON, "json", false, "machine-readable result")

	pos, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if evidencePath != "" && workspacePath != "" {
		fmt.Fprintln(a.Err, "stratum: use either --workspace or --evidence, not both")
		return 2
	}
	if digest == "" {
		if len(pos) != 1 {
			fmt.Fprintln(a.Err, "stratum: compare needs exactly one file (or --hash)")
			fs.Usage()
			return 2
		}
	} else if len(pos) != 0 {
		fmt.Fprintln(a.Err, "stratum: use either a file or --hash, not both")
		return 2
	}

	var file string
	var size int64
	if digest == "" {
		file = pos[0]
		sum, n, err := compare.FileSHA256(file)
		if err != nil {
			return a.compareFail("%v", err)
		}
		digest = sum
		size = n
		if abs, err := filepath.Abs(file); err == nil {
			file = abs
		}
	} else {
		var err error
		digest, err = compare.NormalizeDigest(digest)
		if err != nil {
			return a.compareFail("%v", err)
		}
	}

	var ws *workspace.Workspace
	if evidencePath == "" {
		var code int
		ws, code = a.openWorkspace(workspacePath)
		if code != 0 {
			return 2
		}
		evidencePath = ws.EvidencePath
	} else {
		if !strings.HasSuffix(strings.ToLower(evidencePath), ".jsonl") {
			return a.compareFail("--evidence needs a .jsonl ledger")
		}
	}

	loaded, err := chain.Load(evidencePath)
	if err != nil {
		return a.compareFail("%v", err)
	}
	if !loaded.Report.OK {
		fmt.Fprintln(a.Err, "stratum: evidence chain BROKEN — refusing to compare")
		for i, e := range loaded.Report.Errors {
			if i >= 10 {
				fmt.Fprintf(a.Err, "  ... and %d more\n", len(loaded.Report.Errors)-i)
				break
			}
			fmt.Fprintf(a.Err, "  - %s\n", e)
		}
		return 2
	}

	sidecarMsg := ""
	if data, err := os.ReadFile(evidencePath + ".sha256"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			if !strings.EqualFold(fields[0], loaded.Report.FileSHA256) {
				fmt.Fprintf(a.Err, "stratum: evidence sidecar MISMATCH: file says %s\n", fields[0])
				return 2
			}
			sidecarMsg = "matches " + filepath.Base(evidencePath+".sha256")
		}
	}

	matches, err := compare.Find(loaded.Records, digest)
	if err != nil {
		return a.compareFail("%v", err)
	}
	if matches == nil {
		matches = []compare.Match{}
	}
	captured := len(matches) > 0

	if asJSON {
		out := compareOutput{
			Evidence:   evidencePath,
			File:       file,
			Size:       size,
			SHA256:     digest,
			RootSHA256: loaded.Report.RootSHA256,
			Records:    loaded.Report.Records,
			ChainOK:    true,
			Sidecar:    sidecarMsg,
			Captured:   captured,
			Matches:    matches,
		}
		if ws != nil {
			out.Workspace = ws.Root
			out.CaseID = ws.Case.CaseID
			out.CaseName = ws.Case.Name
		}
		enc := json.NewEncoder(a.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			return a.compareFail("%v", err)
		}
	} else {
		if ws != nil {
			fmt.Fprintf(a.Out, "case        %s — %s\n", ws.Case.CaseID, ws.Case.Name)
			fmt.Fprintf(a.Out, "workspace   %s\n", ws.Root)
		}
		fmt.Fprintf(a.Out, "evidence    %s\n", evidencePath)
		fmt.Fprintf(a.Out, "records     %d\n", loaded.Report.Records)
		fmt.Fprintln(a.Out, "chain       OK")
		if sidecarMsg != "" {
			fmt.Fprintf(a.Out, "sidecar     %s\n", sidecarMsg)
		}
		if file != "" {
			fmt.Fprintf(a.Out, "file        %s\n", file)
			fmt.Fprintf(a.Out, "size        %d\n", size)
		}
		fmt.Fprintf(a.Out, "sha256      %s\n", digest)
		if captured {
			fmt.Fprintln(a.Out, "result      ALREADY CAPTURED")
		} else {
			fmt.Fprintln(a.Out, "result      NEW EVIDENCE")
		}
		fmt.Fprintf(a.Out, "matches     %d\n", len(matches))
		limit := len(matches)
		if limit > 50 {
			limit = 50
		}
		for _, m := range matches[:limit] {
			if m.ItemID != "" {
				name := m.ItemName
				if name == "" {
					name = m.Target
				}
				fmt.Fprintf(a.Out, "  seq %-4d item %-8s %-30s field=%s\n", m.Seq, m.ItemID, name, m.Field)
			} else {
				fmt.Fprintf(a.Out, "  seq %-4d type=%-14s target=%s field=%s\n", m.Seq, m.Type, m.Target, m.Field)
			}
		}
		if len(matches) > limit {
			fmt.Fprintf(a.Out, "  ... %d more\n", len(matches)-limit)
		}
	}

	if captured {
		return 0
	}
	return 1
}

func (a *App) compareFail(format string, args ...any) int {
	fmt.Fprintf(a.Err, "stratum: "+format+"\n", args...)
	return 2
}
