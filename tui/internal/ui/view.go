package ui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sethgw/agent-fleet/tui/internal/adw"
)

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#5a4fcf", Dark: "#a99bff"}
	cDim    = lipgloss.AdaptiveColor{Light: "#7a7a7a", Dark: "#8a8a8a"}
	cRed    = lipgloss.AdaptiveColor{Light: "#c0392b", Dark: "#ff6b6b"}
	cGreen  = lipgloss.AdaptiveColor{Light: "#1e8449", Dark: "#5fd38d"}
	cYellow = lipgloss.AdaptiveColor{Light: "#9a7d0a", Dark: "#f4d35e"}
	cCyan   = lipgloss.AdaptiveColor{Light: "#117a8b", Dark: "#5ccfe6"}
	cBlue   = lipgloss.AdaptiveColor{Light: "#2e5aac", Dark: "#7aa2f7"}
	cSelBg  = lipgloss.AdaptiveColor{Light: "#e4e0ff", Dark: "#2f2a4a"}

	sBold   = lipgloss.NewStyle().Bold(true)
	sDim    = lipgloss.NewStyle().Foreground(cDim)
	sHead   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sTitle  = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sErr    = lipgloss.NewStyle().Foreground(cRed)
	sOK     = lipgloss.NewStyle().Foreground(cGreen)
	sWarn   = lipgloss.NewStyle().Foreground(cYellow)
	sCode   = lipgloss.NewStyle().Foreground(cCyan)
	sSel    = lipgloss.NewStyle().Background(cSelBg)
	sTabOn  = lipgloss.NewStyle().Bold(true).Foreground(cAccent).Underline(true)
	sTabOff = lipgloss.NewStyle().Foreground(cDim)
)

func statusStyle(s string) lipgloss.Style {
	switch s {
	case "blocked", "fail", "failed", "overdue", "high", "critical":
		return sErr
	case "pr-ready", "done", "pass", "merged", "finished":
		return sOK
	case "building", "working", "waiting", "med", "important", "asks":
		return sWarn
	case "pr-open", "background":
		return lipgloss.NewStyle().Foreground(cCyan)
	case "ready", "low", "suggestion":
		return lipgloss.NewStyle().Foreground(cBlue)
	}
	return sDim
}

func (m *Model) View() string {
	if m.w == 0 {
		return ""
	}
	if m.mode == modeHelp {
		return helpText
	}
	lw := m.listWidth()
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderList(lw, m.h-3),
		sDim.Render(strings.Repeat("│\n", m.h-3)),
		" "+m.renderDetailHeader()+"\n "+indentLines(m.vp.View(), " "),
	)
	return m.renderHeader() + "\n" + body + "\n" + m.renderFooter()
}

func indentLines(s, pre string) string { return strings.ReplaceAll(s, "\n", "\n"+pre) }

func (m *Model) renderHeader() string {
	parts := []string{sTitle.Render("adw")}
	if m.board != nil {
		needs := 0
		for _, it := range m.items {
			if it.kind == kDecision || it.kind == kDialog {
				needs++
			}
		}
		if needs > 0 {
			parts = append(parts, sErr.Bold(true).Render(fmt.Sprintf("%d needs you", needs)))
		} else {
			parts = append(parts, sOK.Render("nothing needs you"))
		}
		working := 0
		for _, a := range m.board.Agents {
			if a.Status == "working" {
				working++
			}
		}
		parts = append(parts, fmt.Sprintf("%d epics", len(m.board.Epics)),
			fmt.Sprintf("%d tabs (%d working)", len(m.board.Agents), working))
	}
	if m.loadErr != nil {
		parts = append(parts, sErr.Render(truncate(m.loadErr.Error(), 80)))
	} else if !m.loadedAt.IsZero() {
		parts = append(parts, sDim.Render("updated "+m.loadedAt.Format("15:04:05")))
	}
	return truncate(" "+strings.Join(parts, sDim.Render(" · ")), m.w)
}

