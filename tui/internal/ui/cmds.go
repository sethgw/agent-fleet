package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sethgw/agent-fleet/tui/internal/adw"
	"github.com/sethgw/agent-fleet/tui/internal/herdr"
)

type boardMsg struct {
	board  *adw.Board
	graphs map[string]*adw.Graph
	err    error
}

type tickMsg struct{}
type screenTickMsg struct{}
type HerdrChangedMsg struct{}

type segDataMsg struct {
	key      string
	wt       *adw.Worktree
	wtErr    error
	ledger   []adw.LedgerEntry
	verdicts []*verdictView
}

type verdictView struct {
	entry adw.LedgerEntry
	v     *adw.Verdict
	err   error
}

type prMsg struct {
	key string
	pr  *adw.PRState
	err error
}

type diffMsg struct {
	key  string
	stat string
	diff string
	err  error
}

type screenMsg struct {
	pane string
	text string
	err  error
}

type flashMsg struct {
	text string
	err  bool
}

func loadBoard() tea.Msg {
	b, err := adw.LoadBoard()
	if err != nil {
		return boardMsg{err: err}
	}
	graphs := map[string]*adw.Graph{}
	for _, e := range b.Epics {
		if g, err := adw.LoadGraph(e.Epic); err == nil {
			graphs[e.Epic] = g
		}
	}
	return boardMsg{board: b, graphs: graphs}
}

func tick(d time.Duration, msg tea.Msg) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msg })
}

func loadSegment(key string, s adw.Segment, base string) tea.Cmd {
	return func() tea.Msg {
		m := segDataMsg{key: key}
		m.wt, m.wtErr = adw.InspectWorktree(s.Worktree, base)
		if m.wt == nil {
			return m
		}
		m.ledger = adw.LoadLedger(s.Repo, m.wt.Branch)
		for _, e := range adw.LatestByReviewer(m.ledger) {
			vv := &verdictView{entry: e}
			if e.Report != "" {
				vv.v, vv.err = adw.LoadVerdict(e.Report)
			}
			m.verdicts = append(m.verdicts, vv)
		}
		return m
	}
}

func loadPR(key, repoDir string, pr int) tea.Cmd {
	return func() tea.Msg {
		s, err := adw.LoadPRState(repoDir, pr)
		return prMsg{key: key, pr: s, err: err}
	}
}

func loadDiff(key string, wt *adw.Worktree, repoDir string, pr int) tea.Cmd {
	return func() tea.Msg {
		m := diffMsg{key: key}
		if wt != nil {
			m.stat, _ = wt.Diff(true)
			m.diff, m.err = wt.Diff(false)
			return m
		}
		if pr > 0 && repoDir != "" {
			m.diff, m.err = adw.PRDiff(repoDir, pr)
			return m
		}
		m.err = fmt.Errorf("no worktree and no PR to diff")
		return m
	}
}

func loadScreen(pane string) tea.Cmd {
	return func() tea.Msg {
		t, err := herdr.ReadPane(pane, 200)
		return screenMsg{pane: pane, text: t, err: err}
	}
}

func focusPane(pane string) tea.Cmd {
	return func() tea.Msg {
		if err := herdr.Focus(pane); err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		return flashMsg{text: "focused " + pane}
	}
}

func promptPane(pane, text string) tea.Cmd {
	return func() tea.Msg {
		st, err := herdr.AgentStatus(pane)
		if err != nil {
			return flashMsg{text: "no agent in " + pane + ": " + err.Error(), err: true}
		}
		if st == "blocked" {
			return flashMsg{text: pane + " is in a dialog; jump to it (enter) to answer", err: true}
		}
		if err := herdr.Prompt(pane, text); err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		return flashMsg{text: "sent to " + pane}
	}
}

func answer(id, choice, note string) tea.Cmd {
	return func() tea.Msg {
		out, err := adw.AnswerDecision(id, choice, note)
		if err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		if out == "" {
			out = fmt.Sprintf("answered %s with (%s)", id, choice)
		}
		return flashMsg{text: firstLine(out)}
	}
}

func snooze(key string, until time.Time) tea.Cmd {
	return func() tea.Msg {
		if err := adw.SetSnooze(key, until); err != nil {
			return flashMsg{text: err.Error(), err: true}
		}
		return flashMsg{text: "snoozed until " + until.Format("Mon 15:04")}
	}
}

func editorCmd(file string, line int) *exec.Cmd {
	ed := os.Getenv("VISUAL")
	if ed == "" {
		ed = os.Getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
	}
	parts := strings.Fields(ed)
	switch filepath.Base(parts[0]) {
	case "code", "cursor", "windsurf", "zed":
		if line > 0 {
			return exec.Command(parts[0], append(parts[1:], "-g", fmt.Sprintf("%s:%d", file, line))...)
		}
	default:
		if line > 0 {
			return exec.Command(parts[0], append(parts[1:], fmt.Sprintf("+%d", line), file)...)
		}
	}
	return exec.Command(parts[0], append(parts[1:], file)...)
}

// pagerCmd shows text in $PAGER (less -R by default), through delta when it is installed.
func pagerCmd(text string, isDiff bool) (*exec.Cmd, error) {
	f, err := os.CreateTemp("", "adw-tui-*.diff")
	if err != nil {
		return nil, err
	}
	_, _ = f.WriteString(text)
	f.Close()
	q := shellQuote(f.Name())
	if isDiff {
		if _, err := exec.LookPath("delta"); err == nil {
			return exec.Command("sh", "-c", "delta --paging=always < "+q+"; rm -f "+q), nil
		}
	}
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less -R"
	}
	return exec.Command("sh", "-c", pager+" "+q+"; rm -f "+q), nil
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func openURL(url string) tea.Cmd {
	return func() tea.Msg {
		for _, c := range [][]string{{"wslview", url}, {"xdg-open", url}, {"open", url}} {
			if _, err := exec.LookPath(c[0]); err == nil {
				if err := exec.Command(c[0], c[1:]...).Start(); err == nil {
					return flashMsg{text: "opened " + url}
				}
			}
		}
		return flashMsg{text: url, err: true}
	}
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}
