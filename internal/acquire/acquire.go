// Package acquire turns source paths into preserved, read-only evidence items.
//
// Source files are opened read-only and never modified. Each preserved item is
// written to a temporary file, fsynced, read back and SHA-256 verified before it
// is renamed into items/ and made read-only. Source size and mtime are checked
// again after acquisition; a change is recorded, not hidden.
package acquire

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"stratum/internal/casestate"
	"stratum/internal/chain"
	"stratum/internal/docmeta"
	"stratum/internal/exif"
	"stratum/internal/filekind"
	"stratum/internal/logx"
	"stratum/internal/metadata"
	"stratum/internal/model"
	"stratum/internal/workspace"
)

// ErrSealed means the evidence set is sealed and cannot be extended.
var ErrSealed = errors.New("case is sealed")

// Options controls one acquisition batch.
type Options struct {
	Copy       bool
	Exclude    []string
	Origin     string
	Note       string
	SkipHidden bool
}

// Request is one acquisition run.
type Request struct {
	Paths     []string
	Options   Options
	Operator  model.Person
	ForceLock bool
	Log       *logx.Logger
}

// Result summarises an acquisition.
type Result struct {
	Items           []model.Item
	Issues          []model.Issue
	Bytes           int64
	Duplicates      int
	Errors          int
	EvidenceInfo    model.EvidenceInfo
	CustodyInfo     model.EvidenceInfo
	CustodyRecorded bool
}