func (m *Model) renderFooter() string {
	switch m.mode {
	case modeReply, modeAnswerNote:
		return " " + m.input.View()
	case modeAnswerPick:
		return sDim.Render(" j/k or letter pick · enter confirm · esc cancel")
	}
	if m.flash != "" && time.Since(m.flashAt) < 8*time.Second {
		st := sOK
		if m.flashErr {
			st = sErr
		}
		return " " + st.Render(truncate(m.flash, m.w-2))
	}
	var hint string
	it := m.current()
	switch {
	case it == nil:
	case it.kind == kDecision:
		hint = "enter answer · f jump to tab · r reply · v page · z/Z snooze"
	case it.kind == kDialog || it.kind == kAgent:
		hint = "enter jump to tab · r reply · v page screen"
	case it.kind == kSegment && m.tab == tabReviews && m.detailFocus:
		hint = "j/k finding · enter/e open in editor · r note to lead re finding · tab back"
	case it.kind == kSegment:
		hint = "1-4/[ ] tabs · d diff in pager · e edit brief · o open PR · r note to lead · c note to builder · enter jump"
	}
	focus := "list"
	if m.detailFocus {
		focus = "detail"
	}
	return sDim.Render(truncate(fmt.Sprintf(" [%s] %s · tab focus · n next · ? help · q quit", focus, hint), m.w))
}

func (m *Model) renderList(w, h int) string {
	lines := make([]string, len(m.items))
	for i, it := range m.items {
		lines[i] = m.listRow(it, w-1)
		if i == m.cursor {
			st := sSel
			if !m.detailFocus {
				st = st.Bold(true)
			}
			lines[i] = st.Render(padRight(lines[i], w-1))
		}
	}
	if m.cursor < m.listTop {
		m.listTop = m.cursor
	}
	if m.cursor >= m.listTop+h {
		m.listTop = m.cursor - h + 1
	}
	m.listTop = max(0, min(m.listTop, max(0, len(lines)-h)))
	end := min(len(lines), m.listTop+h)
	out := lines[m.listTop:end]
	for len(out) < h {
		out = append(out, "")
	}
	for i := range out {
		out[i] = padRight(out[i], w)
	}
	return strings.Join(out, "\n")
}

func (m *Model) listRow(it item, w int) string {
	var s string
	switch it.kind {
	case kHeader:
		if strings.HasPrefix(it.label, "  ") {
			s = sDim.Render(it.label)
		} else {
			s = sHead.Render(it.label)
		}
	case kDecision:
		s = sErr.Render("◆ ") + sDim.Render(it.dec.ID+" ") + it.dec.Title
	case kDialog:
		s = sErr.Render("▲ ") + sDim.Render(it.agent.Pane+" dialog ") + it.agent.Title
	case kSegment:
		seg := it.seg
		s = " " + statusStyle(seg.Status).Render("●") + " " + seg.ID + " " + statusStyle(seg.Status).Render(seg.Status)
		if pr := seg.PRNumber(); pr > 0 {
			s += sDim.Render(fmt.Sprintf(" #%d", pr))
		}
		s += m.segBadges(it.key)
	case kAgent:
		a := it.agent
		st := a.Status
		if a.Attention != "" && a.Attention != a.Status {
			st = a.Attention
		}
		role := ""
		if a.Role == "lead" {
			role = sCode.Render(" lead")
		}
		s = " " + sDim.Render(a.Pane) + " " + statusStyle(st).Render(fmt.Sprintf("%-8s", st)) + role + " " + a.Title
	case kDigest:
		s = " digest" + sDim.Render(" "+ago(m.board.Mate.At))
	}
	return truncate(s, w)
}

