package app

import (
	"fmt"
	"strings"

	"github.com/subsll/stratum/internal/casestate"
	"github.com/subsll/stratum/internal/workspace"
)

type workspaceRow struct {
	Path    string `json:"path"`
	CaseID  string `json:"case_id"`
	Name    string `json:"name"`
	Created string `json:"created,omitempty"`
	Status  string `json:"status"`
	Items   int    `json:"items"`
	Sealed  bool   `json:"sealed"`
	Error   string `json:"error,omitempty"`
}

func (a *App) cmdWorkspaces(args []string) int {
	fs := a.newFlagSet("workspaces", "stratum workspaces [flags]")
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "machine-readable output")
	_, code := a.parseFlags(fs, args)
	if code >= 0 {
		return code
	}
	entries, err := workspace.ListRegistry()
	if err != nil {
		return a.fail("%v", err)
	}
	if len(entries) == 0 {
		if asJSON {
			return a.writeJSON([]workspaceRow{})
		}
		fmt.Fprintln(a.Out, "No case workspaces are registered.")
		fmt.Fprintln(a.Out, `Create one with: stratum init "Case name" --examiner "Your Name"`)
		return 0
	}
	rows := make([]workspaceRow, 0, len(entries))
	for _, entry := range entries {
		row := workspaceRow{Path: entry.Path, CaseID: entry.CaseID, Name: entry.Name, Created: entry.Created, Status: "missing"}
		ws, err := workspace.Open(entry.Path)
		if err != nil {
			row.Error = err.Error()
			rows = append(rows, row)
			continue
		}
		st, err := casestate.Load(ws)
		if err != nil {
			row.Status = "unreadable"
			row.Error = err.Error()
			rows = append(rows, row)
			continue
		}
		row.CaseID = st.Case.CaseID
		row.Name = st.Case.Name
		row.Created = st.Case.Created
		row.Items = len(st.Items)
		row.Sealed = st.Sealed
		row.Status = "ok"
		if !st.ChainsOK() {
			row.Status = "broken"
		}
		rows = append(rows, row)
	}
	if asJSON {
		return a.writeJSON(rows)
	}
	fmt.Fprintf(a.Out, "%-9s %-18s %-6s %-7s %-20s %s\n", "STATUS", "CASE ID", "ITEMS", "SEALED", "CREATED (UTC)", "PATH")
	for _, row := range rows {
		created := row.Created
		if len(created) > 20 {
			created = created[:19] + "Z"
		}
		fmt.Fprintf(a.Out, "%-9s %-18s %-6d %-7t %-20s %s\n",
			row.Status, truncate(row.CaseID, 18), row.Items, row.Sealed, created, row.Path)
		if row.Error != "" {
			fmt.Fprintf(a.Out, "          %s\n", strings.ReplaceAll(row.Error, "\n", " "))
		}
	}
	return 0
}
