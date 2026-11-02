// Package report renders a case as text, Markdown, standalone HTML, JSON and
// CSV, plus a manifest and SHA-256 checksums. Reports are derivative artefacts:
// generating one never modifies either ledger.
package report

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	htmltemplate "html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	texttemplate "text/template"
	"time"

	"stratum/internal/casestate"
	"stratum/internal/chain"
	"stratum/internal/model"
	"stratum/internal/verify"
	"stratum/internal/workspace"
)

// DefaultFormats are generated when --format is not supplied.
var DefaultFormats = []string{"text", "md", "html", "json", "csv"}

// Options controls report generation.
type Options struct {
	Formats     []string
	OutDir      string
	GeneratedBy model.Person
}

// Output describes generated files.
type Output struct {
	Dir      string    `json:"dir"`
	Files    []string  `json:"files"`
	Manifest *Manifest `json:"manifest,omitempty"`
}

// TypeCount is one row in a type/category breakdown.
type TypeCount struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// ItemView is an evidence item enriched for reporting.
type ItemView struct {
	model.Item
	CurrentHolder string `json:"current_holder"`
	TypeLabel     string `json:"type_label"`
	SizeHuman     string `json:"size_human"`
	ModeLabel     string `json:"mode_label"`
	EXIFSummary   string `json:"exif_summary,omitempty"`
	GPS           string `json:"gps,omitempty"`
	Duplicate     bool   `json:"duplicate"`
}

// EventView is one evidence-ledger record rendered for humans.
type EventView struct {
	Seq          int             `json:"seq"`
	Time         string          `json:"time"`
	Type         string          `json:"type"`
	Title        string          `json:"title"`
	Subject      string          `json:"subject"`
	Detail       string          `json:"detail,omitempty"`
	Source       string          `json:"source"`
	ObjectSHA256 string          `json:"object_sha256,omitempty"`
	Hash         string          `json:"hash"`
	Data         json.RawMessage `json:"data,omitempty"`
}

// CustodyView is one custody-ledger record rendered for humans.
type CustodyView struct {
	Seq          int             `json:"seq"`
	Time         string          `json:"time"`
	Action       string          `json:"action"`
	Title        string          `json:"title"`
	From         string          `json:"from"`
	To           string          `json:"to"`
	Actor        string          `json:"actor"`
	Method       string          `json:"method,omitempty"`
	Purpose      string          `json:"purpose,omitempty"`
	Location     string          `json:"location,omitempty"`
	Note         string          `json:"note,omitempty"`
	Items        []string        `json:"items,omitempty"`
	ItemsLabel   string          `json:"items_label"`
	ItemCount    int             `json:"item_count"`
	EvidenceRoot string          `json:"evidence_root"`
	Hash         string          `json:"hash"`
	Data         json.RawMessage `json:"data,omitempty"`
}

// Summary is the case-level overview used by every format.
type Summary struct {
	ItemCount             int                  `json:"item_count"`
	TotalBytes            int64                `json:"total_bytes"`
	TotalBytesHuman       string               `json:"total_bytes_human"`
	PreservedCount        int                  `json:"preserved_count"`
	ReferenceCount        int                  `json:"reference_count"`
	DuplicateCount        int                  `json:"duplicate_count"`
	EXIFCount             int                  `json:"exif_count"`
	GPSCount              int                  `json:"gps_count"`
	MetadataCount         int                  `json:"metadata_count"`
	DocumentMetadataCount int                  `json:"document_metadata_count"`
	WebCount              int                  `json:"web_count"`
	ImageCount            int                  `json:"image_count"`
	DocumentCount         int                  `json:"document_count"`
	IssueCount            int                  `json:"issue_count"`
	ErrorCount            int                  `json:"error_count"`
	EvidenceRecords       int                  `json:"evidence_records"`
	CustodyRecords        int                  `json:"custody_records"`
	ChainOK               bool                 `json:"chain_ok"`
	VerificationOK        bool                 `json:"verification_ok"`
	VerificationComplete  bool                 `json:"verification_complete"`
	Rehashed              bool                 `json:"rehashed"`
	Sealed                bool                 `json:"sealed"`
	Holders               []verify.HolderCount `json:"holders,omitempty"`
	Types                 []TypeCount          `json:"types,omitempty"`
	Categories            []TypeCount          `json:"categories,omitempty"`
}

