package app

import (
	"fmt"
	"os"

	"github.com/subsll/stratum/internal/report"
	"github.com/subsll/stratum/internal/verify"
)

func (a *App) cmdReport(args []string) int {
	fs := a.newFlagSet("report", "stratum report [flags]")
	var workspacePath, outDir string
	var noRehash bool
	var formats stringList
	fs.StringVar(&workspacePath, "w", "", "case directory")
	fs.StringVar(&workspacePath, "workspace", "", "case directory")
	fs.Var(&formats, "f", "report format (repeatable or comma-separated)")
	fs.Var(&formats, "format", "report format (repeatable or comma-separated)")
	fs.StringVar(&outDir, "o", "", "exact output directory")
	fs.StringVar(&outDir, "out", "", "exact output directory")
	fs.BoolVar(&noRehash, "no-rehash", false, "skip re-hashing item bytes")
	p := a.addPersonFlags(fs, false)
	_, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	normalized, err := report.NormalizeFormats(formats.values)
	if err != nil {
		return a.fail("%v", err)
	}
	ws, code := a.openWorkspace(workspacePath)
	if code != 0 {
		return code
	}
	st, code := a.loadState(ws)
	if code != 0 {
		return code
	}
	operator, err := p.Person(st.Case.Examiner, false)
	if err != nil {
		return a.fail("%v", err)
	}
	vrep, err := verify.Run(ws, st, !noRehash)
	if err != nil {
		return a.fail("%v", err)
	}
	out, err := report.Write(ws, st, vrep, report.Options{
		Formats:     normalized,
		OutDir:      outDir,
		GeneratedBy: operator,
	})
	if err != nil {
		return a.fail("%v", err)
	}

	fmt.Fprintln(a.Out, "[+] report written")
	fmt.Fprintf(a.Out, "    dir          %s\n", out.Dir)
	fmt.Fprintf(a.Out, "    case         %s — %s\n", st.Case.CaseID, st.Case.Name)
	fmt.Fprintf(a.Out, "    formats      %v\n", normalized)
	fmt.Fprintf(a.Out, "    verification %s\n", reportVerdict(vrep))
	if !vrep.Complete {
		fmt.Fprintln(a.Out, "    note         item bytes were not fully re-hashed; the report is marked partial")
	}
	fmt.Fprintln(a.Out, "    files:")
	for _, name := range out.Files {
		if info, err := os.Stat(out.Dir + string(os.PathSeparator) + name); err == nil {
			fmt.Fprintf(a.Out, "      %-18s %s\n", name, humanBytes(info.Size()))
		} else {
			fmt.Fprintf(a.Out, "      %s\n", name)
		}
	}
	if out.Manifest != nil {
		fmt.Fprintf(a.Out, "    evidence root %s\n", out.Manifest.Evidence.RootSHA256)
		fmt.Fprintf(a.Out, "    custody root  %s\n", out.Manifest.Custody.RootSHA256)
	}
	if !vrep.OK {
		fmt.Fprintln(a.Err, "stratum: the report was written, but integrity checks failed; see the verification section")
		return 1
	}
	return 0
}

func reportVerdict(v *verify.Report) string {
	if v == nil {
		return "unavailable"
	}
	if !v.OK {
		return "FAILED"
	}
	if v.Complete {
		return "PASSED (complete)"
	}
	return "PASSED (partial)"
}
