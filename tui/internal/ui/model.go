// Package ui is the adw-tui Bubble Tea program: a review-first view over the fleet.
package ui

import (
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sethgw/agent-fleet/tui/internal/adw"
)

var debug = os.Getenv("ADW_TUI_LOG") != ""

type kind int

const (
	kHeader kind = iota
	kDecision
	kDialog
	kSegment
	kAgent
	kDigest
)

type item struct {
	kind  kind
	key   string
	label string
	dec   *adw.Decision
	agent *adw.Agent
	seg   *adw.Segment
}

func (it item) selectable() bool { return it.kind != kHeader }

type segTab int

const (
	tabOverview segTab = iota
	tabReviews
	tabDiff
	tabBrief
)

var tabNames = []string{"overview", "reviews", "diff", "brief"}

type mode int

const (
	modeNormal mode = iota
	modeReply
	modeAnswerPick
	modeAnswerNote
	modeHelp
)

type Model struct {
	w, h int

	board    *adw.Board
	graphs   map[string]*adw.Graph
	manifest map[string]adw.RepoConf
	loadErr  error
	loading  bool
	reload   bool
	loadedAt time.Time

	items   []item
	cursor  int
	listTop int

	detailFocus bool
	tab         segTab
	vp          viewport.Model
	findingSel  int
	findingRows []int
	findingRefs []findingRef

	herdrPending bool
	inflight     map[string]bool

	segData map[string]*segDataMsg
	prs     map[string]*prMsg
	diffs   map[string]*diffMsg
	screens map[string]*screenMsg

	mode      mode
	input     textinput.Model
	replyTo   string
	replyHint string
	pickSel   int

	flash    string
	flashErr bool
	flashAt  time.Time
}

type findingRef struct {
	file string
	line int
}

func New() *Model {
	ti := textinput.New()
	ti.CharLimit = 4000
	return &Model{
		manifest: adw.LoadManifest(),
		inflight: map[string]bool{},
		segData:  map[string]*segDataMsg{},
		prs:      map[string]*prMsg{},
		diffs:    map[string]*diffMsg{},
		screens:  map[string]*screenMsg{},
		vp:       viewport.New(80, 20),
		input:    ti,
		loading:  true,
	}
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(loadBoard, tick(3*time.Second, tickMsg{}), tick(2*time.Second, screenTickMsg{}))
}

func (m *Model) current() *item {
	if m.cursor >= 0 && m.cursor < len(m.items) {
		return &m.items[m.cursor]
	}
	return nil
}