// Data is the complete report payload. Raw records are included so the JSON
// report is a standalone audit copy.
type Data struct {
	FormatVersion  int                `json:"format_version"`
	Tool           string             `json:"tool"`
	Version        string             `json:"version"`
	GeneratedAt    string             `json:"generated_at"`
	GeneratedBy    model.Person       `json:"generated_by"`
	Formats        []string           `json:"formats"`
	Case           model.Case         `json:"case"`
	Sealed         bool               `json:"sealed"`
	SealedAt       string             `json:"sealed_at,omitempty"`
	Summary        Summary            `json:"summary"`
	Items          []ItemView         `json:"items"`
	EvidenceEvents []EventView        `json:"evidence_events"`
	CustodyEvents  []CustodyView      `json:"custody_events"`
	Issues         []model.Issue      `json:"issues,omitempty"`
	Warnings       []string           `json:"warnings,omitempty"`
	Verification   *verify.Report     `json:"verification"`
	EvidenceInfo   model.EvidenceInfo `json:"evidence"`
	CustodyInfo    model.EvidenceInfo `json:"custody"`
	EvidenceRaw    []chain.Record     `json:"evidence_records,omitempty"`
	CustodyRaw     []chain.Record     `json:"custody_records,omitempty"`
}

// Manifest is written after the report files and before SHA256SUMS.
type Manifest struct {
	FormatVersion  int                 `json:"format_version"`
	Tool           string              `json:"tool"`
	Version        string              `json:"version"`
	GeneratedAt    string              `json:"generated_at"`
	GeneratedBy    model.Person        `json:"generated_by"`
	CaseID         string              `json:"case_id"`
	Sealed         bool                `json:"sealed"`
	Evidence       model.EvidenceInfo  `json:"evidence"`
	Custody        model.EvidenceInfo  `json:"custody"`
	CaseFileSHA256 string              `json:"case_file_sha256,omitempty"`
	Verification   VerificationSummary `json:"verification"`
	Reports        map[string]string   `json:"reports"`
	Note           string              `json:"note"`
}

// VerificationSummary is the compact verification result embedded in a
// manifest.
type VerificationSummary struct {
	OK                 bool `json:"ok"`
	Complete           bool `json:"complete"`
	ChainsOK           bool `json:"chains_ok"`
	CrossLinksOK       bool `json:"cross_links_ok"`
	Rehashed           bool `json:"rehashed"`
	ItemsChecked       int  `json:"items_checked"`
	ItemsOK            int  `json:"items_ok"`
	Missing            int  `json:"missing"`
	Modified           int  `json:"modified"`
	Unexpected         int  `json:"unexpected"`
	Reference          int  `json:"reference_only"`
	MetadataChecked    int  `json:"metadata_checked"`
	MetadataOK         int  `json:"metadata_ok"`
	MetadataMissing    int  `json:"metadata_missing"`
	MetadataModified   int  `json:"metadata_modified"`
	MetadataUnexpected int  `json:"metadata_unexpected"`
}

