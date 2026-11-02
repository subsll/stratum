package app

import (
	"fmt"
	"strings"

	"stratum/internal/casestate"
	"stratum/internal/chain"
	"stratum/internal/custody"
	"stratum/internal/model"
)

func (a *App) cmdCustody(args []string) int {
	sub := "list"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = strings.ToLower(args[0])
		args = args[1:]
	}
	switch sub {
	case "list", "show", "status":
		return a.custodyList(args)
	case "transfer", "handover", "handoff":
		return a.custodyMove("transferred", args)
	case "return", "returned":
		return a.custodyMove("returned", args)
	default:
		fmt.Fprintf(a.Err, "stratum custody: unknown subcommand %q (want list, transfer or return)\n", sub)
		return 2
	}
}

type custodyListOutput struct {
	Case     model.Case            `json:"case"`
	Sealed   bool                  `json:"sealed"`
	Evidence model.EvidenceInfo    `json:"evidence"`
	Custody  model.EvidenceInfo    `json:"custody"`
	Events   []custody.Event       `json:"events"`
	Holders  []custody.HolderCount `json:"holders,omitempty"`
	Warnings []string              `json:"warnings,omitempty"`
}

func (a *App) custodyList(args []string) int {
	fs := a.newFlagSet("custody list", "stratum custody list [flags]")
	var workspacePath string
	var asJSON bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.BoolVar(&asJSON, "json", false, "machine-readable output")
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
		return a.writeJSON(custodyListOutput{
			Case: st.Case, Sealed: st.Sealed,
			Evidence: st.EvidenceInfo(), Custody: st.CustodyInfo(),
			Events: st.Ledger.Events, Holders: st.Ledger.HolderCounts(), Warnings: st.Warnings,
		})
	}
	fmt.Fprintf(a.Out, "CHAIN OF CUSTODY — %s — %s  [%s]\n", st.Case.CaseID, st.Case.Name, sealedLabel(st.Sealed))
	if !st.CustodyOK() {
		fmt.Fprintln(a.Err, "[!] custody ledger failed verification; run: stratum verify")
	}
	if len(st.Ledger.Events) == 0 {
		fmt.Fprintln(a.Out, "No custody records.")
		return 0
	}
	fmt.Fprintln(a.Out)
	for _, ev := range st.Ledger.Events {
		fmt.Fprintf(a.Out, "#%-3d %s  %-14s %s -> %s\n",
			ev.Seq, fmtTime(ev.Time), ev.Data.Action, dash(ev.Data.From), dash(ev.Data.To.Display()))
		fmt.Fprintf(a.Out, "     actor: %s", dash(ev.Data.Actor.Display()))
		if len(ev.Data.Items) == 0 || (len(ev.Data.Items) == 1 && ev.Data.Items[0] == "*") {
			fmt.Fprintf(a.Out, " · items: all in scope (%d)", ev.Data.ItemCount)
		} else {
			fmt.Fprintf(a.Out, " · items: %s (%d)", strings.Join(ev.Data.Items, ", "), ev.Data.ItemCount)
		}
		fmt.Fprintln(a.Out)
		if ev.Data.Method != "" {
			fmt.Fprintf(a.Out, "     method: %s\n", ev.Data.Method)
		}
		if ev.Data.Purpose != "" {
			fmt.Fprintf(a.Out, "     purpose: %s\n", ev.Data.Purpose)
		}
		if ev.Data.Location != "" {
			fmt.Fprintf(a.Out, "     location: %s\n", ev.Data.Location)
		}
		if ev.Data.Note != "" {
			fmt.Fprintf(a.Out, "     note: %s\n", ev.Data.Note)
		}
		fmt.Fprintf(a.Out, "     evidence anchor: %s\n", ev.Data.EvidenceRoot)
		fmt.Fprintf(a.Out, "     record sha256:  %s\n", ev.Hash)
	}
	fmt.Fprintln(a.Out, "\nCurrent custody:")
	for _, h := range st.Ledger.HolderCounts() {
		fmt.Fprintf(a.Out, "  %3d item(s)  %s\n", h.Count, h.Holder)
	}
	return 0
}

