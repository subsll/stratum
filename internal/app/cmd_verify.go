package app

import (
	"fmt"
	"os"
	"strings"

	"stratum/internal/casestate"
	"stratum/internal/chain"
	"stratum/internal/model"
	"stratum/internal/verify"
	"stratum/internal/workspace"
)

func (a *App) cmdVerify(args []string) int {
	fs := a.newFlagSet("verify", "stratum verify [evidence.jsonl] [flags]")
	var workspacePath string
	var noRehash, record, asJSON, forceLock bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.BoolVar(&noRehash, "no-rehash", false, "check chains and cross-links only")
	fs.BoolVar(&record, "record", false, "append a verification event to the ledgers")
	fs.BoolVar(&asJSON, "json", false, "machine-readable report")
	fs.BoolVar(&forceLock, "force-lock", false, "break a workspace lock after confirming it is stale")
	p := a.addPersonFlags(fs, false)
	pos, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if len(pos) > 1 {
		fmt.Fprintln(a.Err, "stratum: verify accepts at most one ledger path")
		fs.Usage()
		return 2
	}
	if len(pos) == 1 {
		if !strings.HasSuffix(strings.ToLower(pos[0]), ".jsonl") {
			return a.fail("standalone verification needs a .jsonl ledger (or use --workspace for a case)")
		}
		if record {
			return a.fail("--record applies to a case workspace, not a standalone ledger")
		}
		return a.verifyStandalone(pos[0], asJSON)
	}

	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	var lock *workspace.Lock
	if record {
		var err error
		lock, err = ws.AcquireLock(forceLock, "verify --record")
		if err != nil {
			return a.fail("%v", err)
		}
		defer func() {
			if rerr := lock.Release(); rerr != nil {
				fmt.Fprintf(a.Err, "[!] could not release workspace lock: %v\n", rerr)
			}
		}()
	}
	st, code := a.loadState(ws)
	if code != 0 {
		return code
	}
	vrep, err := verify.Run(ws, st, !noRehash)
	if err != nil {
		return a.fail("%v", err)
	}

	if record {
		if !vrep.OK {
			a.printVerify(vrep, ws.Root)
			return a.fail("verification failed; no verification event was recorded")
		}
		operator, perr := p.Person(st.Case.Examiner, true)
		if perr != nil {
			return a.fail("%v", perr)
		}
		evRec, cuRec, evInfo, cuInfo, rerr := a.recordVerification(ws, st, vrep, operator)
		if rerr != nil {
			a.printVerify(vrep, ws.Root)
			return a.fail("%v", rerr)
		}
		if asJSON {
			return a.writeJSON(struct {
				Verification   *verify.Report `json:"verification"`
				EvidenceRecord chain.Record   `json:"evidence_record"`
				CustodyRecord  chain.Record   `json:"custody_record"`
				EvidenceRoot   string         `json:"evidence_root"`
				CustodyRoot    string         `json:"custody_root"`
			}{vrep, evRec, cuRec, evInfo.RootSHA256, cuInfo.RootSHA256})
		}
		a.printVerify(vrep, ws.Root)
		fmt.Fprintf(a.Out, "recorded     verification #%d %s\n", evRec.Seq, evRec.Hash)
		fmt.Fprintf(a.Out, "             custody check #%d %s\n", cuRec.Seq, cuRec.Hash)
		fmt.Fprintf(a.Out, "new roots    evidence %s\n", evInfo.RootSHA256)
		fmt.Fprintf(a.Out, "             custody  %s\n", cuInfo.RootSHA256)
		return 0
	}

	if asJSON {
		_ = a.writeJSON(vrep)
	} else {
		a.printVerify(vrep, ws.Root)
	}
	if !vrep.OK {
		return 1
	}
	return 0
}

func (a *App) verifyStandalone(path string, asJSON bool) int {
	res, err := chain.Load(path)
	if err != nil {
		return a.fail("%v", err)
	}
	rep := res.Report
	sidecar := ""
	if data, err := os.ReadFile(path + ".sha256"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			sidecar = fields[0]
			if sidecar != rep.FileSHA256 {
				rep.OK = false
				rep.Errors = append(rep.Errors, "sidecar hash does not match the ledger file")
			}
		}
	}
	if asJSON {
		_ = a.writeJSON(rep)
	} else {
		fmt.Fprintf(a.Out, "file        %s\n", rep.File)
		fmt.Fprintf(a.Out, "records     %d\n", rep.Records)
		fmt.Fprintf(a.Out, "root sha256 %s\n", rep.RootSHA256)
		fmt.Fprintf(a.Out, "file sha256 %s\n", rep.FileSHA256)
		if sidecar != "" {
			if sidecar == rep.FileSHA256 {
				fmt.Fprintf(a.Out, "sidecar     matches %s.sha256\n", path)
			} else {
				fmt.Fprintf(a.Out, "sidecar     MISMATCH (sidecar says %s)\n", sidecar)
			}
		}
		if rep.OK {
			fmt.Fprintln(a.Out, "chain       OK — every record matches its hash and the previous record")
			fmt.Fprintln(a.Out, "verdict     OK")
		} else {
			fmt.Fprintln(a.Out, "chain       BROKEN")
			for _, e := range rep.Errors {
				fmt.Fprintf(a.Out, "  - %s\n", e)
			}
			fmt.Fprintln(a.Out, "verdict     FAILED")
		}
	}
	if rep.OK {
		return 0
	}
	return 1
}