// segBadges shows each reviewer's latest verdict and CI, once loaded: "C✗ X✓ CI✓".
func (m *Model) segBadges(key string) string {
	var b strings.Builder
	if sd := m.segData[key]; sd != nil {
		for _, v := range sd.verdicts {
			mark := sOK.Render("✓")
			if v.entry.Verdict != "pass" {
				mark = sErr.Render("✗")
			}
			stale := ""
			if sd.wt != nil && !strings.HasPrefix(sd.wt.Head, v.entry.SHA) && !strings.HasPrefix(v.entry.SHA, sd.wt.Head) {
				stale = sDim.Render("~")
			}
			b.WriteString(" " + sDim.Render(reviewerTag(v.entry.Who)) + mark + stale)
		}
	}
	if p := m.prs[key]; p != nil && p.pr != nil {
		ci := p.pr.CI
		switch {
		case len(ci.Failed) > 0:
			b.WriteString(" " + sErr.Render("CI✗"))
		case ci.Pending > 0:
			b.WriteString(" " + sWarn.Render("CI…"))
		case ci.Total > 0:
			b.WriteString(" " + sOK.Render("CI✓"))
		}
	}
	return b.String()
}

func reviewerTag(who string) string {
	switch who {
	case "claude":
		return "cl"
	case "codex":
		return "cx"
	}
	if len(who) > 2 {
		return who[:2]
	}
	return who
}

func (m *Model) renderDetailHeader() string {
	it := m.current()
	if it == nil || it.kind != kSegment {
		return ""
	}
	var tabs []string
	for i, n := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, n)
		if segTab(i) == m.tab {
			tabs = append(tabs, sTabOn.Render(label))
		} else {
			tabs = append(tabs, sTabOff.Render(label))
		}
	}
	return strings.Join(tabs, "  ")
}

// refreshDetail renders the selected item into the viewport, keeping the scroll position.
func (m *Model) refreshDetail() {
	w := m.vp.Width
	it := m.current()
	m.findingRows, m.findingRefs = nil, nil
	var content string
	switch {
	case it == nil:
		if m.board == nil {
			content = sDim.Render("loading…")
		} else {
			content = sDim.Render("no epics or tabs to show")
		}
	case it.kind == kDecision:
		content = m.decisionView(it.dec, w)
	case it.kind == kDialog || it.kind == kAgent:
		content = m.agentView(it.agent, w)
	case it.kind == kSegment:
		switch m.tab {
		case tabOverview:
			content = m.overviewView(it, w)
		case tabReviews:
			content = m.reviewsView(it, w)
		case tabDiff:
			content = m.diffView(it, w)
		case tabBrief:
			content = m.briefView(it, w)
		}
	case it.kind == kDigest:
		content = sHead.Render("FIRST MATE DIGEST") + sDim.Render("  "+ago(m.board.Mate.At)) + "\n\n" + markdown(m.board.Mate.Text, w)
	}
	y := m.vp.YOffset
	m.vp.SetContent(content)
	m.vp.SetYOffset(y)
}

func (m *Model) decisionView(d *adw.Decision, w int) string {
	var b strings.Builder
	b.WriteString(wrap(sTitle.Render(d.Title), w) + "\n")
	meta := []string{d.ID, "asked " + ago(d.Created)}
	if d.Epic != "" {
		meta = append(meta, "epic "+d.Epic)
	}
	if d.Pane != "" {
		meta = append(meta, "from "+d.Pane)
	}
	b.WriteString(sDim.Render(strings.Join(meta, " · ")) + "\n")
	if d.DefaultAt != nil && d.Recommend != "" {
		b.WriteString(sWarn.Render(fmt.Sprintf("goes with (%s) %s unless you answer", d.Recommend, until(*d.DefaultAt))) + "\n")
	}
	b.WriteString("\n" + sHead.Render("OPTIONS") + "\n")
	for i, o := range d.Options {
		mark := "  "
		if m.mode == modeAnswerPick && i == m.pickSel {
			mark = sTitle.Render("› ")
		}
		rec := ""
		if o.Key == d.Recommend {
			rec = sOK.Render(" ★ recommended")
		}
		line := mark + sBold.Render("("+o.Key+") ") + o.Label + rec
		if m.mode == modeAnswerPick && i == m.pickSel {
			line = sSel.Render(line)
		}
		b.WriteString(wrap(line, w) + "\n")
	}
	b.WriteString("\n" + markdown(d.Body, w))
	return b.String()
}

