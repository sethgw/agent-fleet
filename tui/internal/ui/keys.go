package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sethgw/agent-fleet/tui/internal/adw"
)

func (m *Model) key(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch m.mode {
	case modeHelp:
		m.mode = modeNormal
		return m, nil
	case modeReply, modeAnswerNote:
		return m.inputKey(msg)
	case modeAnswerPick:
		return m.pickKey(k)
	}

	it := m.current()
	switch k {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
		return m, nil
	case "tab":
		m.detailFocus = !m.detailFocus
		return m, nil
	case "esc":
		m.detailFocus = false
		return m, nil
	case "R", "ctrl+r":
		return m, tea.Batch(m.requestBoard(), m.onSelect(true))
	case "J", "pgdown", "ctrl+d":
		m.vp.HalfPageDown()
		return m, nil
	case "K", "pgup", "ctrl+u":
		m.vp.HalfPageUp()
		return m, nil
	case "1", "2", "3", "4":
		if it != nil && it.kind == kSegment {
			return m, m.setTab(segTab(k[0] - '1'))
		}
	case "[", "h":
		if it != nil && it.kind == kSegment {
			return m, m.setTab((m.tab + 3) % 4)
		}
	case "]", "l":
		if it != nil && it.kind == kSegment {
			return m, m.setTab((m.tab + 1) % 4)
		}
	}

	if m.detailFocus {
		switch k {
		case "j", "down":
			if m.reviewCursorActive() {
				m.findingSel = min(m.findingSel+1, len(m.findingRows)-1)
				m.refreshDetail()
				m.ensureVisible(m.findingRows[m.findingSel])
			} else {
				m.vp.ScrollDown(1)
			}
			return m, nil
		case "k", "up":
			if m.reviewCursorActive() {
				m.findingSel = max(m.findingSel-1, 0)
				m.refreshDetail()
				m.ensureVisible(m.findingRows[m.findingSel])
			} else {
				m.vp.ScrollUp(1)
			}
			return m, nil
		case "g", "home":
			m.vp.GotoTop()
			return m, nil
		case "G", "end":
			m.vp.GotoBottom()
			return m, nil
		}
	} else {
		switch k {
		case "j", "down":
			return m, m.selectDelta(1)
		case "k", "up":
			return m, m.selectDelta(-1)
		case "g", "home":
			m.cursor = 0
			return m, m.selectDelta(0)
		case "G", "end":
			m.cursor = len(m.items) - 1
			return m, m.selectDelta(0)
		case "n":
			return m, m.nextNeedsYou()
		}
	}

	if it == nil {
		return m, nil
	}
	return m.action(k, it)
}

func (m *Model) selectDelta(d int) tea.Cmd {
	m.moveCursor(d)
	m.findingSel = 0
	m.vp.GotoTop()
	m.refreshDetail()
	if it := m.current(); it != nil && (it.kind == kAgent || it.kind == kDialog) {
		m.vp.GotoBottom()
	}
	return m.onSelect(false)
}

// nextNeedsYou jumps to the next decision or dialog, wrapping around.
func (m *Model) nextNeedsYou() tea.Cmd {
	for off := 1; off <= len(m.items); off++ {
		i := (m.cursor + off) % len(m.items)
		if k := m.items[i].kind; k == kDecision || k == kDialog {
			m.cursor = i
			return m.selectDelta(0)
		}
	}
	return nil
}

func (m *Model) setTab(t segTab) tea.Cmd {
	m.tab = t
	m.findingSel = 0
	m.vp.GotoTop()
	m.refreshDetail()
	if t == tabDiff {
		return m.diffCmd(false)
	}
	return nil
}

func (m *Model) reviewCursorActive() bool {
	it := m.current()
	return it != nil && it.kind == kSegment && m.tab == tabReviews && len(m.findingRows) > 0
}

func (m *Model) ensureVisible(line int) {
	if line < m.vp.YOffset {
		m.vp.SetYOffset(max(0, line-1))
	} else if line >= m.vp.YOffset+m.vp.Height-1 {
		m.vp.SetYOffset(line - m.vp.Height + 3)
	}
}

func (m *Model) action(k string, it *item) (tea.Model, tea.Cmd) {
	switch it.kind {
	case kDecision:
		switch k {
		case "a", "enter":
			m.mode = modeAnswerPick
			m.pickSel = 0
			for i, o := range it.dec.Options {
				if o.Key == it.dec.Recommend {
					m.pickSel = i
				}
			}
			return m, nil
		case "f":
			if it.dec.Pane != "" {
				return m, focusPane(it.dec.Pane)
			}
		case "r":
			return m, m.startReply(it.dec.Pane, "asking tab "+it.dec.Pane)
		case "z":
			return m, snooze("d:"+it.dec.ID, time.Now().Add(time.Hour))
		case "Z":
			return m, snooze("d:"+it.dec.ID, adw.Tomorrow8())
		case "v":
			return m.page(decisionText(it.dec), false)
		}
	case kDialog, kAgent:
		switch k {
		case "enter", "f":
			return m, focusPane(it.agent.Pane)
		case "r":
			return m, m.startReply(it.agent.Pane, it.agent.Pane+" "+it.agent.Title)
		case "z":
			if it.kind == kDialog {
				return m, snooze(dialogKey(it.agent), time.Now().Add(time.Hour))
			}
		case "Z":
			if it.kind == kDialog {
				return m, snooze(dialogKey(it.agent), adw.Tomorrow8())
			}
		case "v":
			if s := m.screens[it.agent.Pane]; s != nil {
				return m.page(s.text, false)
			}
		}
	case kSegment:
		return m.segmentAction(k, it)
	case kDigest:
		if k == "v" || k == "enter" {
			return m.page(m.board.Mate.Text, false)
		}
	}
	return m, nil
}