// Build assembles the report payload.
func Build(st *casestate.State, vrep *verify.Report, opts Options) (*Data, error) {
	formats, err := NormalizeFormats(opts.Formats)
	if err != nil {
		return nil, err
	}
	generatedBy := opts.GeneratedBy
	if generatedBy.Empty() {
		generatedBy = st.Case.Examiner
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	d := &Data{
		FormatVersion: model.FormatVersion,
		Tool:          model.ToolName,
		Version:       model.Version,
		GeneratedAt:   now,
		GeneratedBy:   generatedBy,
		Formats:       formats,
		Case:          st.Case,
		Sealed:        st.Sealed,
		SealedAt:      st.SealedAt,
		Issues:        st.Issues,
		Verification:  vrep,
		EvidenceInfo:  st.EvidenceInfo(),
		CustodyInfo:   st.CustodyInfo(),
		EvidenceRaw:   st.Evidence.Records,
		CustodyRaw:    st.Custody.Records,
	}
	if vrep != nil {
		d.Warnings = append(d.Warnings, vrep.Warnings...)
	}
	d.Warnings = append(d.Warnings, st.Warnings...)
	d.Warnings = dedupe(d.Warnings)
	d.Items = buildItems(st)
	d.EvidenceEvents = buildEvidenceEvents(st.Evidence.Records)
	d.CustodyEvents = buildCustodyEvents(st.Custody.Records)
	d.Summary = buildSummary(st, vrep, d.Items)
	return d, nil
}

// NormalizeFormats validates and de-duplicates requested formats.
func NormalizeFormats(in []string) ([]string, error) {
	if len(in) == 0 {
		in = DefaultFormats
	}
	var out []string
	seen := map[string]bool{}
	for _, raw := range in {
		for _, f := range strings.Split(raw, ",") {
			f = strings.ToLower(strings.TrimSpace(f))
			if f == "" {
				continue
			}
			switch f {
			case "txt", "text":
				f = "text"
			case "markdown":
				f = "md"
			case "html", "json", "csv", "md":
			default:
				return nil, fmt.Errorf("unknown report format %q (want text, md, html, json, csv)", f)
			}
			if !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no report formats selected")
	}
	return out, nil
}

// Write renders the requested formats into a report directory.
func Write(ws *workspace.Workspace, st *casestate.State, vrep *verify.Report, opts Options) (*Output, error) {
	formats, err := NormalizeFormats(opts.Formats)
	if err != nil {
		return nil, err
	}
	opts.Formats = formats
	data, err := Build(st, vrep, opts)
	if err != nil {
		return nil, err
	}

	dir, err := reportDir(ws, opts.OutDir)
	if err != nil {
		return nil, err
	}
	if insideDir(ws.ItemsPath, dir) {
		return nil, fmt.Errorf("refusing to write reports inside the evidence directory %s", ws.ItemsPath)
	}
	if insideDir(ws.MetadataPath, dir) {
		return nil, fmt.Errorf("refusing to write reports inside the metadata directory %s", ws.MetadataPath)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}

	written := map[string]string{}
	write := func(name string, content []byte) error {
		path := filepath.Join(dir, name)
		if err := writeAtomic(path, content, 0o600); err != nil {
			return err
		}
		sum := sha256.Sum256(content)
		written[name] = hex.EncodeToString(sum[:])
		return nil
	}

	for _, f := range formats {
		switch f {
		case "text":
			var b strings.Builder
			if err := renderText(&b, data); err != nil {
				return nil, err
			}
			if err := write("report.txt", []byte(b.String())); err != nil {
				return nil, err
			}
		case "md":
			var b strings.Builder
			if err := renderMarkdown(&b, data); err != nil {
				return nil, err
			}
			if err := write("report.md", []byte(tidyMarkdown(b.String()))); err != nil {
				return nil, err
			}
		case "html":
			var b strings.Builder
			if err := renderHTML(&b, data); err != nil {
				return nil, err
			}
			if err := write("report.html", []byte(b.String())); err != nil {
				return nil, err
			}
		case "json":
			b, err := json.MarshalIndent(data, "", "  ")
			if err != nil {
				return nil, err
			}
			b = append(b, '\n')
			if err := write("report.json", b); err != nil {
				return nil, err
			}
		case "csv":
			for name, fn := range map[string]func(io.Writer, *Data) error{
				"items.csv":    writeItemsCSV,
				"metadata.csv": writeMetadataCSV,
				"evidence.csv": writeEvidenceCSV,
				"custody.csv":  writeCustodyCSV,
			} {
				var b strings.Builder
				if err := fn(&b, data); err != nil {
					return nil, err
				}
				if err := write(name, []byte(b.String())); err != nil {
					return nil, err
				}
			}
		}
	}

	manifest := &Manifest{
		FormatVersion:  model.FormatVersion,
		Tool:           model.ToolName,
		Version:        model.Version,
		GeneratedAt:    data.GeneratedAt,
		GeneratedBy:    data.GeneratedBy,
		CaseID:         data.Case.CaseID,
		Sealed:         data.Sealed,
		Evidence:       data.EvidenceInfo,
		Custody:        data.CustodyInfo,
		CaseFileSHA256: verificationCaseHash(vrep),
		Reports:        written,
		Note:           "Reports are derivative artefacts. SHA256SUMS covers every file in this directory, including this manifest.",
	}
	if vrep != nil {
		manifest.Verification = VerificationSummary{
			OK: vrep.OK, Complete: vrep.Complete, ChainsOK: vrep.ChainsOK,
			CrossLinksOK: vrep.CrossLinksOK, Rehashed: vrep.Rehashed,
			ItemsChecked: vrep.ItemCount, ItemsOK: vrep.ItemsOK,
			Missing: len(vrep.Missing), Modified: len(vrep.Modified),
			Unexpected: len(vrep.Unexpected), Reference: len(vrep.Reference),
			MetadataChecked: vrep.MetadataChecked, MetadataOK: vrep.MetadataOK,
			MetadataMissing: len(vrep.MetadataMissing), MetadataModified: len(vrep.MetadataModified),
			MetadataUnexpected: len(vrep.MetadataUnexpected),
		}
	}
	mb, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	mb = append(mb, '\n')
	if err := write("MANIFEST.json", mb); err != nil {
		return nil, err
	}

	var names []string
	for name := range written {
		names = append(names, name)
	}
	sort.Strings(names)
	var sums strings.Builder
	for _, name := range names {
		fmt.Fprintf(&sums, "%s  %s\n", written[name], name)
	}
	if err := writeAtomic(filepath.Join(dir, "SHA256SUMS"), []byte(sums.String()), 0o600); err != nil {
		return nil, err
	}
	names = append(names, "SHA256SUMS")

	out := &Output{Dir: dir, Files: names, Manifest: manifest}
	return out, nil
}

func verificationCaseHash(v *verify.Report) string {
	if v == nil {
		return ""
	}
	return v.CaseFileHash
}

func buildItems(st *casestate.State) []ItemView {
	items := make([]ItemView, 0, len(st.Items))
	for _, it := range st.Items {
		view := ItemView{
			Item:          it,
			CurrentHolder: st.Holder(it.ID),
			TypeLabel:     typeLabel(it.FileType),
			SizeHuman:     humanBytes(it.Size),
			ModeLabel:     modeLabel(it.Mode),
			Duplicate:     it.DuplicateOf != "",
		}
		view.EXIFSummary = exifSummary(it.EXIF)
		if it.EXIF.GPS != nil {
			view.GPS = gpsString(it.EXIF.GPS)
		}
		items = append(items, view)
	}
	return items
}

func buildSummary(st *casestate.State, vrep *verify.Report, items []ItemView) Summary {
	s := Summary{
		ItemCount:       len(items),
		TotalBytes:      st.TotalBytes(),
		TotalBytesHuman: humanBytes(st.TotalBytes()),
		EvidenceRecords: st.EvidenceInfo().Records,
		CustodyRecords:  st.CustodyInfo().Records,
		Sealed:          st.Sealed,
	}
	typeCounts := map[string]*TypeCount{}
	catCounts := map[string]*TypeCount{}
	for _, it := range items {
		if it.Mode == "reference" {
			s.ReferenceCount++
		} else {
			s.PreservedCount++
		}
		if it.Duplicate {
			s.DuplicateCount++
		}
		if it.Metadata.SHA256 != "" {
			s.MetadataCount++
		}
		if it.Document.Present {
			s.DocumentMetadataCount++
		}
		if it.EXIF.Present {
			s.EXIFCount++
		}
		if it.EXIF.GPS != nil {
			s.GPSCount++
		}
		switch it.FileType.Category {
		case "web":
			s.WebCount++
		case "image":
			s.ImageCount++
		case "document":
			s.DocumentCount++
		}
		id := it.FileType.ID
		if id == "" {
			id = "unknown"
		}
		if typeCounts[id] == nil {
			label := it.FileType.Description
			if label == "" {
				label = id
			}
			typeCounts[id] = &TypeCount{ID: id, Label: label}
		}
		typeCounts[id].Count++
		cat := it.FileType.Category
		if cat == "" {
			cat = "other"
		}
		if catCounts[cat] == nil {
			catCounts[cat] = &TypeCount{ID: cat, Label: titleCase(cat)}
		}
		catCounts[cat].Count++
	}
	s.Types = sortedCounts(typeCounts)
	s.Categories = sortedCounts(catCounts)
	s.IssueCount = len(st.Issues)
	for _, issue := range st.Issues {
		if issue.Kind == "error" {
			s.ErrorCount++
		}
	}
	if vrep != nil {
		s.ChainOK = vrep.ChainsOK
		s.VerificationOK = vrep.OK
		s.VerificationComplete = vrep.Complete
		s.Rehashed = vrep.Rehashed
		s.Holders = vrep.HolderCounts
	}
	return s
}

func sortedCounts(m map[string]*TypeCount) []TypeCount {
	out := make([]TypeCount, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Label < out[j].Label
		}
		return out[i].Count > out[j].Count
	})
	return out
}

