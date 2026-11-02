package app

import (
	"fmt"
	"strings"

	"stratum/internal/model"
)

var banner = []string{
	"███████╗████████╗██████╗  █████╗ ████████╗██╗   ██╗███╗   ███╗",
	"██╔════╝╚══██╔══╝██╔══██╗██╔══██╗╚══██╔══╝██║   ██║████╗ ████║",
	"███████╗   ██║   ██████╔╝███████║   ██║   ██║   ██║██╔████╔██║",
	"╚════██║   ██║   ██╔══██╗██╔══██║   ██║   ██║   ██║██║╚██╔╝██║",
	"███████║   ██║   ██║  ██║██║  ██║   ██║   ╚██████╔╝██║ ╚═╝ ██║",
	"╚══════╝   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚═╝     ╚═╝",
}

type painter struct{ on bool }

func (p painter) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return code + s + "\x1b[0m"
}
func (p painter) bold(s string) string   { return p.wrap("\x1b[1m", s) }
func (p painter) dim(s string) string    { return p.wrap("\x1b[2m", s) }
func (p painter) cyan(s string) string   { return p.wrap("\x1b[36m", s) }
func (p painter) green(s string) string  { return p.wrap("\x1b[32m", s) }
func (p painter) yellow(s string) string { return p.wrap("\x1b[33m", s) }
func (p painter) red(s string) string    { return p.wrap("\x1b[31m", s) }

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func (a *App) printWelcome() {
	p := painter{on: a.Color}
	w := a.Out
	fmt.Fprintln(w)
	for _, line := range banner {
		fmt.Fprintln(w, "  "+p.bold(p.cyan(line)))
	}
	fmt.Fprintf(w, "  %s\n\n", p.dim("Stratum  ·  case evidence & chain-of-custody workspaces  ·  v"+model.Version))

	commands := [][2]string{
		{"init <case-name>", "create a case workspace"},
		{"add <path>...", "hash, identify and preserve files"},
		{"list", "items and current custodians"},
		{"show <item-id>", "one item and its custody history"},
		{"extract [--all]", "add/refresh metadata sidecars for existing items"},
		{"custody transfer", "record a handover; list / return too"},
		{"seal / unseal", "close or reopen the evidence set"},
		{"verify", "verify chains and re-hash items"},
		{"report", "text, Markdown, HTML, JSON, CSV"},
		{"workspaces", "list registered cases"},
		{"version / help", "build info / this screen"},
	}
	cmdW := 0
	for _, c := range commands {
		if len(c[0]) > cmdW {
			cmdW = len(c[0])
		}
	}
	descW := 0
	for _, c := range commands {
		if len(c[1]) > descW {
			descW = len(c[1])
		}
	}
	inner := cmdW + descW + 4
	rule := strings.Repeat("─", inner)
	fmt.Fprintf(w, "  %s\n", p.dim("╭"+rule+"╮"))
	for _, c := range commands {
		fmt.Fprintf(w, "  %s %s  %s %s\n",
			p.dim("│"), p.bold(padRight(c[0], cmdW)), padRight(c[1], descW), p.dim("│"))
	}
	fmt.Fprintf(w, "  %s\n", p.dim("╰"+rule+"╯"))

	fmt.Fprintf(w, "\n  %s\n", p.bold("QUICK START"))
	examples := [][2]string{
		{`stratum init "Case" --examiner "Name"`, "create a case and its first custodian"},
		{`stratum add ./collected`, "hash and preserve files"},
		{`stratum list`, "see items, hashes, types and holders"},
		{`stratum custody transfer --to "Name"`, "document a handover"},
		{`stratum seal --note "closed"`, "freeze the evidence set"},
		{`stratum report`, "generate every report format"},
		{`stratum verify`, "re-check chains and item bytes"},
	}
	exW := 0
	for _, e := range examples {
		if len(e[0]) > exW {
			exW = len(e[0])
		}
	}
	for _, e := range examples {
		fmt.Fprintf(w, "    %s  %s\n", p.cyan(padRight(e[0], exW)), p.dim(e[1]))
	}

	fmt.Fprintf(w, "\n  %s\n", p.bold("HOW IT WORKS"))
	fmt.Fprintf(w, "    %s  what and when: every file hash, type, EXIF finding and action.\n", p.bold("Chain of evidence"))
	fmt.Fprintf(w, "    %s   who: every collection, handover, return, seal and verification.\n", p.bold("Chain of custody"))
	fmt.Fprintf(w, "    Copies are read back and verified, made read-only, and the source is never written.\n")

	fmt.Fprintf(w, "\n  %s\n", p.bold("NEXT"))
	fmt.Fprintf(w, "    %s\n", p.green(`stratum init "Case name" --examiner "Your Name" --org "Your Organisation"`))
	fmt.Fprintf(w, "    %s\n", p.dim(`stratum help add   ·   stratum help custody   ·   stratum help report`))

	fmt.Fprintf(w, "\n  %s\n", p.yellow("Use only for material you are authorised to handle."))
	fmt.Fprintf(w, "  %s\n\n", p.dim("Stratum records evidence; it does not decide legal admissibility."))
}