func decisionText(d *adw.Decision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n\n", d.ID, d.Title)
	for _, o := range d.Options {
		fmt.Fprintf(&b, "(%s) %s\n", o.Key, o.Label)
	}
	b.WriteString("\n" + d.Body)
	return b.String()
}

func (m *Model) agentView(a *adw.Agent, w int) string {
	var b strings.Builder
	b.WriteString(wrap(sTitle.Render(a.Title), w) + "\n")
	meta := []string{a.Pane, a.Kind, statusStyle(a.Status).Render(a.Status)}
	if a.Attention != "" && a.Attention != a.Status {
		meta = append(meta, statusStyle(a.Attention).Render(a.Attention))
	}
	switch a.Role {
	case "lead":
		meta = append(meta, sCode.Render("lead of "+a.Epic))
	case "crew":
		meta = append(meta, "crew of "+a.Epic)
	}
	b.WriteString(strings.Join(meta, sDim.Render(" · ")) + "\n")
	b.WriteString(sDim.Render(a.Cwd))
	if a.Context != nil {
		b.WriteString(sDim.Render(fmt.Sprintf(" · context %dk", *a.Context/1000)))
	}
	if a.LastActive != nil {
		b.WriteString(sDim.Render(" · active " + ago(*a.LastActive)))
	}
	b.WriteString("\n")
	if a.Watch != nil {
		b.WriteString(wrap(statusStyle(a.Watch.State).Render(a.Watch.State)+sDim.Render(" "+a.Watch.Kind+" "+ago(a.Watch.At)+": ")+a.Watch.Text, w) + "\n")
	}
	if a.Summary != "" {
		b.WriteString("\n" + wrap(a.Summary, w) + "\n")
	}
	b.WriteString("\n" + sHead.Render("SCREEN") + "\n")
	if s := m.screens[a.Pane]; s == nil {
		b.WriteString(sDim.Render("reading…"))
	} else if s.err != nil {
		b.WriteString(sErr.Render(s.err.Error()))
	} else {
		b.WriteString(clip(strings.TrimRight(s.text, "\n"), w))
	}
	return b.String()
}

func (m *Model) overviewView(it *item, w int) string {
	s := it.seg
	var b strings.Builder
	b.WriteString(sTitle.Render(s.ID) + "  " + statusStyle(s.Status).Render(s.Status) + "\n")
	meta := []string{"epic " + s.Epic}
	if s.Builder != "" {
		meta = append(meta, "builder "+s.Builder)
	}
	if len(s.Deps) > 0 {
		meta = append(meta, "deps "+strings.Join(s.Deps, ", "))
	}
	b.WriteString(sDim.Render(strings.Join(meta, " · ")) + "\n")
	if s.Note != "" {
		b.WriteString(wrap(s.Note, w) + "\n")
	}

	sd := m.segData[it.key]
	b.WriteString("\n" + sHead.Render("WORKTREE") + "\n")
	switch {
	case sd == nil:
		b.WriteString(sDim.Render("reading…") + "\n")
	case sd.wt == nil:
		msg := "none"
		if sd.wtErr != nil {
			msg = sd.wtErr.Error()
		}
		b.WriteString(sDim.Render(msg) + "\n")
	default:
		wt := sd.wt
		b.WriteString(sDim.Render(wt.Path) + "\n")
		line := sCode.Render(wt.Branch) + " @" + short(wt.Head) + fmt.Sprintf(" · %d commits ahead", wt.Ahead)
		if wt.Dirty > 0 {
			line += sWarn.Render(fmt.Sprintf(" · %d uncommitted", wt.Dirty))
		}
		b.WriteString(line + "\n")
	}

	if pr := s.PRNumber(); pr > 0 {
		b.WriteString("\n" + sHead.Render(fmt.Sprintf("PR #%d", pr)) + "\n")
		p := m.prs[it.key]
		switch {
		case p == nil:
			b.WriteString(sDim.Render("loading pr-state…") + "\n")
		case p.err != nil:
			b.WriteString(sErr.Render(truncate(p.err.Error(), w)) + "\n")
		default:
			b.WriteString(m.prLines(p.pr, sd, w))
		}
	}

	if sd != nil && len(sd.verdicts) > 0 {
		b.WriteString("\n" + sHead.Render("LATEST REVIEWS") + sDim.Render("  (2 for findings)") + "\n")
		for _, v := range sd.verdicts {
			b.WriteString(ledgerLine(v.entry, sd.wt, w) + "\n")
		}
	}

	if sd != nil && sd.wt != nil {
		if as := m.agentsIn(sd.wt.Path); len(as) > 0 {
			b.WriteString("\n" + sHead.Render("TABS IN THIS WORKTREE") + "\n")
			for _, a := range as {
				b.WriteString(truncate(sDim.Render(a.Pane)+" "+statusStyle(a.Status).Render(a.Status)+" "+a.Title, w) + "\n")
			}
		}
	}

	if p := s.BriefPath(); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			lines := strings.Split(string(raw), "\n")
			b.WriteString("\n" + sHead.Render("BRIEF") + sDim.Render("  (4 for all, e to edit)") + "\n")
			b.WriteString(markdown(strings.Join(lines[:min(len(lines), 20)], "\n"), w))
		}
	}
	return b.String()
}