func buildEvidenceEvents(records []chain.Record) []EventView {
	out := make([]EventView, 0, len(records))
	for _, rec := range records {
		ev := EventView{Seq: rec.Seq, Time: rec.Time, Type: rec.Type, Source: rec.Source, Hash: rec.Hash, Data: rec.Data}
		switch rec.Type {
		case "case_created":
			var d model.CaseCreatedData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Case opened"
				ev.Subject = d.CaseID + " — " + d.Name
				ev.Detail = "examiner: " + d.Examiner.Display()
			}
		case "acquisition_started":
			var d model.AcquisitionStartedData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Acquisition started"
				ev.Subject = d.Operator.Display()
				ev.Detail = fmt.Sprintf("%d source path(s); mode %s", len(d.Paths), copyMode(d.Options.Copy))
			}
		case "item_acquired":
			var it model.Item
			if json.Unmarshal(rec.Data, &it) == nil {
				ev.Title = "Item acquired"
				ev.Subject = it.ID + " — " + it.Name
				ev.Detail = fmt.Sprintf("%s; %s; %s", typeLabel(it.FileType), humanBytes(it.Size), modeLabel(it.Mode))
				ev.ObjectSHA256 = it.SHA256
			}
		case "item_metadata":
			var d model.ItemMetadataData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Metadata extracted"
				ev.Subject = d.ItemID
				ev.Detail = fmt.Sprintf("%s; %d bytes; exif=%t document=%t", d.Metadata.Path, d.Metadata.Size, d.EXIF.Present, d.Document.Present)
				ev.ObjectSHA256 = d.Metadata.SHA256
			}
		case "item_error":
			var d model.ItemErrorData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Acquisition issue"
				ev.Subject = d.Path
				ev.Detail = d.Kind + ": " + d.Reason
			}
		case "acquisition_finished":
			var d model.AcquisitionFinishedData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Acquisition finished"
				ev.Subject = fmt.Sprintf("%d item(s), %s", d.ItemsAdded, humanBytes(d.Bytes))
				ev.Detail = fmt.Sprintf("%d issue(s), %d duplicate(s)", d.Issues, d.Duplicates)
			}
		case "case_sealed":
			var d model.CaseSealedData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Case sealed"
				ev.Subject = d.Operator.Display()
				ev.Detail = fmt.Sprintf("%d item(s), %s", d.ItemCount, humanBytes(d.Bytes))
				if d.Note != "" {
					ev.Detail += "; " + d.Note
				}
			}
		case "case_unsealed":
			var d model.CaseUnsealedData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Case unsealed"
				ev.Subject = d.Operator.Display()
				ev.Detail = d.Reason
			}
		case "verification":
			var d model.VerificationData
			if json.Unmarshal(rec.Data, &d) == nil {
				ev.Title = "Verification recorded"
				ev.Subject = d.Operator.Display()
				ev.Detail = fmt.Sprintf("ok=%t complete=%t items=%d", d.OK, d.Complete, d.ItemsChecked)
			}
		default:
			ev.Title = rec.Type
			ev.Subject = rec.Target
		}
		out = append(out, ev)
	}
	return out
}