func (a *App) custodyMove(action string, args []string) int {
	verb := "transfer"
	if action == "returned" {
		verb = "return"
	}
	fs := a.newFlagSet("custody "+verb, "stratum custody "+verb+" --to \"Name\" [flags]")
	var workspacePath, toName, toOrg, toEmail, from, method, purpose, location, note string
	var items stringList
	var forceLock bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.StringVar(&toName, "to", "", "recipient name (required)")
	fs.StringVar(&toOrg, "to-org", "", "recipient organisation")
	fs.StringVar(&toEmail, "to-email", "", "recipient email")
	fs.StringVar(&from, "from", "", "expected current custodian")
	fs.Var(&items, "item", "item id to move (repeatable)")
	fs.StringVar(&method, "method", "", "handover method")
	fs.StringVar(&purpose, "purpose", "", "reason for the handover")
	fs.StringVar(&location, "location", "", "place of handover")
	fs.StringVar(&note, "note", "", "free-text note")
	fs.BoolVar(&forceLock, "force-lock", false, "break a workspace lock after confirming it is stale")
	p := a.addPersonFlags(fs, true)
	_, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if strings.TrimSpace(toName) == "" {
		fmt.Fprintln(a.Err, "stratum: --to is required")
		fs.Usage()
		return 2
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	lock, err := ws.AcquireLock(forceLock, "custody "+verb)
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
	if len(st.Items) == 0 {
		return a.fail("this case has no evidence items to move")
	}
	operator, err := p.Person(st.Case.Examiner, true)
	if err != nil {
		return a.fail("%v", err)
	}

	scopeIDs, scopeLabel, count := []string{"*"}, "all items", len(st.Items)
	if len(items.values) > 0 {
		scopeIDs = nil
		count = 0
		seen := map[string]bool{}
		for _, raw := range items.values {
			id := strings.ToUpper(strings.TrimSpace(raw))
			if _, ok := st.Item(id); !ok {
				return a.fail("unknown item %q", raw)
			}
			if !seen[id] {
				seen[id] = true
				scopeIDs = append(scopeIDs, id)
				count++
			}
		}
		scopeLabel = strings.Join(scopeIDs, ", ")
	}

	holders := map[string]bool{}
	for _, id := range scopeIDs {
		if id == "*" {
			for _, it := range st.Items {
				if h := st.Holder(it.ID); h != "" {
					holders[h] = true
				}
			}
			continue
		}
		if h := st.Holder(id); h != "" {
			holders[h] = true
		}
	}
	if len(holders) == 0 {
		return a.fail("no current custodian is recorded for %s", scopeLabel)
	}
	if len(holders) > 1 {
		var names []string
		for h := range holders {
			names = append(names, h)
		}
		return a.fail("items have different custodians (%s); move them in separate transfers with --item", strings.Join(names, "; "))
	}
	var current string
	for h := range holders {
		current = h
	}
	if strings.TrimSpace(from) != "" && from != current {
		return a.fail("--from %q does not match the current custodian %q", from, current)
	}
	toName, toEmail = splitEmail(toName, toEmail)
	to := model.Person{Name: toName, Org: strings.TrimSpace(toOrg), Email: toEmail}
	if to.Display() == current {
		return a.fail("recipient is already the current custodian (%s)", current)
	}

	cu, err := chain.OpenAppend(ws.CustodyPath)
	if err != nil {
		return a.fail("%v", err)
	}
	actionName := "transferred"
	recordType := "custody_transfer"
	if action == "returned" {
		actionName = "returned"
		recordType = "custody_return"
	}
	rec, err := cu.Add(recordType, st.Case.CaseID, "custody", model.CustodyData{
		Action:       actionName,
		Actor:        operator,
		From:         current,
		To:           to,
		Method:       strings.TrimSpace(method),
		Purpose:      strings.TrimSpace(purpose),
		Location:     strings.TrimSpace(location),
		Note:         strings.TrimSpace(note),
		Items:        scopeIDs,
		ItemCount:    count,
		EvidenceRoot: st.EvidenceInfo().RootSHA256,
	})
	if err != nil {
		_, _ = cu.Close()
		return a.fail("%v", err)
	}
	info, err := cu.Close()
	if err != nil {
		return a.fail("%v", err)
	}
	fmt.Fprintf(a.Out, "[+] custody %s recorded\n", actionName)
	fmt.Fprintf(a.Out, "    from        %s\n", current)
	fmt.Fprintf(a.Out, "    to          %s\n", to.Display())
	fmt.Fprintf(a.Out, "    items       %s (%d)\n", scopeLabel, count)
	fmt.Fprintf(a.Out, "    record      #%d %s\n", rec.Seq, rec.Hash)
	fmt.Fprintf(a.Out, "    custody root %s (%d records)\n", info.RootSHA256, info.Records)
	return 0
}

func (a *App) cmdSeal(args []string) int {
	fs := a.newFlagSet("seal", "stratum seal [flags]")
	var workspacePath, note string
	var forceLock bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.StringVar(&note, "note", "", "sealing note")
	fs.BoolVar(&forceLock, "force-lock", false, "break a workspace lock after confirming it is stale")
	p := a.addPersonFlags(fs, false)
	_, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	lock, err := ws.AcquireLock(forceLock, "seal")
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
	if st.Sealed {
		return a.fail("case is already sealed (sealed %s); use stratum unseal --reason \"...\" to reopen it", fmtTime(st.SealedAt))
	}
	operator, err := p.Person(st.Case.Examiner, true)
	if err != nil {
		return a.fail("%v", err)
	}
	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		return a.fail("%v", err)
	}
	sealRec, err := ev.Add("case_sealed", st.Case.CaseID, "stratum", model.CaseSealedData{
		Operator: operator, Note: strings.TrimSpace(note), ItemCount: len(st.Items), Bytes: st.TotalBytes(),
	})
	if err != nil {
		_, _ = ev.Close()
		return a.fail("%v", err)
	}
	evInfo, err := ev.Close()
	if err != nil {
		return a.fail("%v", err)
	}
	holder := custodyHolderSummary(st)
	cu, err := chain.OpenAppend(ws.CustodyPath)
	if err != nil {
		return a.fail("evidence seal recorded (seq %d) but custody ledger could not be opened: %v", sealRec.Seq, err)
	}
	cuRec, err := cu.Add("custody_seal", st.Case.CaseID, "custody", model.CustodyData{
		Action: "sealed", Actor: operator,
		From:    holder,
		Purpose: "case sealed", Note: strings.TrimSpace(note),
		Items: []string{"*"}, ItemCount: len(st.Items), EvidenceRoot: evInfo.RootSHA256,
	})
	if err != nil {
		_, _ = cu.Close()
		return a.fail("evidence seal recorded but custody seal failed: %v", err)
	}
	cuInfo, err := cu.Close()
	if err != nil {
		return a.fail("%v", err)
	}
	fmt.Fprintln(a.Out, "[+] case sealed")
	fmt.Fprintf(a.Out, "    by         %s\n", operator.Display())
	fmt.Fprintf(a.Out, "    items      %d (%s)\n", len(st.Items), humanBytes(st.TotalBytes()))
	fmt.Fprintf(a.Out, "    evidence   #%d %s\n", sealRec.Seq, sealRec.Hash)
	fmt.Fprintf(a.Out, "    custody    #%d %s\n", cuRec.Seq, cuRec.Hash)
	fmt.Fprintf(a.Out, "    evidence root %s\n", evInfo.RootSHA256)
	fmt.Fprintf(a.Out, "    custody root  %s\n", cuInfo.RootSHA256)
	fmt.Fprintln(a.Out, "New acquisitions are blocked; transfers, metadata extraction, verification and reports still work.")
	return 0
}