func (m *Model) prLines(p *adw.PRState, sd *segDataMsg, w int) string {
	var b strings.Builder
	b.WriteString(wrap(p.Title, w) + "\n")
	ci := p.CI
	var ciS string
	switch {
	case len(ci.Failed) > 0:
		ciS = sErr.Render(fmt.Sprintf("CI ✗ %d failed", len(ci.Failed)))
	case ci.Pending > 0:
		ciS = sWarn.Render(fmt.Sprintf("CI … %d/%d", ci.OK, ci.Total))
	default:
		ciS = sOK.Render(fmt.Sprintf("CI ✔ %d/%d", ci.OK, ci.Total))
	}
	parts := []string{p.State, ciS}
	if p.Bot.Login != "" {
		bot := p.Bot.Login + " " + p.Bot.Status
		if p.Bot.Approved {
			bot = sOK.Render(p.Bot.Login + " approved")
		}
		parts = append(parts, bot)
	}
	parts = append(parts, "review "+orNone(p.ReviewDecision), "merge "+orNone(p.MergeState))
	if p.OpenThreads > 0 {
		parts = append(parts, sWarn.Render(fmt.Sprintf("%d open threads", p.OpenThreads)))
	}
	if p.Draft {
		parts = append(parts, "draft")
	}
	b.WriteString(wrap(strings.Join(parts, sDim.Render(" · ")), w) + "\n")
	head := "head " + short(p.Head)
	if sd != nil && sd.wt != nil && sd.wt.Head != "" && sd.wt.Head != p.Head {
		head += sWarn.Render(" · worktree is at " + short(sd.wt.Head) + " (not pushed)")
	}
	b.WriteString(head + "\n" + sDim.Render(p.URL) + "\n")
	return b.String()
}

func ledgerLine(e adw.LedgerEntry, wt *adw.Worktree, w int) string {
	s := fmt.Sprintf("%-6s ", e.Who) + statusStyle(e.Verdict).Render(fmt.Sprintf("%-4s", e.Verdict))
	if e.Sev != "" && e.Sev != "none" {
		s += " " + statusStyle(e.Sev).Render(e.Sev)
	}
	if e.Found != nil {
		s += fmt.Sprintf(" · found %d", *e.Found)
		if e.Unique != nil {
			s += fmt.Sprintf(" (%d unique)", *e.Unique)
		}
	}
	s += sDim.Render(" @" + short(e.SHA))
	if wt != nil && wt.Head != "" {
		if strings.HasPrefix(wt.Head, e.SHA) || strings.HasPrefix(e.SHA, wt.Head) {
			s += sOK.Render(" current")
		} else {
			s += sDim.Render(" stale")
		}
	}
	if e.Unverified {
		s += sWarn.Render(" unverified")
	}
	s += sDim.Render(" · " + ago(e.At))
	s = wrap(s, w)
	if e.Notes != "" {
		s += "\n" + sDim.Render(indentLines("       "+wrap(e.Notes, w-7), "       "))
	}
	return s
}