func buildCustodyEvents(records []chain.Record) []CustodyView {
	out := make([]CustodyView, 0, len(records))
	for _, rec := range records {
		if !strings.HasPrefix(rec.Type, "custody_") {
			continue
		}
		var d model.CustodyData
		if json.Unmarshal(rec.Data, &d) != nil {
			continue
		}
		action := d.Action
		if action == "" {
			action = strings.TrimPrefix(rec.Type, "custody_")
		}
		itemsLabel := "all items in scope"
		if len(d.Items) > 0 && !(len(d.Items) == 1 && d.Items[0] == "*") {
			itemsLabel = strings.Join(d.Items, ", ")
		}
		to := d.To.Display()
		if to == "" {
			to = "— (case-level record)"
		}
		out = append(out, CustodyView{
			Seq: rec.Seq, Time: rec.Time, Action: action, Title: custodyTitle(action),
			From: d.From, To: to, Actor: d.Actor.Display(),
			Method: d.Method, Purpose: d.Purpose, Location: d.Location, Note: d.Note,
			Items: d.Items, ItemsLabel: itemsLabel, ItemCount: d.ItemCount,
			EvidenceRoot: d.EvidenceRoot, Hash: rec.Hash, Data: rec.Data,
		})
	}
	return out
}

func titleCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(strings.ToLower(p))
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

