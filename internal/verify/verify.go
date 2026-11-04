// Package verify checks both ledgers, the cross-links between them, and (when
// requested) re-hashes every preserved item. It is read-only.
package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/subsll/stratum/internal/casestate"
	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/metadata"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

// ItemCheck is the result of checking one item.
type ItemCheck struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Mode           string `json:"mode"`
	StoredPath     string `json:"stored_path,omitempty"`
	SourcePath     string `json:"source_path"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ActualSHA256   string `json:"actual_sha256,omitempty"`
	Size           int64  `json:"size"`
	Status         string `json:"status"`
	Note           string `json:"note,omitempty"`
}

// Report is the full integrity result.
type Report struct {
	CheckedAt          string        `json:"checked_at"`
	Rehashed           bool          `json:"rehashed"`
	OK                 bool          `json:"ok"`
	Complete           bool          `json:"complete"`
	ChainsOK           bool          `json:"chains_ok"`
	CrossLinksOK       bool          `json:"cross_links_ok"`
	Evidence           chain.Report  `json:"evidence"`
	Custody            chain.Report  `json:"custody"`
	CaseFileHash       string        `json:"case_file_sha256,omitempty"`
	Items              []ItemCheck   `json:"items,omitempty"`
	Missing            []string      `json:"missing,omitempty"`
	Modified           []ItemCheck   `json:"modified,omitempty"`
	Unexpected         []string      `json:"unexpected,omitempty"`
	Reference          []ItemCheck   `json:"reference_only,omitempty"`
	MetadataChecked    int           `json:"metadata_checked"`
	MetadataOK         int           `json:"metadata_ok"`
	MetadataMissing    []string      `json:"metadata_missing,omitempty"`
	MetadataModified   []ItemCheck   `json:"metadata_modified,omitempty"`
	MetadataUnexpected []string      `json:"metadata_unexpected,omitempty"`
	Warnings           []string      `json:"warnings,omitempty"`
	Errors             []string      `json:"errors,omitempty"`
	ItemCount          int           `json:"item_count"`
	ItemsOK            int           `json:"items_ok"`
	Sealed             bool          `json:"sealed"`
	SealedAt           string        `json:"sealed_at,omitempty"`
	HolderCounts       []HolderCount `json:"holder_counts,omitempty"`
}

// HolderCount groups items by current custodian.
type HolderCount struct {
	Holder string `json:"holder"`
	Count  int    `json:"count"`
}

// Run verifies a loaded case. rehash controls whether item bytes are read and
// compared again; chain verification always runs.
func Run(ws *workspace.Workspace, st *casestate.State, rehash bool) (*Report, error) {
	r := &Report{
		CheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Rehashed:  rehash,
		OK:        true,
		Sealed:    st.Sealed,
		SealedAt:  st.SealedAt,
		ItemCount: len(st.Items),
	}
	if st.Evidence != nil && st.Evidence.Report != nil {
		r.Evidence = *st.Evidence.Report
	}
	if st.Custody != nil && st.Custody.Report != nil {
		r.Custody = *st.Custody.Report
	}
	r.ChainsOK = r.Evidence.OK && r.Custody.OK
	if !r.ChainsOK {
		r.OK = false
		for _, e := range r.Evidence.Errors {
			r.Errors = append(r.Errors, "evidence chain: "+e)
		}
		for _, e := range r.Custody.Errors {
			r.Errors = append(r.Errors, "custody chain: "+e)
		}
	}

	if len(st.Items) != len(st.ItemIndex) {
		r.Errors = append(r.Errors, "evidence ledger contains duplicate item IDs")
		r.OK = false
	}
	checkCaseShape(st, r)
	checkCrossLinks(st, r)
	if h, _, err := chain.HashFile(ws.CasePath); err == nil {
		r.CaseFileHash = h
	} else {
		r.Warnings = append(r.Warnings, "case.json could not be hashed: "+err.Error())
	}
	for _, h := range st.Ledger.HolderCounts() {
		r.HolderCounts = append(r.HolderCounts, HolderCount{Holder: h.Holder, Count: h.Count})
	}

	if info, err := os.Lstat(ws.ItemsPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		r.Errors = append(r.Errors, "items/ is a symbolic link; preserved evidence must live in a real directory")
		r.OK = false
	}
	expected := make(map[string]bool, len(st.Items))
	for _, it := range st.Items {
		check := ItemCheck{
			ID:             it.ID,
			Name:           it.Name,
			Mode:           it.Mode,
			StoredPath:     it.StoredPath,
			SourcePath:     it.SourcePath,
			ExpectedSHA256: it.SHA256,
			Size:           it.Size,
			Status:         "not-checked",
		}
		if it.StoredPath != "" {
			expected[filepath.Clean(filepath.FromSlash(it.StoredPath))] = true
		}
		if !rehash {
			check.Status = "not-rehashed"
			r.Items = append(r.Items, check)
			continue
		}
		switch it.Mode {
		case "preserved":
			if it.StoredPath == "" {
				check.Status = "missing"
				check.Note = "preserved item has no stored path"
				r.Missing = append(r.Missing, it.ID)
				r.OK = false
				break
			}
			full, err := ws.ResolveStored(it.StoredPath)
			if err != nil {
				check.Status = "unreadable"
				check.Note = err.Error()
				r.Errors = append(r.Errors, it.ID+": "+err.Error())
				r.OK = false
				break
			}
			linfo, err := os.Lstat(full)
			if err != nil {
				check.Status = "missing"
				check.Note = err.Error()
				r.Missing = append(r.Missing, it.ID)
				r.OK = false
				break
			}
			if linfo.Mode()&os.ModeSymlink != 0 {
				check.Status = "modified"
				check.Note = "preserved path is a symbolic link, not a regular evidence file"
				r.Modified = append(r.Modified, check)
				r.Errors = append(r.Errors, it.ID+": preserved path is a symbolic link")
				r.OK = false
				break
			}
			info, err := os.Stat(full)
			if err != nil {
				check.Status = "missing"
				check.Note = err.Error()
				r.Missing = append(r.Missing, it.ID)
				r.OK = false
				break
			}
			if info.Mode()&0o222 != 0 {
				r.Warnings = append(r.Warnings, fmt.Sprintf("%s preserved copy is writable (%s); expected read-only", it.ID, info.Mode()))
			}
			actual, size, err := chain.HashFile(full)
			check.ActualSHA256 = actual
			if err != nil {
				check.Status = "unreadable"
				check.Note = err.Error()
				r.Errors = append(r.Errors, it.ID+": "+err.Error())
				r.OK = false
				break
			}
			if size != it.Size || actual != it.SHA256 {
				check.Status = "modified"
				check.Note = fmt.Sprintf("expected %d bytes, found %d", it.Size, size)
				r.Modified = append(r.Modified, check)
				r.OK = false
				break
			}
			check.Status = "verified"
			r.ItemsOK++
		case "reference":
			check.Status = "reference"
			r.Complete = false
			r.Warnings = append(r.Warnings, it.ID+" is reference-only; no preserved copy exists to re-verify")
			if _, err := os.Stat(it.SourcePath); err != nil {
				check.Note = "source is no longer available: " + err.Error()
				r.Warnings = append(r.Warnings, it.ID+" source is no longer available: "+err.Error())
			} else if actual, _, err := chain.HashFile(it.SourcePath); err != nil {
				check.Note = "source could not be re-hashed: " + err.Error()
				r.Warnings = append(r.Warnings, check.Note)
			} else if actual != it.SHA256 {
				check.ActualSHA256 = actual
				check.Note = "source bytes have changed since acquisition"
				r.Warnings = append(r.Warnings, it.ID+" source bytes have changed since acquisition")
			} else {
				check.ActualSHA256 = actual
				check.Note = "source still matches the acquisition hash"
			}
			r.Reference = append(r.Reference, check)
		default:
			check.Status = "unreadable"
			check.Note = "unknown item mode " + it.Mode
			r.Errors = append(r.Errors, it.ID+": unknown item mode "+it.Mode)
			r.OK = false
		}
		r.Items = append(r.Items, check)
	}
	if rehash && len(r.Reference) == 0 {
		r.Complete = true
	}

	if rehash {
		checkMetadata(ws, st, r)
		checkUnexpected(ws, expected, r)
	}
	if st.Sealed {
		checkSealCustody(st, r)
	}
	if len(st.Warnings) > 0 {
		r.Warnings = append(r.Warnings, st.Warnings...)
	}
	return r, nil
}

func checkCaseShape(st *casestate.State, r *Report) {
	if len(st.Evidence.Records) > 0 && st.Evidence.Records[0].Type != "case_created" {
		r.Errors = append(r.Errors, "first evidence record is not case_created")
		r.OK = false
	}
	caseRecords := 0
	for _, rec := range st.Evidence.Records {
		if rec.Type == "case_created" {
			caseRecords++
		}
	}
	if caseRecords != 1 {
		r.Errors = append(r.Errors, fmt.Sprintf("evidence ledger has %d case_created record(s), want exactly 1", caseRecords))
		r.OK = false
	}
	if len(st.Custody.Records) > 0 && st.Custody.Records[0].Type != "custody_open" {
		r.Errors = append(r.Errors, "first custody record is not custody_open")
		r.OK = false
	}
	openRecords := 0
	for _, rec := range st.Custody.Records {
		if rec.Type == "custody_open" {
			openRecords++
		}
	}
	if openRecords != 1 {
		r.Errors = append(r.Errors, fmt.Sprintf("custody ledger has %d custody_open record(s), want exactly 1", openRecords))
		r.OK = false
	}
}

func checkCrossLinks(st *casestate.State, r *Report) {
	rootTime := make(map[string]string, len(st.Evidence.Records))
	rootSeen := make(map[string]bool, len(st.Evidence.Records))
	for _, rec := range st.Evidence.Records {
		rootTime[rec.Hash] = rec.Time
		rootSeen[rec.Hash] = true
	}
	crossOK := true
	for _, rec := range st.Custody.Records {
		if !strings.HasPrefix(rec.Type, "custody_") {
			continue
		}
		var data model.CustodyData
		if err := json.Unmarshal(rec.Data, &data); err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("custody record %d cannot be read: %v", rec.Seq, err))
			crossOK = false
			continue
		}
		if data.EvidenceRoot == "" {
			r.Errors = append(r.Errors, fmt.Sprintf("custody record %d has no evidence anchor", rec.Seq))
			crossOK = false
		} else if !rootSeen[data.EvidenceRoot] {
			r.Errors = append(r.Errors, fmt.Sprintf("custody record %d anchors an evidence root that is not in the evidence chain (%s)", rec.Seq, short(data.EvidenceRoot)))
			crossOK = false
		} else if when, err := time.Parse(time.RFC3339Nano, rootTime[data.EvidenceRoot]); err == nil {
			if evTime, err := time.Parse(time.RFC3339Nano, rec.Time); err == nil && when.After(evTime.Add(5*time.Minute)) {
				r.Warnings = append(r.Warnings, fmt.Sprintf("custody record %d is timestamped before the evidence state it anchors", rec.Seq))
			}
		}
		if len(data.Items) > 0 {
			for _, id := range data.Items {
				if id == "*" {
					continue
				}
				if _, ok := st.Item(id); !ok {
					r.Errors = append(r.Errors, fmt.Sprintf("custody record %d references unknown item %s", rec.Seq, id))
					crossOK = false
				}
			}
		}
	}
	for _, it := range st.Items {
		history := st.Ledger.History(it)
		covered := false
		for _, ev := range history {
			switch ev.Data.Action {
			case "collected", "transferred", "returned", "opened":
				covered = true
			}
			if covered {
				break
			}
		}
		if !covered {
			r.Errors = append(r.Errors, fmt.Sprintf("item %s has no custody collection event", it.ID))
			crossOK = false
		}
		if st.Ledger.HolderByItem[it.ID] == "" {
			r.Errors = append(r.Errors, fmt.Sprintf("item %s has no current custodian", it.ID))
			crossOK = false
		}
	}
	r.CrossLinksOK = crossOK
	if !crossOK {
		r.OK = false
	}
}

func checkMetadata(ws *workspace.Workspace, st *casestate.State, r *Report) {
	if info, err := os.Lstat(ws.MetadataPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		r.Errors = append(r.Errors, "metadata/ is a symbolic link; sidecars must live in a real directory")
		r.OK = false
		return
	}
	expected := make(map[string]bool, len(st.Items))
	for _, it := range st.Items {
		if it.Metadata.Path == "" {
			r.MetadataMissing = append(r.MetadataMissing, it.ID)
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s has no metadata sidecar (run: stratum extract %s)", it.ID, it.ID))
			r.Complete = false
			continue
		}
		expected[filepath.Clean(filepath.FromSlash(it.Metadata.Path))] = true
		r.MetadataChecked++
		if _, err := metadata.Verify(ws, it); err != nil {
			if os.IsNotExist(err) {
				r.MetadataMissing = append(r.MetadataMissing, it.ID)
				r.Warnings = append(r.Warnings, fmt.Sprintf("%s metadata sidecar is missing: %v", it.ID, err))
				r.Complete = false
			} else {
				check := ItemCheck{ID: it.ID, Name: it.Name, Mode: it.Mode, StoredPath: it.Metadata.Path, ExpectedSHA256: it.Metadata.SHA256, Status: "metadata-modified", Note: err.Error()}
				r.MetadataModified = append(r.MetadataModified, check)
				r.Errors = append(r.Errors, fmt.Sprintf("%s metadata: %v", it.ID, err))
			}
			r.OK = false
			continue
		}
		r.MetadataOK++
	}
	checkUnexpectedMetadata(ws, expected, r)
}

func checkUnexpectedMetadata(ws *workspace.Workspace, expected map[string]bool, r *Report) {
	entries, err := os.ReadDir(ws.MetadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		r.Warnings = append(r.Warnings, "metadata directory could not be listed: "+err.Error())
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".DS_Store" || name == "Thumbs.db" || name == "desktop.ini" {
			r.Warnings = append(r.Warnings, "ignored operating-system metadata in metadata/: "+name)
			continue
		}
		full := filepath.Join(ws.MetadataPath, name)
		rel, _ := filepath.Rel(ws.Root, full)
		rel = filepath.Clean(rel)
		if e.IsDir() {
			r.MetadataUnexpected = append(r.MetadataUnexpected, filepath.ToSlash(rel)+"/")
			r.Errors = append(r.Errors, "unrecorded directory in metadata/: "+filepath.ToSlash(rel))
			r.OK = false
			continue
		}
		if !expected[rel] {
			r.MetadataUnexpected = append(r.MetadataUnexpected, filepath.ToSlash(rel))
			if strings.HasPrefix(name, ".incoming-meta-") {
				r.Warnings = append(r.Warnings, "orphan temporary metadata file: "+filepath.ToSlash(rel))
			} else {
				r.Errors = append(r.Errors, "unrecorded file in metadata/: "+filepath.ToSlash(rel))
				r.OK = false
			}
		}
	}
}

func checkUnexpected(ws *workspace.Workspace, expected map[string]bool, r *Report) {
	entries, err := os.ReadDir(ws.ItemsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		r.Warnings = append(r.Warnings, "items directory could not be listed: "+err.Error())
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".DS_Store" || name == "Thumbs.db" || name == "desktop.ini" {
			r.Warnings = append(r.Warnings, "ignored operating-system metadata in items/: "+name)
			continue
		}
		full := filepath.Join(ws.ItemsPath, name)
		rel, _ := filepath.Rel(ws.Root, full)
		rel = filepath.Clean(rel)
		if e.IsDir() {
			r.Unexpected = append(r.Unexpected, filepath.ToSlash(rel)+"/")
			r.Errors = append(r.Errors, "unrecorded directory in items/: "+filepath.ToSlash(rel))
			r.OK = false
			continue
		}
		if !expected[rel] {
			r.Unexpected = append(r.Unexpected, filepath.ToSlash(rel))
			if strings.HasPrefix(name, ".incoming-") {
				r.Warnings = append(r.Warnings, "orphan temporary file from an interrupted acquisition: "+filepath.ToSlash(rel))
			} else {
				r.Errors = append(r.Errors, "unrecorded file in items/: "+filepath.ToSlash(rel))
				r.OK = false
			}
		}
	}
}

func checkSealCustody(st *casestate.State, r *Report) {
	found := false
	for _, ev := range st.Ledger.Events {
		if ev.Data.Action == "sealed" {
			found = true
		}
	}
	if !found {
		r.Warnings = append(r.Warnings, "case is sealed in the evidence ledger but has no custody seal event")
	}
}

func short(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "..."
}
