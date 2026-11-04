package app

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/subsll/stratum/internal/chain"
	"github.com/subsll/stratum/internal/metadata"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

// cmdExtract adds or refreshes metadata sidecars for existing items. It appends
// an item_metadata record so the sidecar hash is anchored in the evidence chain.
func (a *App) cmdExtract(args []string) int {
	fs := a.newFlagSet("extract", "stratum extract [--all | <item-id>...] [flags]")
	var workspacePath string
	var all, force, forceLock bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.BoolVar(&all, "all", false, "extract metadata for every item")
	fs.BoolVar(&force, "force", false, "re-extract and replace existing sidecars")
	fs.BoolVar(&forceLock, "force-lock", false, "break a workspace lock after confirming it is stale")
	p := a.addPersonFlags(fs, false)
	pos, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if !all && len(pos) == 0 {
		fmt.Fprintln(a.Err, "stratum: give one or more item ids, or --all")
		fs.Usage()
		return 2
	}
	if all && len(pos) > 0 {
		return a.fail("use either --all or explicit item ids, not both")
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	lock, err := ws.AcquireLock(forceLock, "extract")
	if err != nil {
		return a.fail("%v", err)
	}
	defer func() {
		if rerr := lock.Release(); rerr != nil {
			fmt.Fprintf(a.Err, "[!] could not release workspace lock: %v\n", rerr)
		}
	}()
	st, code := a.loadState(ws)
	if code != 0 {
		return code
	}
	if !a.chainsReady(st) {
		return 1
	}
	operator, err := p.Person(st.Case.Examiner, true)
	if err != nil {
		return a.fail("%v", err)
	}

	var selected []model.Item
	if all {
		selected = append(selected, st.Items...)
	} else {
		seen := map[string]bool{}
		for _, raw := range pos {
			id := strings.ToUpper(strings.TrimSpace(raw))
			it, ok := st.Item(id)
			if !ok {
				return a.fail("unknown item %q", raw)
			}
			if !seen[id] {
				seen[id] = true
				selected = append(selected, it)
			}
		}
	}
	if len(selected) == 0 {
		fmt.Fprintln(a.Out, "No items to extract.")
		return 0
	}

	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		return a.fail("%v", err)
	}
	written := 0
	skipped := 0
	failed := 0
	var writtenIDs []string
	for _, item := range selected {
		if item.Metadata.Path != "" && !force {
			if _, err := metadata.Verify(ws, item); err == nil {
				fmt.Fprintf(a.Out, "= %s  metadata already current  %s\n", item.ID, item.Metadata.Path)
				skipped++
				continue
			} else {
				fmt.Fprintf(a.Err, "[!] %s  existing metadata sidecar failed verification: %v\n", item.ID, err)
				fmt.Fprintf(a.Err, "    use --force to re-extract and record a replacement\n")
				failed++
				continue
			}
		}
		source, err := a.itemSource(ws, item)
		if err != nil {
			fmt.Fprintf(a.Err, "[!] %s  %v\n", item.ID, err)
			failed++
			continue
		}
		if actual, _, err := chain.HashFile(source); err != nil {
			fmt.Fprintf(a.Err, "[!] %s  cannot hash source: %v\n", item.ID, err)
			failed++
			continue
		} else if actual != item.SHA256 {
			fmt.Fprintf(a.Err, "[!] %s  item bytes do not match the recorded SHA-256; run stratum verify\n", item.ID)
			failed++
			continue
		}
		extractedAt := time.Now().UTC().Format(time.RFC3339Nano)
		ref, exifData, doc, err := metadata.ExtractAndWrite(ws, item, source, extractedAt, force)
		if err != nil {
			fmt.Fprintf(a.Err, "[!] %s  write metadata: %v\n", item.ID, err)
			failed++
			continue
		}
		rec, err := ev.Add("item_metadata", item.ID, "stratum", model.ItemMetadataData{
			ItemID: item.ID, Operator: operator, Metadata: ref, EXIF: exifData, Document: doc,
		})
		if err != nil {
			_, _ = ev.Close()
			return a.fail("metadata sidecar %s was written but the evidence record failed: %v", ref.Path, err)
		}
		written++
		writtenIDs = append(writtenIDs, item.ID)
		fmt.Fprintf(a.Out, "[+] %s  metadata  %s\n", item.ID, ref.Path)
		fmt.Fprintf(a.Out, "    sha256 %s (%d bytes)\n", ref.SHA256, ref.Size)
		if doc.Present || exifData.Present {
			fmt.Fprintf(a.Out, "    data   exif=%t document=%t\n", exifData.Present, doc.Present)
		}
		if exifData.Error != "" {
			fmt.Fprintf(a.Out, "    exif note %s\n", exifData.Error)
		}
		if doc.Error != "" {
			fmt.Fprintf(a.Out, "    doc note  %s\n", doc.Error)
		}
		fmt.Fprintf(a.Out, "    record #%d %s\n", rec.Seq, rec.Hash)
	}
	info, err := ev.Close()
	if err != nil {
		return a.fail("%v", err)
	}
	custodyInfo := model.EvidenceInfo{}
	if written > 0 {
		ids := writtenIDs
		cu, err := chain.OpenAppend(ws.CustodyPath)
		if err != nil {
			fmt.Fprintf(a.Err, "[!] metadata records were written but the custody ledger could not be opened: %v\n", err)
			failed++
		} else {
			holder := custodyHolderSummary(st)
			rec, err := cu.Add("custody_metadata", st.Case.CaseID, "custody", model.CustodyData{
				Action: "metadata", Actor: operator,
				From: holder, Purpose: "extract EXIF/document metadata",
				Items: ids, ItemCount: len(ids), EvidenceRoot: info.RootSHA256,
			})
			if err != nil {
				_, _ = cu.Close()
				fmt.Fprintf(a.Err, "[!] metadata records were written but the custody event failed: %v\n", err)
				failed++
			} else {
				if ci, cerr := cu.Close(); cerr == nil {
					custodyInfo = model.EvidenceInfo{File: workspace.CustodyFile, Records: ci.Records, RootSHA256: ci.RootSHA256, FileSHA256: ci.FileSHA256, ChainOK: ci.ChainOK}
					fmt.Fprintf(a.Out, "    custody record #%d %s\n", rec.Seq, rec.Hash)
				}
			}
		}
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintf(a.Out, "[+] extract complete: %d written, %d already current, %d failed\n", written, skipped, failed)
	fmt.Fprintf(a.Out, "    evidence root %s (%d records)\n", info.RootSHA256, info.Records)
	if custodyInfo.RootSHA256 != "" {
		fmt.Fprintf(a.Out, "    custody root  %s (%d records)\n", custodyInfo.RootSHA256, custodyInfo.Records)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func (a *App) itemSource(ws *workspace.Workspace, item model.Item) (string, error) {
	if item.Mode == "preserved" {
		if item.StoredPath == "" {
			return "", errors.New("preserved item has no stored path")
		}
		full, err := ws.ResolveStored(item.StoredPath)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(full)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("preserved item is a symbolic link")
		}
		return full, nil
	}
	if item.SourcePath == "" {
		return "", errors.New("reference item has no source path")
	}
	info, err := os.Lstat(item.SourcePath)
	if err != nil {
		return "", fmt.Errorf("reference source is unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("reference source is a symbolic link")
	}
	return item.SourcePath, nil
}