func (m *Model) requestBoard() tea.Cmd {
	if m.loading {
		m.reload = true
		return nil
	}
	m.loading = true
	return loadBoard
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if debug {
		log.Printf("%T %v", msg, msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		return m, nil

	case boardMsg:
		m.loading = false
		m.loadErr = msg.err
		var cmds []tea.Cmd
		if msg.err == nil {
			prevKey := ""
			if it := m.current(); it != nil {
				prevKey = it.key
			}
			m.board, m.graphs, m.loadedAt = msg.board, msg.graphs, time.Now()
			m.buildItems()
			m.restoreCursor(prevKey)
			cmds = append(cmds, m.onSelect(false))
		}
		if m.reload {
			m.reload = false
			cmds = append(cmds, m.requestBoard())
		}
		m.refreshDetail()
		return m, tea.Batch(cmds...)

	case tickMsg:
		return m, tea.Batch(m.requestBoard(), tick(3*time.Second, tickMsg{}))

	case HerdrChangedMsg:
		// Events arrive in bursts; one reload shortly after the first is enough.
		if m.herdrPending {
			return m, nil
		}
		m.herdrPending = true
		return m, tick(400*time.Millisecond, herdrFlushMsg{})

	case herdrFlushMsg:
		m.herdrPending = false
		return m, m.requestBoard()

	case screenTickMsg:
		var cmd tea.Cmd
		if it := m.current(); it != nil && (it.kind == kAgent || it.kind == kDialog) {
			cmd = loadScreen(it.agent.Pane)
		}
		return m, tea.Batch(cmd, tick(2*time.Second, screenTickMsg{}))

	case segDataMsg:
		m.segData[msg.key] = &msg
		delete(m.inflight, "seg:"+msg.key)
		m.refreshDetail()
		if it := m.current(); it != nil && it.key == msg.key && m.tab == tabDiff {
			return m, m.diffCmd(false)
		}
		return m, nil

	case prMsg:
		m.prs[msg.key] = &msg
		delete(m.inflight, "pr:"+msg.key)
		m.refreshDetail()
		return m, nil

	case diffMsg:
		m.diffs[msg.key] = &msg
		delete(m.inflight, "diff:"+msg.key)
		m.refreshDetail()
		return m, nil

	case screenMsg:
		follow := m.vp.AtBottom()
		m.screens[msg.pane] = &msg
		m.refreshDetail()
		if follow {
			m.vp.GotoBottom()
		}
		return m, nil

	case flashMsg:
		m.flash, m.flashErr, m.flashAt = msg.text, msg.err, time.Now()
		return m, m.requestBoard()

	case execDoneMsg:
		if msg.err != nil {
			m.flash, m.flashErr, m.flashAt = msg.err.Error(), true, time.Now()
		}
		return m, nil

	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

type execDoneMsg struct{ err error }

type herdrFlushMsg struct{}

// once runs cmd unless the same load is already in flight, or already cached and not forced.
func (m *Model) once(id string, cached, force bool, cmd tea.Cmd) tea.Cmd {
	if m.inflight[id] || (cached && !force) {
		return nil
	}
	m.inflight[id] = true
	return cmd
}

func execDone(err error) tea.Msg { return execDoneMsg{err} }

func (m *Model) layout() {
	lw := m.listWidth()
	m.vp.Width = max(10, m.w-lw-3)
	m.vp.Height = max(3, m.h-4)
	m.input.Width = max(10, m.w-20)
	m.refreshDetail()
}

func (m *Model) listWidth() int {
	return min(56, max(30, m.w*2/5))
}

// buildItems lays the board out as: what needs you, segments to review per epic, then every tab.
func (m *Model) buildItems() {
	b := m.board
	var items []item
	snoozed := func(key string) bool { _, ok := b.Snoozes[key]; return ok }

	var needs []item
	for i := range b.Decisions {
		d := &b.Decisions[i]
		if d.Status == "open" && !snoozed("d:"+d.ID) {
			needs = append(needs, item{kind: kDecision, key: "d:" + d.ID, dec: d})
		}
	}
	for i := range b.Agents {
		a := &b.Agents[i]
		if a.Status == "blocked" && !snoozed(dialogKey(a)) {
			needs = append(needs, item{kind: kDialog, key: "x:" + a.Pane, agent: a})
		}
	}
	if len(needs) > 0 {
		items = append(items, item{kind: kHeader, label: "NEEDS YOU"})
		items = append(items, needs...)
	}

	epics := make([]string, 0, len(m.graphs))
	for _, e := range b.Epics {
		if _, ok := m.graphs[e.Epic]; ok {
			epics = append(epics, e.Epic)
		}
	}
	for _, name := range epics {
		g := m.graphs[name]
		var segs []item
		for i := range g.Segments {
			s := &g.Segments[i]
			if reviewable(s) {
				segs = append(segs, item{kind: kSegment, key: "s:" + name + "/" + s.ID, seg: s})
			}
		}
		sort.SliceStable(segs, func(i, j int) bool { return statusRank(segs[i].seg.Status) < statusRank(segs[j].seg.Status) })
		items = append(items, item{kind: kHeader, label: "REVIEW · " + name, key: "e:" + name})
		if len(segs) == 0 {
			items = append(items, item{kind: kHeader, label: "  nothing in flight"})
		}
		items = append(items, segs...)
	}

	if len(b.Agents) > 0 {
		items = append(items, item{kind: kHeader, label: "TABS"})
		for i := range b.Agents {
			a := &b.Agents[i]
			items = append(items, item{kind: kAgent, key: "a:" + a.Pane, agent: a})
		}
	}
	if b.Mate != nil && strings.TrimSpace(b.Mate.Text) != "" {
		items = append(items, item{kind: kHeader, label: "FIRST MATE"}, item{kind: kDigest, key: "m", label: "digest"})
	}
	m.items = items
}

func dialogKey(a *adw.Agent) string {
	if a.Session != "" {
		return "p:" + a.Session
	}
	return "p:" + a.Pane
}

func reviewable(s *adw.Segment) bool {
	switch s.Status {
	case "building", "pr-open", "blocked", "pr-ready", "ready":
		return true
	}
	return false
}

func statusRank(s string) int {
	return map[string]int{"pr-ready": 0, "blocked": 1, "pr-open": 2, "building": 3, "ready": 4}[s]
}

func (m *Model) restoreCursor(key string) {
	for i, it := range m.items {
		if it.key == key && it.selectable() {
			m.cursor = i
			return
		}
	}
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	m.moveCursor(0)
}

func (m *Model) moveCursor(delta int) {
	if len(m.items) == 0 {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	n := delta
	if n < 0 {
		n = -n
	}
	i := max(0, min(m.cursor, len(m.items)-1))
	for ; n > 0; n-- {
		j := i + step
		for j >= 0 && j < len(m.items) && !m.items[j].selectable() {
			j += step
		}
		if j < 0 || j >= len(m.items) {
			break
		}
		i = j
	}
	for !m.items[i].selectable() {
		if i+1 < len(m.items) {
			i++
		} else if i > 0 {
			i--
		} else {
			break
		}
	}
	m.cursor = i
}

// onSelect kicks off the loads the selected item's detail needs; force re-fetches cached ones.
func (m *Model) onSelect(force bool) tea.Cmd {
	it := m.current()
	if it == nil {
		return nil
	}
	switch it.kind {
	case kAgent, kDialog:
		if _, ok := m.screens[it.agent.Pane]; !ok || force {
			return loadScreen(it.agent.Pane)
		}
	case kSegment:
		var cmds []tea.Cmd
		s := it.seg
		repo := m.manifest[s.Repo]
		_, ok := m.segData[it.key]
		cmds = append(cmds, m.once("seg:"+it.key, ok, force, loadSegment(it.key, *s, repo.BaseBranch)))
		if pr := s.PRNumber(); pr > 0 && repo.Path != "" {
			_, ok := m.prs[it.key]
			cmds = append(cmds, m.once("pr:"+it.key, ok, force, loadPR(it.key, repo.Path, pr)))
		}
		if m.tab == tabDiff {
			cmds = append(cmds, m.diffCmd(force))
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (m *Model) diffCmd(force bool) tea.Cmd {
	it := m.current()
	if it == nil || it.kind != kSegment {
		return nil
	}
	sd := m.segData[it.key]
	if sd == nil {
		return nil // loadSegment's result re-triggers this
	}
	_, ok := m.diffs[it.key]
	repo := m.manifest[it.seg.Repo]
	return m.once("diff:"+it.key, ok, force, loadDiff(it.key, sd.wt, repo.Path, it.seg.PRNumber()))
}

// agentsIn returns tabs whose working directory is inside dir.
func (m *Model) agentsIn(dir string) []*adw.Agent {
	if dir == "" || m.board == nil {
		return nil
	}
	dir = filepath.Clean(dir)
	var out []*adw.Agent
	for i := range m.board.Agents {
		a := &m.board.Agents[i]
		cwd := filepath.Clean(adw.ExpandHome(a.Cwd))
		if a.Machine == "" && (cwd == dir || strings.HasPrefix(cwd, dir+string(os.PathSeparator))) {
			out = append(out, a)
		}
	}
	return out
}

func (m *Model) epicLead(epic string) string {
	if m.board == nil {
		return ""
	}
	for _, e := range m.board.Epics {
		if e.Epic == epic {
			return e.LeadPane
		}
	}
	return ""
}