func (m *Model) reviewsView(it *item, w int) string {
	sd := m.segData[it.key]
	if sd == nil {
		return sDim.Render("reading…")
	}
	if sd.wt == nil {
		return sDim.Render("no worktree, so no local review history (the ledger is keyed by branch)")
	}
	var lines []string
	add := func(s string) { lines = append(lines, strings.Split(s, "\n")...) }
	if len(sd.ledger) == 0 {
		add(sDim.Render("no reviews recorded for branch " + sd.wt.Branch))
		return strings.Join(lines, "\n")
	}
	add(sHead.Render("FINDINGS") + sDim.Render("  latest verdict per reviewer"))
	n := 0
	for _, v := range sd.verdicts {
		add("")
		hdr := sBold.Render(v.entry.Who) + " " + statusStyle(v.entry.Verdict).Render(v.entry.Verdict)
		if v.v == nil {
			why := "no report file"
			if v.err != nil {
				why = "report unreadable: " + v.err.Error()
			}
			add(hdr + sDim.Render(" @"+short(v.entry.SHA)+" · "+why))
			if v.entry.Notes != "" {
				add(indentLines("     "+wrap(v.entry.Notes, w-5), "     "))
			}
			continue
		}
		var counts []string
		for _, k := range []string{"critical", "important", "suggestion", "nitpick"} {
			if c := v.v.Counts[k]; c > 0 {
				counts = append(counts, statusStyle(k).Render(fmt.Sprintf("%d %s", c, k)))
			}
		}
		if v.v.Blocking > 0 {
			counts = append(counts, sErr.Render(fmt.Sprintf("%d blocking", v.v.Blocking)))
		}
		add(hdr + " · " + strings.Join(counts, sDim.Render(" · ")) + sDim.Render(" @"+short(v.v.SHA)))
		for _, f := range v.v.Findings {
			n++
			sel := m.detailFocus && len(m.findingRows) == m.findingSel
			m.findingRows = append(m.findingRows, len(lines))
			m.findingRefs = append(m.findingRefs, findingRef{file: f.File, line: f.Line})
			mark := "  "
			if sel {
				mark = sTitle.Render("› ")
			}
			loc := f.File
			if f.Line > 0 {
				loc += fmt.Sprintf(":%d", f.Line)
			}
			head := mark + fmt.Sprintf("#%-2d ", n) + statusStyle(f.Severity).Render(f.Severity) + sDim.Render(" "+f.Confidence+" ") + sCode.Render(loc)
			if sel {
				head = sSel.Render(head)
			}
			add(truncate(head, w))
			add(indentLines("     "+wrap(f.Claim, w-5), "     "))
			if f.Fix != "" {
				add(sDim.Render(indentLines("     fix: "+wrap(f.Fix, w-10), "     ")))
			}
		}
	}
	add("")
	add(sHead.Render("HISTORY") + sDim.Render(fmt.Sprintf("  %d verdicts on %s", len(sd.ledger), sd.wt.Branch)))
	for _, e := range sd.ledger {
		add(ledgerLine(e, sd.wt, w))
	}
	if m.findingSel >= len(m.findingRows) {
		m.findingSel = max(0, len(m.findingRows)-1)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) diffView(it *item, w int) string {
	d := m.diffs[it.key]
	if d == nil {
		return sDim.Render("diffing…")
	}
	if d.err != nil {
		return sErr.Render(d.err.Error())
	}
	if strings.TrimSpace(d.diff) == "" {
		return sDim.Render("no changes against the base branch")
	}
	var b strings.Builder
	if d.stat != "" {
		b.WriteString(clip(strings.TrimRight(d.stat, "\n"), w) + "\n\n")
	}
	for _, l := range strings.Split(strings.TrimRight(d.diff, "\n"), "\n") {
		l = strings.ReplaceAll(l, "\t", "    ")
		var st lipgloss.Style
		switch {
		case strings.HasPrefix(l, "diff --git"):
			b.WriteString("\n")
			st = sHead
		case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"), strings.HasPrefix(l, "index "):
			st = sDim
		case strings.HasPrefix(l, "@@"):
			st = sCode
		case strings.HasPrefix(l, "+"):
			st = sOK
		case strings.HasPrefix(l, "-"):
			st = sErr
		default:
			b.WriteString(ansi.Truncate(l, w, "…") + "\n")
			continue
		}
		b.WriteString(st.Render(ansi.Truncate(l, w, "…")) + "\n")
	}
	return b.String()
}

func (m *Model) briefView(it *item, w int) string {
	p := it.seg.BriefPath()
	if p == "" {
		return sDim.Render("no brief recorded")
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return sErr.Render(err.Error())
	}
	return sDim.Render(p) + "\n\n" + markdown(string(raw), w)
}

var (
	reBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reCode = regexp.MustCompile("`([^`]+)`")
	reHead = regexp.MustCompile(`^#{1,6}\s+`)
)

// markdown is just enough styling to make briefs and decision bodies scannable.
func markdown(s string, w int) string {
	var out []string
	inFence := false
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
			out = append(out, sDim.Render(l))
			continue
		}
		if inFence {
			out = append(out, sCode.Render(ansi.Truncate(l, w, "…")))
			continue
		}
		if reHead.MatchString(l) {
			out = append(out, sHead.Render(reHead.ReplaceAllString(l, "")))
			continue
		}
		l = reBold.ReplaceAllStringFunc(l, func(x string) string { return sBold.Render(x[2 : len(x)-2]) })
		l = reCode.ReplaceAllStringFunc(l, func(x string) string { return sCode.Render(x[1 : len(x)-1]) })
		indent := len(l) - len(strings.TrimLeft(l, " "))
		if t := strings.TrimLeft(l, " "); strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			indent += 2
		}
		indent = min(indent, 12)
		out = append(out, indentLines(wrap(l, w-indent), strings.Repeat(" ", indent)))
	}
	return strings.Join(out, "\n")
}