// Run acquires paths into ws under a workspace lock. It refuses to run against
// a broken chain or a sealed case.
func Run(ws *workspace.Workspace, req Request) (*Result, error) {
	log := req.Log
	if log == nil {
		log = logx.New(io.Discard, false)
	}
	if len(req.Paths) == 0 {
		return nil, errors.New("at least one source path is required")
	}

	lock, err := ws.AcquireLock(req.ForceLock, "add")
	if err != nil {
		return nil, err
	}
	defer func() {
		if rerr := lock.Release(); rerr != nil {
			log.Warn("could not release workspace lock: %v", rerr)
		}
	}()

	st, err := casestate.Load(ws)
	if err != nil {
		return nil, err
	}
	if !st.ChainsOK() {
		return nil, fmt.Errorf("refusing to add evidence: ledger verification failed: %s", firstChainError(st))
	}
	if st.Sealed {
		return nil, fmt.Errorf("%w (last sealed %s); use: stratum unseal --reason \"...\"", ErrSealed, st.SealedAt)
	}
	operator := req.Operator
	if operator.Empty() {
		operator = st.Case.Examiner
	}
	if strings.TrimSpace(operator.Name) == "" {
		return nil, errors.New("an operator name is required (use --examiner or set STRATUM_EXAMINER)")
	}
	if info, err := os.Lstat(ws.ItemsPath); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("evidence directory %s is missing, not a directory, or a symbolic link", ws.ItemsPath)
	}

	// Reject paths inside the workspace before doing any work.
	var paths []string
	seenPath := make(map[string]bool)
	for _, p := range req.Paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		abs = filepath.Clean(abs)
		if ws.Contains(abs) {
			return nil, fmt.Errorf("%s is inside the workspace; pass source material from outside %s", abs, ws.Root)
		}
		if seenPath[abs] {
			continue
		}
		seenPath[abs] = true
		paths = append(paths, abs)
	}

	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		return nil, err
	}
	cwd, _ := os.Getwd()
	if _, err := ev.Add("acquisition_started", st.Case.CaseID, "fs", model.AcquisitionStartedData{
		Operator: operator,
		Paths:    paths,
		Cwd:      cwd,
		Options: model.AcquisitionOptions{
			Copy:       req.Options.Copy,
			Exclude:    req.Options.Exclude,
			Origin:     req.Options.Origin,
			Note:       req.Options.Note,
			SkipHidden: req.Options.SkipHidden,
		},
		Environment: workspace.Environment(),
	}); err != nil {
		_, _ = ev.Close()
		return nil, err
	}

	res := &Result{}
	hashToID := make(map[string]string, len(st.Items))
	for _, it := range st.Items {
		if _, ok := hashToID[it.SHA256]; !ok {
			hashToID[it.SHA256] = it.ID
		}
	}
	nextID := st.NextItemID()
	nextNum := idNumber(nextID, len(st.Items)+1)
	seenSource := make(map[string]bool)

	addIssue := func(issue model.Issue) error {
		issue.Time = time.Now().UTC().Format(time.RFC3339Nano)
		rec, err := ev.Add("item_error", issue.Path, "fs", model.ItemErrorData{Path: issue.Path, Kind: issue.Kind, Reason: issue.Reason})
		if err != nil {
			return err
		}
		issue.EventHash = rec.Hash
		res.Issues = append(res.Issues, issue)
		if issue.Kind == "error" {
			res.Errors++
		}
		if issue.Kind != "warning" {
			log.Warn("%s: %s", issue.Path, issue.Reason)
		} else {
			log.Info("note %s: %s", issue.Path, issue.Reason)
		}
		return nil
	}

	for _, root := range paths {
		if err := acquirePath(ws, root, root, req.Options, operator, ev, hashToID, seenSource, &nextNum, res, addIssue, log); err != nil {
			if errors.Is(err, filepath.SkipDir) {
				continue
			}
			_, _ = ev.Close()
			return res, err
		}
	}

	finished, err := ev.Add("acquisition_finished", st.Case.CaseID, "fs", model.AcquisitionFinishedData{
		Operator:   operator,
		ItemsAdded: len(res.Items),
		ItemIDs:    itemIDs(res.Items),
		Issues:     len(res.Issues),
		Errors:     res.Errors,
		Bytes:      res.Bytes,
		Duplicates: res.Duplicates,
	})
	if err != nil {
		_, _ = ev.Close()
		return res, err
	}
	_ = finished
	evInfo, err := ev.Close()
	if err != nil {
		return res, err
	}
	res.EvidenceInfo = model.EvidenceInfo{
		File:       workspace.EvidenceFile,
		Records:    evInfo.Records,
		RootSHA256: evInfo.RootSHA256,
		FileSHA256: evInfo.FileSHA256,
		ChainOK:    evInfo.ChainOK,
	}

	if len(res.Items) == 0 {
		return res, nil
	}

	cu, err := chain.OpenAppend(ws.CustodyPath)
	if err != nil {
		return res, fmt.Errorf("evidence was recorded (%d item(s), root %s), but the custody ledger could not be opened: %w", len(res.Items), evInfo.RootSHA256, err)
	}
	method := "forensic copy with read-back verification"
	if !req.Options.Copy {
		method = "reference hashing (no preserved copy)"
	}
	if _, err := cu.Add("custody_collect", st.Case.CaseID, "custody", model.CustodyData{
		Action:       "collected",
		Actor:        operator,
		From:         "source media",
		To:           operator,
		Method:       method,
		Purpose:      fmt.Sprintf("acquired %d item(s)", len(res.Items)),
		Items:        itemIDs(res.Items),
		ItemCount:    len(res.Items),
		EvidenceRoot: evInfo.RootSHA256,
	}); err != nil {
		_, _ = cu.Close()
		return res, fmt.Errorf("evidence was recorded but the custody collection event failed: %w", err)
	}
	cuInfo, err := cu.Close()
	if err != nil {
		return res, err
	}
	res.CustodyRecorded = true
	res.CustodyInfo = model.EvidenceInfo{
		File:       workspace.CustodyFile,
		Records:    cuInfo.Records,
		RootSHA256: cuInfo.RootSHA256,
		FileSHA256: cuInfo.FileSHA256,
		ChainOK:    cuInfo.ChainOK,
	}
	return res, nil
}