func (a *App) printVerify(v *verify.Report, workspacePath string) {
	fmt.Fprintf(a.Out, "workspace   %s\n", workspacePath)
	fmt.Fprintf(a.Out, "evidence    %s  %d record(s)  root %s\n", okLabel(v.Evidence.OK), v.Evidence.Records, v.Evidence.RootSHA256)
	fmt.Fprintf(a.Out, "custody     %s  %d record(s)  root %s\n", okLabel(v.Custody.OK), v.Custody.Records, v.Custody.RootSHA256)
	fmt.Fprintf(a.Out, "cross-links %s\n", okLabel(v.CrossLinksOK))
	if v.Rehashed {
		fmt.Fprintln(a.Out, "item bytes  re-hashed")
	} else {
		fmt.Fprintln(a.Out, "item bytes  not re-hashed")
	}
	fmt.Fprintf(a.Out, "items       %d checked, %d verified, %d missing, %d modified, %d unexpected, %d reference-only\n",
		v.ItemCount, v.ItemsOK, len(v.Missing), len(v.Modified), len(v.Unexpected), len(v.Reference))
	fmt.Fprintf(a.Out, "metadata    %d checked, %d verified, %d missing, %d modified, %d unexpected\n",
		v.MetadataChecked, v.MetadataOK, len(v.MetadataMissing), len(v.MetadataModified), len(v.MetadataUnexpected))
	if len(v.Errors) > 0 {
		fmt.Fprintln(a.Out, "errors:")
		for _, e := range v.Errors {
			fmt.Fprintf(a.Out, "  - %s\n", e)
		}
	}
	if len(v.Warnings) > 0 {
		fmt.Fprintln(a.Out, "warnings:")
		limit := len(v.Warnings)
		if limit > 10 {
			limit = 10
		}
		for _, w := range v.Warnings[:limit] {
			fmt.Fprintf(a.Out, "  - %s\n", w)
		}
		if len(v.Warnings) > limit {
			fmt.Fprintf(a.Out, "  ... and %d more\n", len(v.Warnings)-limit)
		}
	}
	verdict := "FAILED"
	if v.OK {
		if v.Complete {
			verdict = "PASSED (complete)"
		} else {
			verdict = "PASSED (partial — see warnings)"
		}
	}
	fmt.Fprintf(a.Out, "verdict     %s\n", verdict)
}

func (a *App) recordVerification(ws *workspace.Workspace, st *casestate.State, v *verify.Report, operator model.Person) (chain.Record, chain.Record, model.EvidenceInfo, model.EvidenceInfo, error) {
	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		return chain.Record{}, chain.Record{}, model.EvidenceInfo{}, model.EvidenceInfo{}, err
	}
	evRec, err := ev.Add("verification", st.Case.CaseID, "stratum", model.VerificationData{
		Operator: operator, OK: v.OK, Complete: v.Complete, Rehashed: v.Rehashed,
		ChainsOK: v.ChainsOK, CrossLinksOK: v.CrossLinksOK,
		ItemsChecked: v.ItemCount, ItemsOK: v.ItemsOK,
		Missing: len(v.Missing), Modified: len(v.Modified),
		Unexpected: len(v.Unexpected), Reference: len(v.Reference),
		MetadataChecked: v.MetadataChecked, MetadataOK: v.MetadataOK,
		MetadataMissing: len(v.MetadataMissing), MetadataModified: len(v.MetadataModified),
		MetadataUnexpected: len(v.MetadataUnexpected),
	})
	if err != nil {
		_, _ = ev.Close()
		return chain.Record{}, chain.Record{}, model.EvidenceInfo{}, model.EvidenceInfo{}, err
	}
	evInfo, err := ev.Close()
	if err != nil {
		return chain.Record{}, chain.Record{}, model.EvidenceInfo{}, model.EvidenceInfo{}, err
	}
	cu, err := chain.OpenAppend(ws.CustodyPath)
	if err != nil {
		return evRec, chain.Record{}, model.EvidenceInfo{}, model.EvidenceInfo{}, err
	}
	holder := custodyHolderSummary(st)
	cuRec, err := cu.Add("custody_verify", st.Case.CaseID, "custody", model.CustodyData{
		Action: "verified", Actor: operator,
		From:    holder,
		Purpose: "integrity verification", Items: []string{"*"},
		ItemCount: v.ItemCount, EvidenceRoot: evInfo.RootSHA256,
	})
	if err != nil {
		_, _ = cu.Close()
		return evRec, chain.Record{}, model.EvidenceInfo{File: workspace.EvidenceFile, Records: evInfo.Records, RootSHA256: evInfo.RootSHA256, FileSHA256: evInfo.FileSHA256, ChainOK: evInfo.ChainOK}, model.EvidenceInfo{}, err
	}
	cuInfo, err := cu.Close()
	if err != nil {
		return evRec, cuRec, model.EvidenceInfo{File: workspace.EvidenceFile, Records: evInfo.Records, RootSHA256: evInfo.RootSHA256, FileSHA256: evInfo.FileSHA256, ChainOK: evInfo.ChainOK}, model.EvidenceInfo{}, err
	}
	return evRec, cuRec, model.EvidenceInfo{
		File: workspace.EvidenceFile, Records: evInfo.Records, RootSHA256: evInfo.RootSHA256, FileSHA256: evInfo.FileSHA256, ChainOK: evInfo.ChainOK,
	}, model.EvidenceInfo{
		File: workspace.CustodyFile, Records: cuInfo.Records, RootSHA256: cuInfo.RootSHA256, FileSHA256: cuInfo.FileSHA256, ChainOK: cuInfo.ChainOK,
	}, nil
}

func okLabel(ok bool) string {
	if ok {
		return "OK  "
	}
	return "FAIL"
}