var helpTexts = map[string]string{
	"init": `stratum init [case-name] [flags]

Create a new case workspace containing case.json, evidence.jsonl,
custody.jsonl, items/ and reports/. The examiner becomes the first custodian.

Options:
  --id ID             case identifier (default CASE-YYYYMMDD-XXXXXX)
  --dir PATH          exact workspace directory (default ./<id>-<slug>)
  --examiner NAME     examiner name (or STRATUM_EXAMINER)
  --org NAME          examiner organisation (or STRATUM_ORG)
  --email ADDR        examiner email (or STRATUM_EMAIL)
  --description TEXT  case description
  --no-register       do not add the case to the local registry
  --no-readme         do not write the handling README

Examples:
  stratum init "Smith v Acme" --examiner "Jane Doe"
  stratum init --id CASE-2026-001 --dir ./cases/acme --examiner "Jane Doe"
`,
	"add": `stratum add <path>... [flags]

Acquire files or directories into the current case. Each source is opened
read-only; regular files are copied into items/, read back, SHA-256 verified,
timestamp-preserved and made read-only. Symlinks are not followed.

Options:
  -w, --workspace PATH  case directory (default: nearest case, or STRATUM_WORKSPACE)
  --reference           record a hash only; do not preserve a copy
  --copy=false          alias for --reference semantics
  --exclude PATTERN     skip a shell-style glob (repeatable; matches basename or path)
  --origin TEXT         provenance note stored with every item in this batch
  --note TEXT           free-text note stored with every item
  --skip-hidden         skip dot-files and dot-directories
  --examiner NAME       operator (default: case examiner)
  --org NAME            operator organisation
  --email ADDR          operator email
  --force-lock          break a workspace lock after confirming no other run is active
  -v, --verbose         per-file detail

Examples:
  stratum add ./collected/web ./collected/docs --origin "client delivery 2026-11-02"
  stratum add ./large-video.mp4 --reference --note "hash only; file left on source media"
`,
	"list": `stratum list [flags]

List evidence items, types, sizes, acquisition times and current holders.

Options:
  -w, --workspace PATH  case directory
  --hash                show each item's SHA-256
  --json                machine-readable output

Aliases: ls, items
`,
	"show": `stratum show <item-id> [flags]

Show one item in full: paths, hashes, content type, EXIF metadata, integrity
status and its custody history.

Options:
  -w, --workspace PATH  case directory
  --json                machine-readable output
`,
	"custody": `stratum custody [list] [flags]
stratum custody transfer --to "Name" [flags]
stratum custody return --to "Name" [flags]

The chain of custody records who held which items, when, and through which
documented handover. Every record anchors the current evidence-chain root.

Options for list:
  -w, --workspace PATH  case directory
  --json                machine-readable output

Options for transfer / return:
  -w, --workspace PATH  case directory
  --to NAME             recipient name (required)
  --to-org NAME         recipient organisation
  --to-email ADDR       recipient email
  --from TEXT           expected current custodian (validated)
  --item ID             item to move (repeatable; default: every item)
  --method TEXT         e.g. "hand delivery", "courier", "encrypted transfer"
  --purpose TEXT        reason for the handover
  --location TEXT       place of handover
  --note TEXT           free-text note
  --examiner NAME       person recording the handover (alias: --by)
  --org NAME            their organisation
  --email ADDR          their email
  --force-lock          break a workspace lock after confirmation

Examples:
  stratum custody transfer --to "Bob Smith" --to-org "Acme Legal" --purpose "production to counsel"
  stratum custody return --to "Jane Doe" --item E-0003 --method "hand delivery"
`,
	"extract": `stratum extract [--all | <item-id>...] [flags]

Extract EXIF and document metadata for existing items and write a per-item
metadata sidecar under metadata/. The sidecar's SHA-256 is appended to the
evidence chain as an item_metadata record.

Options:
  -w, --workspace PATH  case directory
  --all                 every item in the case
  --force               re-extract and replace an existing sidecar
  --examiner NAME       person recording the extraction
  --org NAME            their organisation
  --email ADDR          their email
  --force-lock          break a workspace lock after confirming it is stale

Example:
  stratum extract --all
  stratum extract E-0003 E-0007 --force
`,
	"seal": `stratum seal [flags]

Append a seal record to both ledgers. A sealed case refuses new acquisitions;
custody transfers, verification and reports remain possible. Use unseal only
with a documented reason.

Options:
  -w, --workspace PATH  case directory
  --note TEXT           sealing note (recommended)
  --examiner NAME       person sealing the case
  --org NAME            their organisation
  --email ADDR          their email
  --force-lock          break a workspace lock after confirmation
`,
	"unseal": `stratum unseal --reason "..." [flags]

Break the seal in a documented, append-only way. The reason is required and is
written to both ledgers.

Options:
  -w, --workspace PATH  case directory
  --reason TEXT         reason for reopening (required)
  --examiner NAME       person reopening the case
  --org NAME            their organisation
  --email ADDR          their email
  --force-lock          break a workspace lock after confirmation
`,
	"verify": `stratum verify [evidence.jsonl] [flags]

Verify the evidence chain, the custody chain, the cross-links and, by default,
re-hash every preserved item against its recorded SHA-256. A path ending in
.jsonl verifies one ledger standalone.

Options:
  -w, --workspace PATH  case directory
  --no-rehash           check both chains and cross-links only
  --record              append a signed-by-record verification event (requires healthy chains)
  --json                machine-readable report
  --examiner NAME       operator for --record (default: case examiner)
  --org NAME            operator organisation
  --email ADDR          operator email
  --force-lock          break a workspace lock after confirmation

Exit status: 0 if checks pass, 1 if they fail.
`,
	"report": `stratum report [flags]

Generate a case report. Formats: text, md (Markdown), html (standalone,
print-friendly), json (complete audit copy), csv (items, evidence, custody).
Reports are written to reports/<UTC timestamp>/ unless --out is given.

Options:
  -w, --workspace PATH  case directory
  -f, --format LIST     formats, comma-separated or repeatable (default: all)
  -o, --out PATH        exact output directory
  --no-rehash           skip re-hashing item bytes (report is marked partial)
  --examiner NAME       "generated by" person (default: case examiner)
  --org NAME            their organisation
  --email ADDR          their email

Every report directory gets MANIFEST.json and SHA256SUMS.

Examples:
  stratum report
  stratum report -f html,json
  stratum report -f md --out ./delivery
`,
	"workspaces": `stratum workspaces [flags]

List case workspaces in the local registry (~/.stratum/workspaces.json, or
STRATUM_HOME). A case remains valid if the registry is lost; this is only a
convenience index.

Options:
  --json  machine-readable output

Aliases: cases, workspace
`,
}