func (a *App) cmdUnseal(args []string) int {
	fs := a.newFlagSet("unseal", "stratum unseal --reason \"...\" [flags]")
	var workspacePath, reason string
	var forceLock bool
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.StringVar(&reason, "reason", "", "reason for reopening (required)")
	fs.BoolVar(&forceLock, "force-lock", false, "break a workspace lock after confirming it is stale")
	p := a.addPersonFlags(fs, false)
	_, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	if strings.TrimSpace(reason) == "" {
		fmt.Fprintln(a.Err, "stratum: --reason is required; breaking a seal is a documented act")
		fs.Usage()
		return 2
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	lock, err := ws.AcquireLock(forceLock, "unseal")
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
	if !st.Sealed {
		return a.fail("case is not sealed")
	}
	operator, err := p.Person(st.Case.Examiner, true)
	if err != nil {
		return a.fail("%v", err)
	}
	ev, err := chain.OpenAppend(ws.EvidencePath)
	if err != nil {
		return a.fail("%v", err)
	}
	evRec, err := ev.Add("case_unsealed", st.Case.CaseID, "stratum", model.CaseUnsealedData{Operator: operator, Reason: strings.TrimSpace(reason)})
	if err != nil {
		_, _ = ev.Close()
		return a.fail("%v", err)
	}
	evInfo, err := ev.Close()
	if err != nil {
		return a.fail("%v", err)
	}
	holder := custodyHolderSummary(st)
	cu, err := chain.OpenAppend(ws.CustodyPath)
	if err != nil {
		return a.fail("evidence unseal recorded but custody ledger could not be opened: %v", err)
	}
	cuRec, err := cu.Add("custody_unseal", st.Case.CaseID, "custody", model.CustodyData{
		Action: "unsealed", Actor: operator,
		From:    holder,
		Purpose: "seal broken", Note: strings.TrimSpace(reason),
		Items: []string{"*"}, ItemCount: len(st.Items), EvidenceRoot: evInfo.RootSHA256,
	})
	if err != nil {
		_, _ = cu.Close()
		return a.fail("evidence unseal recorded but custody unseal failed: %v", err)
	}
	cuInfo, err := cu.Close()
	if err != nil {
		return a.fail("%v", err)
	}
	fmt.Fprintln(a.Out, "[+] case unsealed")
	fmt.Fprintf(a.Out, "    by         %s\n", operator.Display())
	fmt.Fprintf(a.Out, "    reason     %s\n", reason)
	fmt.Fprintf(a.Out, "    evidence   #%d %s\n", evRec.Seq, evRec.Hash)
	fmt.Fprintf(a.Out, "    custody    #%d %s\n", cuRec.Seq, cuRec.Hash)
	fmt.Fprintf(a.Out, "    evidence root %s\n", evInfo.RootSHA256)
	fmt.Fprintf(a.Out, "    custody root  %s\n", cuInfo.RootSHA256)
	fmt.Fprintln(a.Out, "New acquisitions are allowed again, with a documented break in the seal.")
	return 0
}

func custodyHolderSummary(st *casestate.State) string {
	if st.Ledger == nil {
		return st.Case.Examiner.Display()
	}
	counts := st.Ledger.HolderCounts()
	if len(counts) == 0 {
		return st.Case.Examiner.Display()
	}
	if len(counts) == 1 {
		return counts[0].Holder
	}
	parts := make([]string, 0, len(counts))
	for _, h := range counts {
		parts = append(parts, fmt.Sprintf("%s (%d item(s))", h.Holder, h.Count))
	}
	return "several custodians: " + strings.Join(parts, "; ")
}