func custodyTitle(action string) string {
	switch action {
	case "opened":
		return "Case opened"
	case "collected":
		return "Items collected"
	case "transferred":
		return "Custody transferred"
	case "returned":
		return "Custody returned"
	case "sealed":
		return "Case sealed"
	case "unsealed":
		return "Seal broken"
	case "verified":
		return "Verification recorded"
	case "metadata":
		return "Metadata extracted"
	default:
		return titleCase(action)
	}
}

func typeLabel(ft model.FileType) string {
	if ft.Description == "" && ft.MIME == "" {
		return "unknown"
	}
	if ft.MIME == "" {
		return ft.Description
	}
	if ft.Description == "" {
		return ft.MIME
	}
	return ft.Description + " · " + ft.MIME
}

func modeLabel(mode string) string {
	if mode == "preserved" {
		return "preserved copy"
	}
	return "reference only"
}

func copyMode(copy bool) string {
	if copy {
		return "preserved copy"
	}
	return "reference only"
}

func exifSummary(e model.EXIF) string {
	if e.Present {
		var parts []string
		if e.Make != "" || e.Model != "" {
			parts = append(parts, strings.TrimSpace(e.Make+" "+e.Model))
		}
		if e.DateTimeOriginal != "" {
			parts = append(parts, e.DateTimeOriginal)
		}
		if e.Serial != "" {
			parts = append(parts, "serial "+e.Serial)
		}
		if e.GPS != nil {
			parts = append(parts, "GPS "+gpsString(e.GPS))
		}
		if len(parts) == 0 {
			parts = append(parts, "EXIF block present")
		}
		return strings.Join(parts, "; ")
	}
	if e.Capable {
		return "EXIF-capable container (not extracted)"
	}
	if e.XMP {
		return "XMP metadata present"
	}
	return ""
}