func acquirePath(
	ws *workspace.Workspace,
	root, current string,
	opts Options,
	operator model.Person,
	ev *chain.Recorder,
	hashToID map[string]string,
	seenSource map[string]bool,
	nextNum *int,
	res *Result,
	addIssue func(model.Issue) error,
	log *logx.Logger,
) error {
	info, err := os.Lstat(current)
	if err != nil {
		return addIssue(model.Issue{Path: current, Kind: "error", Reason: err.Error()})
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(current)
		return addIssue(model.Issue{Path: current, Kind: "skipped", Reason: "symbolic link not followed; target: " + target})
	}
	if info.IsDir() {
		if ws.Contains(current) {
			log.Warn("skipping workspace directory %s", current)
			return nil
		}
		rel, _ := filepath.Rel(root, current)
		if rel != "." && excluded(opts.Exclude, filepath.Base(current), filepath.ToSlash(rel)) {
			log.Verbose("excluded directory %s", current)
			return filepath.SkipDir
		}
		if opts.SkipHidden && rel != "." && strings.HasPrefix(filepath.Base(current), ".") {
			return filepath.SkipDir
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return addIssue(model.Issue{Path: current, Kind: "error", Reason: err.Error()})
		}
		for _, entry := range entries {
			if err := acquirePath(ws, root, filepath.Join(current, entry.Name()), opts, operator, ev, hashToID, seenSource, nextNum, res, addIssue, log); err != nil {
				if errors.Is(err, filepath.SkipDir) {
					continue
				}
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return addIssue(model.Issue{Path: current, Kind: "skipped", Reason: "not a regular file (" + info.Mode().String() + ")"})
	}
	if ws.Contains(current) {
		return addIssue(model.Issue{Path: current, Kind: "skipped", Reason: "file is inside the workspace"})
	}
	rel, _ := filepath.Rel(root, current)
	if excluded(opts.Exclude, filepath.Base(current), filepath.ToSlash(rel)) {
		log.Verbose("excluded file %s", current)
		return nil
	}
	if opts.SkipHidden && strings.HasPrefix(filepath.Base(current), ".") {
		log.Verbose("skipped hidden file %s", current)
		return nil
	}
	if seenSource[current] {
		return addIssue(model.Issue{Path: current, Kind: "skipped", Reason: "source path already processed in this batch"})
	}
	seenSource[current] = true

	itemID := fmt.Sprintf("E-%04d", *nextNum)
	item, issue, err := acquireFile(ws, current, itemID, opts, operator, hashToID, log)
	if err != nil {
		return err
	}
	if issue != nil {
		if err := addIssue(*issue); err != nil {
			return err
		}
		if item.ID == "" {
			return nil
		}
	}
	if _, err := ev.Add("item_acquired", item.ID, "fs", item); err != nil {
		return fmt.Errorf("record %s: %w (preserved file: %s)", item.ID, err, item.StoredPath)
	}
	*nextNum++
	res.Items = append(res.Items, item)
	res.Bytes += item.Size
	if item.DuplicateOf != "" {
		res.Duplicates++
	}
	if _, ok := hashToID[item.SHA256]; !ok {
		hashToID[item.SHA256] = item.ID
	}
	log.Success("%s  %7s  %s", item.ID, humanBytes(item.Size), item.Name)
	return nil
}

func acquireFile(ws *workspace.Workspace, src, itemID string, opts Options, operator model.Person, hashToID map[string]string, log *logx.Logger) (model.Item, *model.Issue, error) {
	srcF, err := os.Open(src)
	if err != nil {
		return model.Item{}, &model.Issue{Path: src, Kind: "error", Reason: err.Error()}, nil
	}
	defer srcF.Close()
	before, err := srcF.Stat()
	if err != nil {
		return model.Item{}, &model.Issue{Path: src, Kind: "error", Reason: err.Error()}, nil
	}

	acquiredAt := time.Now().UTC().Format(time.RFC3339Nano)
	item := model.Item{
		ID:            itemID,
		Name:          filepath.Base(src),
		SourcePath:    src,
		Mode:          "reference",
		Origin:        strings.TrimSpace(opts.Origin),
		Note:          strings.TrimSpace(opts.Note),
		SourceModTime: before.ModTime().UTC().Format(time.RFC3339Nano),
		SourceMode:    before.Mode().String(),
		AcquiredAt:    acquiredAt,
		AcquiredBy:    operator,
		SourceStable:  true,
	}

	var tmp *os.File
	var tmpName string
	h := sha256.New()
	buf := make([]byte, 1024*1024)

	if opts.Copy {
		tmp, err = os.CreateTemp(ws.ItemsPath, ".incoming-*")
		if err != nil {
			return model.Item{}, nil, fmt.Errorf("create temporary evidence file: %w", err)
		}
		tmpName = tmp.Name()
		defer func() {
			if tmpName != "" {
				_ = os.Remove(tmpName)
			}
		}()
		if err := tmp.Chmod(0o600); err != nil {
			_ = tmp.Close()
			return model.Item{}, nil, err
		}
		n, cerr := io.CopyBuffer(io.MultiWriter(tmp, h), srcF, buf)
		if cerr != nil {
			_ = tmp.Close()
			return model.Item{}, &model.Issue{Path: src, Kind: "error", Reason: "read/copy failed: " + cerr.Error()}, nil
		}
		if n != before.Size() {
			item.SourceStable = false
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			return model.Item{}, nil, fmt.Errorf("sync temporary evidence file: %w", err)
		}
		if err := tmp.Close(); err != nil {
			return model.Item{}, nil, err
		}
		item.Size = n
		item.SHA256 = hex.EncodeToString(h.Sum(nil))

		// Read the preserved bytes back. This catches a bad disk much earlier
		// than a later case verification would.
		readHash, readSize, err := chain.HashFile(tmpName)
		if err != nil {
			return model.Item{}, nil, fmt.Errorf("read back preserved copy: %w", err)
		}
		if readSize != n || readHash != item.SHA256 {
			return model.Item{}, &model.Issue{Path: src, Kind: "error", Reason: "preserved copy did not verify: read-back SHA-256 differs from source bytes"}, nil
		}
		item.CopyVerified = true

		// Metadata is read from the preserved bytes, not the source.
		ft, ferr := filekind.Detect(tmpName, item.Name)
		if ferr != nil {
			item.FileType = unknownType(ferr)
			log.Info("note %s: file type detection failed: %v", item.ID, ferr)
		} else {
			item.FileType = ft
		}
		item.EXIF = exif.Extract(tmpName)
		item.Document = docmeta.Extract(tmpName, item.FileType.ID)
	} else {
		srcF.Seek(0, io.SeekStart)
		n, cerr := io.CopyBuffer(h, srcF, buf)
		if cerr != nil {
			return model.Item{}, &model.Issue{Path: src, Kind: "error", Reason: "read failed: " + cerr.Error()}, nil
		}
		item.Size = n
		item.SHA256 = hex.EncodeToString(h.Sum(nil))
		if n != before.Size() {
			item.SourceStable = false
		}
		ft, ferr := filekind.Detect(src, item.Name)
		if ferr != nil {
			item.FileType = unknownType(ferr)
			log.Info("note %s: file type detection failed: %v", item.ID, ferr)
		} else {
			item.FileType = ft
		}
		item.EXIF = exif.Extract(src)
		item.Document = docmeta.Extract(src, item.FileType.ID)
	}

	// Detect modification of the source while it was being read.
	if after, err := os.Stat(src); err != nil {
		item.SourceStable = false
	} else if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		item.SourceStable = false
	}
	var issue *model.Issue
	if !item.SourceStable {
		issue = &model.Issue{Path: src, Kind: "warning", Reason: "source file changed or disappeared during acquisition; the recorded hash covers exactly the bytes read"}
	}

	if first, ok := hashToID[item.SHA256]; ok {
		item.DuplicateOf = first
	}

	if opts.Copy {
		finalName := storedFilename(itemID, item.Name)
		finalPath := filepath.Join(ws.ItemsPath, finalName)
		if _, err := os.Lstat(finalPath); err == nil {
			return model.Item{}, nil, fmt.Errorf("refusing to overwrite existing evidence file %s", finalPath)
		}
		if err := os.Chtimes(tmpName, before.ModTime(), before.ModTime()); err != nil {
			log.Info("note %s: could not preserve source timestamps: %v", item.ID, err)
		}
		if err := os.Chmod(tmpName, 0o444); err != nil {
			return model.Item{}, nil, err
		}
		if err := os.Rename(tmpName, finalPath); err != nil {
			return model.Item{}, nil, fmt.Errorf("move preserved copy into place: %w", err)
		}
		tmpName = "" // successfully moved; do not remove in the defer
		syncDir(ws.ItemsPath)
		item.StoredPath = filepath.ToSlash(filepath.Join(workspace.ItemsDir, finalName))
		item.Mode = "preserved"
	} else {
		item.Mode = "reference"
	}
	// Persist the extracted EXIF/document metadata as a hashed sidecar. The
	// item record then carries the sidecar's path and SHA-256, so both the
	// evidence bytes and the metadata about them are anchored in the chain.
	extractedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if item.EXIF.Error != "" {
		log.Warn("note %s: EXIF parser: %v", item.ID, item.EXIF.Error)
	}
	if item.Document.Error != "" {
		log.Warn("note %s: document metadata: %v", item.ID, item.Document.Error)
	}
	if item.Document.Present {
		log.Verbose("document metadata %s: format=%s title=%q", item.ID, item.Document.Format, item.Document.Title)
	}
	ref, merr := metadata.Write(ws, item, item.EXIF, item.Document, extractedAt, false)
	if merr != nil {
		item.Metadata = model.MetadataRef{Error: merr.Error()}
		log.Warn("note %s: metadata extraction was not persisted: %v", item.ID, merr)
	} else {
		item.Metadata = ref
		log.Verbose("metadata %s -> %s (%s)", item.ID, ref.Path, ref.SHA256)
	}
	return item, issue, nil
}

func unknownType(err error) model.FileType {
	ft := model.FileType{
		ID: "unknown", MIME: "application/octet-stream", Category: "other",
		Description: "Type detection failed", Confidence: "low",
	}
	if err != nil {
		ft.Notes = []string{err.Error()}
	}
	return ft
}

func storedFilename(id, original string) string {
	base := filepath.Base(strings.ReplaceAll(original, "\\", "/"))
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = "file"
	}
	var b strings.Builder
	for _, r := range base {
		if r < 0x20 || r == 0x7F || strings.ContainsRune(`<>:"/\|?*`, r) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	name := strings.TrimSpace(b.String())
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	// Keep the stored name readable but bounded; evidence IDs make it unique.
	runes := []rune(name)
	if len(runes) > 80 {
		ext := filepath.Ext(name)
		keep := 80 - len([]rune(ext))
		if keep < 8 {
			keep = 8
		}
		name = string(runes[:keep]) + ext
	}
	return id + "__" + name
}

func excluded(patterns []string, base, rel string) bool {
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if ok, _ := path.Match(p, base); ok {
			return true
		}
		if ok, _ := path.Match(p, rel); ok {
			return true
		}
	}
	return false
}

func itemIDs(items []model.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func idNumber(id string, fallback int) int {
	s := strings.TrimPrefix(id, "E-")
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

func firstChainError(st *casestate.State) string {
	errs := st.ChainErrors()
	if len(errs) > 0 {
		return errs[0]
	}
	return "unknown chain error"
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

func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