func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Wrap(s, w, "")
}

// clip truncates each line of already-styled text to the width.
func clip(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	return strings.Join(lines, "\n")
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

func padRight(s string, w int) string {
	if d := w - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func ago(ts float64) string {
	d := time.Since(time.Unix(int64(ts), 0))
	return fmtDur(d) + " ago"
}

func until(ts float64) string {
	return "in " + fmtDur(time.Until(time.Unix(int64(ts), 0)))
}

func fmtDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

const helpText = `
  adw-tui — review-first view over the ADW fleet

  LIST                            DETAIL (tab to focus)
  j/k  move                       j/k  scroll (reviews tab: move between findings)
  n    next thing that needs you  J/K  half page
  g/G  top/bottom                 g/G  top/bottom
  tab  focus detail               esc  back to the list

  SEGMENTS                        DECISIONS
  1-4 or [ ]  overview/reviews/   enter/a  answer: pick an option, add a note
              diff/brief          f        jump to the asking tab
  d    full diff in pager         r        message the asking tab
  e    edit brief, or open the    v        page the full body
       selected finding           z / Z    snooze 1h / until tomorrow 8am
  o    open the PR in a browser
  r    note to the epic lead      TABS AND DIALOGS
       (cites the finding)        enter/f  jump to the tab in herdr
  c    note to the builder tab    r        send it a message
  enter jump to builder or lead   v        page its screen

  R  refresh everything   ? help   q quit

  Answers go through adw-ask, so delivery and locking match the web board.
  Data: herdr socket, adw-board --json, $ADW_HOME, review-ledger.jsonl, pr-state.

  press any key
`
