package app

import (
	"fmt"
	"strings"

	"stratum/internal/model"
	"stratum/internal/workspace"
)

func (a *App) cmdInit(args []string) int {
	fs := a.newFlagSet("init", "stratum init [case-name] [flags]")
	var id, dir, description string
	var noRegister, noReadme bool
	fs.StringVar(&id, "id", "", "case identifier")
	fs.StringVar(&dir, "dir", "", "exact workspace directory")
	fs.StringVar(&description, "description", "", "case description")
	fs.BoolVar(&noRegister, "no-register", false, "do not add the case to the local registry")
	fs.BoolVar(&noReadme, "no-readme", false, "do not write the handling README")
	p := a.addPersonFlags(fs, false)

	pos, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	name := strings.TrimSpace(strings.Join(pos, " "))
	if name == "" {
		fmt.Fprintln(a.Err, "stratum: a case name is required")
		fs.Usage()
		return 2
	}
	examiner, err := p.Person(model.Person{}, true)
	if err != nil {
		a.fail("%v", err)
		return 2
	}
	ws, warnings, err := workspace.Create(workspace.CreateOptions{
		Dir:         dir,
		Name:        name,
		CaseID:      id,
		Description: description,
		Examiner:    examiner,
		Register:    !noRegister,
		NoReadme:    noReadme,
	})
	if err != nil {
		return a.fail("%v", err)
	}

	fmt.Fprintln(a.Out, "[+] case workspace created")
	fmt.Fprintf(a.Out, "    case       %s — %s\n", ws.Case.CaseID, ws.Case.Name)
	fmt.Fprintf(a.Out, "    path       %s\n", ws.Root)
	fmt.Fprintf(a.Out, "    examiner   %s\n", ws.Case.Examiner.Display())
	fmt.Fprintf(a.Out, "    evidence   %s\n", ws.EvidencePath)
	fmt.Fprintf(a.Out, "    custody    %s\n", ws.CustodyPath)
	for _, w := range warnings {
		fmt.Fprintf(a.Err, "[!] %s\n", w)
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, "next:")
	fmt.Fprintf(a.Out, "  cd %s\n", shellQuote(ws.Root))
	fmt.Fprintf(a.Out, "  stratum add <source files or directories> --origin \"where they came from\"\n")
	fmt.Fprintf(a.Out, "  stratum report\n")
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, "Seal the case once the evidence set is complete: stratum seal --note \"...\"")
	return 0
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t'\"$`\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