func gpsString(g *model.GPS) string {
	if g == nil {
		return ""
	}
	out := g.Latitude + ", " + g.Longitude
	if g.Altitude != "" {
		out += " · " + g.Altitude + " m"
	}
	return out
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), units[exp])
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func reportDir(ws *workspace.Workspace, out string) (string, error) {
	if strings.TrimSpace(out) != "" {
		abs, err := filepath.Abs(out)
		if err != nil {
			return "", err
		}
		return filepath.Clean(abs), nil
	}
	base := time.Now().UTC().Format("20060102-150405Z")
	dir := filepath.Join(ws.ReportsPath, base)
	for i := 1; ; i++ {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return dir, nil
		}
		dir = filepath.Join(ws.ReportsPath, fmt.Sprintf("%s-%02d", base, i))
		if i > 99 {
			return filepath.Join(ws.ReportsPath, fmt.Sprintf("%s-%d", base, time.Now().UnixNano())), nil
		}
	}
}

func insideDir(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
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
	return replaceFile(name, path)
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

func renderText(w io.Writer, d *Data) error {
	t, err := texttemplate.New("text").Funcs(textFuncs()).Parse(textTemplate)
	if err != nil {
		return err
	}
	return t.Execute(w, d)
}

func renderMarkdown(w io.Writer, d *Data) error {
	t, err := texttemplate.New("md").Funcs(textFuncs()).Parse(mdTemplate)
	if err != nil {
		return err
	}
	return t.Execute(w, d)
}

func renderHTML(w io.Writer, d *Data) error {
	t, err := htmltemplate.New("html").Funcs(htmlFuncs()).Parse(htmlTemplate)
	if err != nil {
		return err
	}
	return t.Execute(w, d)
}

func textFuncs() texttemplate.FuncMap {
	return texttemplate.FuncMap{
		"datetime": formatTime,
		"short":    shortHash,
		"join":     strings.Join,
		"dash":     dash,
		"md":       mdEscape,
		"oneLine":  oneLine,
		"eq":       func(a, b string) bool { return a == b },
		"lower":    strings.ToLower,
	}
}

func htmlFuncs() htmltemplate.FuncMap {
	return htmltemplate.FuncMap{
		"datetime": formatTime,
		"short":    shortHash,
		"join":     strings.Join,
		"dash":     dash,
		"oneLine":  oneLine,
		"eq":       func(a, b string) bool { return a == b },
		"lower":    strings.ToLower,
	}
}

func formatTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func shortHash(s string) string {
	if len(s) <= 20 {
		return s
	}
	return s[:20] + "…"
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func mdEscape(s string) string {
	// Markdown permits raw HTML. Escape evidence-derived text so a crafted file
	// name or note cannot inject markup into a rendered report.
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", "<br>")
	s = strings.ReplaceAll(s, "\n", "<br>")
	return s
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func tidyMarkdown(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimLeft(s, "\n")
}

// writeCSV is a small shared helper.
func writeCSV(w io.Writer, rows [][]string) error {
	cw := csv.NewWriter(w)
	for _, row := range rows {
		safe := make([]string, len(row))
		for i, cell := range row {
			safe[i] = csvSafe(cell)
		}
		if err := cw.Write(safe); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvSafe prevents a spreadsheet from interpreting evidence-derived text as a
// formula. It leaves ordinary negative numbers alone.
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
