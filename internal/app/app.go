// Package app wires the Stratum commands into a testable CLI. cmd/stratum is a
// thin os.Exit wrapper around Run.
package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/subsll/stratum/internal/casestate"
	"github.com/subsll/stratum/internal/model"
	"github.com/subsll/stratum/internal/workspace"
)

// App holds the output streams and terminal capability for one invocation.
type App struct {
	Out   io.Writer
	Err   io.Writer
	Color bool
}

// Run executes a command line and returns a process exit status.
func Run(args []string, out, errOut io.Writer) int {
	a := &App{Out: out, Err: errOut, Color: colorFor(out)}
	if len(args) == 0 {
		a.printWelcome()
		return 0
	}
	switch args[0] {
	case "init", "new", "case":
		return a.cmdInit(args[1:])
	case "add", "acquire":
		return a.cmdAdd(args[1:])
	case "list", "ls", "items":
		return a.cmdList(args[1:])
	case "show", "inspect":
		return a.cmdShow(args[1:])
	case "extract", "metadata":
		return a.cmdExtract(args[1:])
	case "custody", "chain":
		return a.cmdCustody(args[1:])
	case "seal":
		return a.cmdSeal(args[1:])
	case "unseal":
		return a.cmdUnseal(args[1:])
	case "verify":
		return a.cmdVerify(args[1:])
	case "report":
		return a.cmdReport(args[1:])
	case "workspaces", "cases", "workspace":
		return a.cmdWorkspaces(args[1:])
	case "version", "--version", "-version", "-V":
		fmt.Fprintf(a.Out, "%s %s (%s %s/%s)\n", model.ToolName, model.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	case "help", "-h", "--help":
		return a.cmdHelp(args[1:])
	}
	fmt.Fprintf(a.Err, "stratum: unknown command %q\n\n", args[0])
	a.printWelcome()
	return 2
}

func (a *App) cmdHelp(args []string) int {
	if len(args) == 0 {
		a.printWelcome()
		return 0
	}
	topic := strings.ToLower(strings.TrimSpace(args[0]))
	text, ok := helpTexts[topic]
	if !ok {
		fmt.Fprintf(a.Err, "stratum: no help for %q\n", args[0])
		fmt.Fprintln(a.Err, "topics: init, add, list, show, extract, custody, seal, unseal, verify, report, workspaces")
		return 2
	}
	fmt.Fprint(a.Out, text)
	return 0
}

func (a *App) newFlagSet(name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Err)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: %s\n\noptions:\n", usage)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses interspersed flags and positionals. Exit -1 means continue,
// 0 means help was shown, 2 means a usage error.
func (a *App) parseFlags(fs *flag.FlagSet, args []string) ([]string, int) {
	pos, err := parseInterspersed(fs, args)
	if err == nil {
		return pos, -1
	}
	if errors.Is(err, flag.ErrHelp) {
		fs.SetOutput(a.Out)
		fs.Usage()
		fs.SetOutput(a.Err)
		return nil, 0
	}
	fmt.Fprintf(a.Err, "stratum %s: %v\n", fs.Name(), err)
	fs.Usage()
	return nil, 2
}

func (a *App) openWorkspace(flagPath string) (*workspace.Workspace, int) {
	ws, err := workspace.Resolve(flagPath)
	if err != nil {
		fmt.Fprintf(a.Err, "stratum: %v\n", err)
		fmt.Fprintln(a.Err, "hint: run inside a case directory, use --workspace <path>, or create one with: stratum init \"Case name\" --examiner \"Name\"")
		return nil, 1
	}
	return ws, 0
}

func (a *App) loadState(ws *workspace.Workspace) (*casestate.State, int) {
	st, err := casestate.Load(ws)
	if err != nil {
		fmt.Fprintf(a.Err, "stratum: %v\n", err)
		return nil, 1
	}
	return st, 0
}

func (a *App) chainsReady(st *casestate.State) bool {
	if st.ChainsOK() {
		return true
	}
	fmt.Fprintln(a.Err, "stratum: refusing to continue because a ledger failed verification:")
	for _, e := range st.ChainErrors() {
		fmt.Fprintf(a.Err, "  - %s\n", e)
	}
	fmt.Fprintln(a.Err, "run: stratum verify --no-rehash")
	return false
}

func (a *App) printChainStatus(st *casestate.State) {
	mark := func(ok bool) string {
		if ok {
			return "OK"
		}
		return "BROKEN"
	}
	fmt.Fprintf(a.Out, "  evidence chain  %-6s  %d record(s)  root %s\n", mark(st.EvidenceOK()), st.EvidenceInfo().Records, short(st.EvidenceInfo().RootSHA256))
	fmt.Fprintf(a.Out, "  custody chain   %-6s  %d record(s)  root %s\n", mark(st.CustodyOK()), st.CustodyInfo().Records, short(st.CustodyInfo().RootSHA256))
}

func (a *App) fail(format string, args ...any) int {
	fmt.Fprintf(a.Err, "stratum: "+format+"\n", args...)
	return 1
}

func short(s string) string {
	if len(s) <= 20 {
		return s
	}
	return s[:20] + "…"
}

func colorFor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}
