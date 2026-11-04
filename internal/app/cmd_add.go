package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/subsll/stratum/internal/acquire"
	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/logx"
	"github.com/subsll/stratum/internal/metadata"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

func (a *App) cmdAdd(args []string) int {
	fs := a.newFlagSet("add", "stratum add <path>... [flags]")
	var workspacePath, origin, note string
	var reference, copyFiles, skipHidden, verbose, forceLock bool
	var excludes stringList
	copyFiles = true
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.BoolVar(&reference, "reference", false, "record a hash only; do not preserve a copy")
	fs.BoolVar(&copyFiles, "copy", true, "copy files into the workspace (default true)")
	fs.Var(&excludes, "exclude", "exclude a shell-style glob (repeatable)")
	fs.StringVar(&origin, "origin", "", "provenance note stored with each item")
	fs.StringVar(&note, "note", "", "free-text note stored with each item")
	fs.BoolVar(&skipHidden, "skip-hidden", false, "skip dot-files and dot-directories")
	fs.BoolVar(&verbose, "v", false, "per-file detail")
	fs.BoolVar(&verbose, "verbose", false, "per-file detail")
	fs.BoolVar(&forceLock, "force-lock", false, "break a workspace lock after confirming it is stale")
	p := a.addPersonFlags(fs, false)

	pos, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if len(pos) == 0 {
		fmt.Fprintln(a.Err, "stratum: at least one source path is required")
		fs.Usage()
		return 2
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	operator, err := p.Person(ws.Case.Examiner, false)
	if err != nil {
		return a.fail("%v", err)
	}
	log := logx.New(a.Out, verbose)
	res, err := acquire.Run(ws, acquire.Request{
		Paths: pos,
		Options: acquire.Options{
			Copy:       copyFiles && !reference,
			Exclude:    excludes.values,
			Origin:     origin,
			Note:       note,
			SkipHidden: skipHidden,
		},
		Operator:  operator,
		ForceLock: forceLock,
		Log:       log,
	})
	if err != nil {
		if errors.Is(err, acquire.ErrSealed) {
			fmt.Fprintf(a.Err, "stratum: %v\n", err)
			return 1
		}
		return a.fail("%v", err)
	}
	fmt.Fprintln(a.Out, "[+] acquisition complete")
	fmt.Fprintf(a.Out, "    items      %d (%s)", len(res.Items), humanBytes(res.Bytes))
	if res.Duplicates > 0 {
		fmt.Fprintf(a.Out, ", %d duplicate(s)", res.Duplicates)
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintf(a.Out, "    issues     %d (%d error(s))\n", len(res.Issues), res.Errors)
	fmt.Fprintf(a.Out, "    evidence   %d record(s), root %s\n", res.EvidenceInfo.Records, short(res.EvidenceInfo.RootSHA256))
	if res.CustodyRecorded {
		fmt.Fprintf(a.Out, "    custody    collection recorded, root %s\n", short(res.CustodyInfo.RootSHA256))
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(a.Out, "    no regular files were acquired")
	}
	if res.Errors > 0 {
		fmt.Fprintln(a.Err, "stratum: acquisition completed with errors; review `stratum list` or the report's issue section")
		return 1
	}
	return 0
}

type listItem struct {
	model.Item
	Holder string `json:"current_holder"`
}

type listOutput struct {
	Case     model.Case         `json:"case"`
	Sealed   bool               `json:"sealed"`
	Evidence model.EvidenceInfo `json:"evidence"`
	Custody  model.EvidenceInfo `json:"custody"`
	Items    []listItem         `json:"items"`
	Warnings []string           `json:"warnings,omitempty"`
}

func (a *App) cmdList(args []string) int {
	fs := a.newFlagSet("list", "stratum list [flags]")
	var workspacePath string
	var asJSON, showHash bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.BoolVar(&asJSON, "json", false, "machine-readable output")
	fs.BoolVar(&showHash, "hash", false, "show each item's SHA-256")

	_, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	st, code := a.loadState(ws)
	if code != 0 {
		return code
	}
	if asJSON {
		out := listOutput{
			Case: st.Case, Sealed: st.Sealed,
			Evidence: st.EvidenceInfo(), Custody: st.CustodyInfo(),
			Warnings: st.Warnings,
		}
		for _, it := range st.Items {
			out.Items = append(out.Items, listItem{Item: it, Holder: st.Holder(it.ID)})
		}
		return a.writeJSON(out)
	}

	fmt.Fprintf(a.Out, "%s — %s  [%s]\n", st.Case.CaseID, st.Case.Name, sealedLabel(st.Sealed))
	fmt.Fprintf(a.Out, "examiner %s · %d item(s) · %s\n", st.Case.Examiner.Display(), len(st.Items), humanBytes(st.TotalBytes()))
	if !st.ChainsOK() {
		fmt.Fprintln(a.Err, "[!] one or more ledgers failed verification; run: stratum verify")
	}
	if len(st.Items) == 0 {
		fmt.Fprintln(a.Out, "\nNo evidence items yet. Add some with: stratum add <path>...")
		return 0
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintf(a.Out, "%-8s %-25s %-10s %-17s %-20s %s\n", "ID", "TYPE", "SIZE", "ACQUIRED (UTC)", "HOLDER", "FILE")
	for _, it := range st.Items {
		holder := st.Holder(it.ID)
		if holder == "" {
			holder = "—"
		}
		fmt.Fprintf(a.Out, "%-8s %-25s %-10s %-17s %-20s %s\n",
			it.ID, truncate(it.FileType.Description, 25), humanBytes(it.Size),
			fmtTimeShort(it.AcquiredAt), truncate(holder, 20), it.Name)
		if showHash {
			fmt.Fprintf(a.Out, "         sha256 %s  (%s)\n", it.SHA256, it.Mode)
		}
	}
	if holders := st.Ledger.HolderCounts(); len(holders) > 0 {
		fmt.Fprintln(a.Out, "\nCurrent custody:")
		for _, h := range holders {
			fmt.Fprintf(a.Out, "  %3d item(s)  %s\n", h.Count, h.Holder)
		}
	}
	return 0
}

func (a *App) cmdShow(args []string) int {
	fs := a.newFlagSet("show", "stratum show <item-id> [flags]")
	var workspacePath string
	var asJSON bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.BoolVar(&asJSON, "json", false, "machine-readable output")
	pos, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if len(pos) == 0 {
		fmt.Fprintln(a.Err, "stratum: an item id is required (for example E-0001)")
		fs.Usage()
		return 2
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	st, code := a.loadState(ws)
	if code != 0 {
		return code
	}
	item, ok := st.Item(strings.ToUpper(strings.TrimSpace(pos[0])))
	if !ok {
		return a.fail("item %q not found in %s", pos[0], ws.Root)
	}
	check, actual := a.checkItem(ws, item)
	metaCheck := "not recorded"
	metaBad := false
	if item.Metadata.Path != "" {
		if _, err := metadata.Verify(ws, item); err != nil {
			metaCheck = "MODIFIED: " + err.Error()
			metaBad = true
		} else {
			metaCheck = "OK"
		}
	} else if item.Metadata.Error != "" {
		metaCheck = "not recorded: " + item.Metadata.Error
	} else {
		metaCheck = "not recorded (run: stratum extract " + item.ID + ")"
	}
	history := st.Ledger.History(item)
	if asJSON {
		return a.writeJSON(struct {
			Item          model.Item `json:"item"`
			Holder        string     `json:"current_holder"`
			Check         string     `json:"integrity"`
			Actual        string     `json:"actual_sha256,omitempty"`
			MetadataCheck string     `json:"metadata_integrity"`
			Custody       any        `json:"custody_history"`
		}{Item: item, Holder: st.Holder(item.ID), Check: check, Actual: actual, MetadataCheck: metaCheck, Custody: history})
	}

	fmt.Fprintf(a.Out, "%s  %s\n", item.ID, item.Name)
	fmt.Fprintf(a.Out, "  case        %s — %s\n", st.Case.CaseID, st.Case.Name)
	fmt.Fprintf(a.Out, "  type        %s\n", typeDescription(item.FileType))
	fmt.Fprintf(a.Out, "  category    %s\n", item.FileType.Category)
	fmt.Fprintf(a.Out, "  confidence  %s", item.FileType.Confidence)
	if !item.FileType.ExtensionMatch {
		fmt.Fprintf(a.Out, "  [extension mismatch: .%s]", item.FileType.InputExtension)
	}
	fmt.Fprintln(a.Out)
	if item.FileType.Magic != "" {
		fmt.Fprintf(a.Out, "  magic       %s\n", item.FileType.Magic)
	}
	for _, n := range item.FileType.Notes {
		fmt.Fprintf(a.Out, "  type note   %s\n", n)
	}
	fmt.Fprintf(a.Out, "  size        %s (%d bytes)\n", humanBytes(item.Size), item.Size)
	fmt.Fprintf(a.Out, "  sha256      %s\n", item.SHA256)
	fmt.Fprintf(a.Out, "  acquired    %s by %s\n", fmtTime(item.AcquiredAt), item.AcquiredBy.Display())
	fmt.Fprintf(a.Out, "  source      %s\n", item.SourcePath)
	if item.StoredPath != "" {
		fmt.Fprintf(a.Out, "  stored      %s\n", item.StoredPath)
	} else {
		fmt.Fprintln(a.Out, "  stored      — (reference only)")
	}
	fmt.Fprintf(a.Out, "  source mtime %s  mode %s\n", fmtTime(item.SourceModTime), item.SourceMode)
	fmt.Fprintf(a.Out, "  integrity   %s", check)
	if actual != "" && actual != item.SHA256 {
		fmt.Fprintf(a.Out, " (found %s)", actual)
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintf(a.Out, "  metadata    %s\n", dash(item.Metadata.Path))
	if item.Metadata.SHA256 != "" {
		fmt.Fprintf(a.Out, "  meta sha256 %s (%d bytes)\n", item.Metadata.SHA256, item.Metadata.Size)
	}
	fmt.Fprintf(a.Out, "  meta status %s\n", metaCheck)
	fmt.Fprintf(a.Out, "  holder      %s\n", dash(st.Holder(item.ID)))
	if item.DuplicateOf != "" {
		fmt.Fprintf(a.Out, "  duplicate   same bytes as %s\n", item.DuplicateOf)
	}
	if item.Origin != "" {
		fmt.Fprintf(a.Out, "  origin      %s\n", item.Origin)
	}
	if item.Note != "" {
		fmt.Fprintf(a.Out, "  note        %s\n", item.Note)
	}
	if item.EXIF.Present {
		fmt.Fprintf(a.Out, "\n  EXIF        present (%d tags, %s)\n", item.EXIF.Tags, item.EXIF.Container)
		fmt.Fprintf(a.Out, "  make/model  %s %s\n", dash(item.EXIF.Make), dash(item.EXIF.Model))
		fmt.Fprintf(a.Out, "  serial      %s\n", dash(item.EXIF.Serial))
		fmt.Fprintf(a.Out, "  lens        %s\n", dash(item.EXIF.Lens))
		fmt.Fprintf(a.Out, "  original    %s\n", dash(item.EXIF.DateTimeOriginal))
		fmt.Fprintf(a.Out, "  dimensions  %dx%d, orientation %d\n", item.EXIF.Width, item.EXIF.Height, item.EXIF.Orientation)
		if item.EXIF.GPS != nil {
			fmt.Fprintf(a.Out, "  GPS         %s, %s", item.EXIF.GPS.Latitude, item.EXIF.GPS.Longitude)
			if item.EXIF.GPS.Altitude != "" {
				fmt.Fprintf(a.Out, " · %s m", item.EXIF.GPS.Altitude)
			}
			fmt.Fprintln(a.Out)
		}
		if item.EXIF.Error != "" {
			fmt.Fprintf(a.Out, "  exif note   %s\n", item.EXIF.Error)
		}
	} else if item.EXIF.Capable {
		fmt.Fprintf(a.Out, "\n  EXIF        capable container, not extracted: %s\n", item.EXIF.Note)
	}
	if item.Document.Present {
		fmt.Fprintf(a.Out, "\n  DOCUMENT    %s\n", dash(item.Document.Format))
		fmt.Fprintf(a.Out, "  title       %s\n", dash(item.Document.Title))
		fmt.Fprintf(a.Out, "  author      %s\n", dash(item.Document.Author))
		fmt.Fprintf(a.Out, "  subject     %s\n", dash(item.Document.Subject))
		fmt.Fprintf(a.Out, "  created     %s\n", dash(item.Document.Created))
		fmt.Fprintf(a.Out, "  modified    %s\n", dash(item.Document.Modified))
		fmt.Fprintf(a.Out, "  application %s %s\n", dash(item.Document.Application), item.Document.AppVersion)
		fmt.Fprintf(a.Out, "  company     %s\n", dash(item.Document.Company))
		if len(item.Document.Fields) > 0 {
			for k, v := range item.Document.Fields {
				fmt.Fprintf(a.Out, "  field       %s: %s\n", k, v)
			}
		}
	}
	if item.Document.Error != "" {
		fmt.Fprintf(a.Out, "  doc note    %s\n", item.Document.Error)
	}
	fmt.Fprintln(a.Out, "\nCustody history:")
	if len(history) == 0 {
		fmt.Fprintln(a.Out, "  no custody events cover this item")
	} else {
		for _, ev := range history {
			fmt.Fprintf(a.Out, "  #%d  %s  %-14s %s -> %s  (%s)\n",
				ev.Seq, fmtTime(ev.Time), ev.Data.Action, dash(ev.Data.From), dash(ev.Data.To.Display()), dash(ev.Data.Actor.Display()))
		}
	}
	if check == "MODIFIED" || check == "MISSING" || metaBad {
		return 1
	}
	return 0
}

func (a *App) checkItem(ws *workspace.Workspace, item model.Item) (string, string) {
	if item.Mode == "reference" {
		if _, err := os.Stat(item.SourcePath); err != nil {
			return "MISSING", ""
		}
		h, _, err := chain.HashFile(item.SourcePath)
		if err != nil {
			return "UNREADABLE", ""
		}
		if h != item.SHA256 {
			return "MODIFIED", h
		}
		return "REFERENCE-OK", h
	}
	if item.StoredPath == "" {
		return "MISSING", ""
	}
	full, err := ws.ResolveStored(item.StoredPath)
	if err != nil {
		return "UNREADABLE", ""
	}
	if li, lerr := os.Lstat(full); lerr == nil && li.Mode()&os.ModeSymlink != 0 {
		return "MODIFIED", ""
	}
	h, _, err := chain.HashFile(full)
	if err != nil {
		return "MISSING", ""
	}
	if h != item.SHA256 {
		return "MODIFIED", h
	}
	return "OK", h
}

func (a *App) writeJSON(v any) int {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return a.fail("encode json: %v", err)
	}
	return 0
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

func fmtTime(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func fmtTimeShort(s string) string {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return truncate(s, 17)
	}
	return t.UTC().Format("2006-01-02 15:04")
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func typeDescription(ft model.FileType) string {
	if ft.Description == "" {
		return ft.MIME
	}
	if ft.MIME == "" {
		return ft.Description
	}
	return ft.Description + " · " + ft.MIME
}

func sealedLabel(sealed bool) string {
	if sealed {
		return "SEALED"
	}
	return "OPEN"
}

func dash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