func (m *Model) segmentAction(k string, it *item) (tea.Model, tea.Cmd) {
	s := it.seg
	sd := m.segData[it.key]
	switch k {
	case "enter":
		if m.reviewCursorActive() && m.detailFocus {
			return m.openFinding()
		}
		if sd != nil && sd.wt != nil {
			if as := m.agentsIn(sd.wt.Path); len(as) > 0 {
				return m, focusPane(as[0].Pane)
			}
		}
		if lead := m.epicLead(s.Epic); lead != "" {
			return m, focusPane(lead)
		}
		return m, flash("no tab works in this segment", true)
	case "e":
		if m.reviewCursorActive() {
			return m.openFinding()
		}
		if p := s.BriefPath(); p != "" {
			return m, tea.ExecProcess(editorCmd(p, 0), execDone)
		}
	case "d", "D":
		if d := m.diffs[it.key]; d != nil && d.err == nil {
			return m.page(d.stat+"\n"+d.diff, true)
		}
		return m, m.setTab(tabDiff)
	case "o":
		if p := m.prs[it.key]; p != nil && p.pr != nil && p.pr.URL != "" {
			return m, openURL(p.pr.URL)
		}
		return m, flash("no PR state loaded yet", true)
	case "r":
		lead := m.epicLead(s.Epic)
		if lead == "" {
			return m, flash("epic "+s.Epic+" has no live lead", true)
		}
		prefix := fmt.Sprintf("[review of segment %s", s.ID)
		if pr := s.PRNumber(); pr > 0 {
			prefix += fmt.Sprintf(" #%d", pr)
		}
		if f := m.selectedFinding(); f != nil && m.tab == tabReviews {
			prefix += fmt.Sprintf(", finding %s:%d", f.file, f.line)
		}
		m.replyHint = prefix + "] "
		return m, m.startReply(lead, "lead of "+s.Epic+" ("+lead+")")
	case "c":
		if sd != nil && sd.wt != nil {
			if as := m.agentsIn(sd.wt.Path); len(as) > 0 {
				m.replyHint = fmt.Sprintf("[review of segment %s] ", s.ID)
				return m, m.startReply(as[0].Pane, "builder "+as[0].Pane)
			}
		}
		return m, flash("no tab is working in this segment's worktree", true)
	}
	return m, nil
}

func (m *Model) selectedFinding() *findingRef {
	if m.findingSel >= 0 && m.findingSel < len(m.findingRefs) {
		return &m.findingRefs[m.findingSel]
	}
	return nil
}

func (m *Model) openFinding() (tea.Model, tea.Cmd) {
	it := m.current()
	f := m.selectedFinding()
	sd := m.segData[it.key]
	if f == nil || sd == nil || sd.wt == nil {
		return m, flash("no worktree to open the finding in", true)
	}
	path := f.file
	if !filepath.IsAbs(path) {
		path = filepath.Join(sd.wt.Path, path)
	}
	return m, tea.ExecProcess(editorCmd(path, f.line), execDone)
}

func (m *Model) page(text string, isDiff bool) (tea.Model, tea.Cmd) {
	cmd, err := pagerCmd(text, isDiff)
	if err != nil {
		return m, flash(err.Error(), true)
	}
	return m, tea.ExecProcess(cmd, execDone)
}

func (m *Model) startReply(pane, to string) tea.Cmd {
	if pane == "" {
		return flash("nobody to send to", true)
	}
	m.mode = modeReply
	m.replyTo = pane
	m.input.Prompt = "to " + truncate(to, 40) + " › "
	m.input.Placeholder = "message (enter sends, esc cancels)"
	m.input.SetValue(m.replyHint)
	m.input.CursorEnd()
	m.replyHint = ""
	return m.input.Focus()
}

func (m *Model) inputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "enter":
		text := strings.TrimSpace(m.input.Value())
		m.input.Blur()
		mode := m.mode
		m.mode = modeNormal
		if mode == modeAnswerNote {
			it := m.current()
			if it == nil || it.kind != kDecision {
				return m, nil
			}
			return m, answer(it.dec.ID, it.dec.Options[m.pickSel].Key, text)
		}
		if text == "" {
			return m, nil
		}
		return m, promptPane(m.replyTo, text)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) pickKey(k string) (tea.Model, tea.Cmd) {
	it := m.current()
	if it == nil || it.kind != kDecision || len(it.dec.Options) == 0 {
		m.mode = modeNormal
		return m, nil
	}
	opts := it.dec.Options
	switch k {
	case "esc", "q":
		m.mode = modeNormal
	case "j", "down":
		m.pickSel = min(m.pickSel+1, len(opts)-1)
	case "k", "up":
		m.pickSel = max(m.pickSel-1, 0)
	case "enter":
		m.mode = modeAnswerNote
		m.input.Prompt = fmt.Sprintf("(%s) note › ", opts[m.pickSel].Key)
		m.input.Placeholder = "optional note to the tab (enter answers, esc cancels)"
		m.input.SetValue("")
		return m, m.input.Focus()
	default:
		for i, o := range opts {
			if o.Key == k {
				m.pickSel = i
			}
		}
	}
	m.refreshDetail()
	return m, nil
}

func flash(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return flashMsg{text: text, err: isErr} }
}
