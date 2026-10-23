package ui

import (
	"testing"

	"github.com/sethgw/agent-fleet/tui/internal/adw"
)

func testModel() *Model {
	m := New()
	m.board = &adw.Board{
		Epics: []adw.EpicSummary{{Epic: "e1"}},
		Decisions: []adw.Decision{
			{ID: "d-000001", Status: "open", Title: "open"},
			{ID: "d-000002", Status: "open", Title: "snoozed"},
			{ID: "d-000003", Status: "answered", Title: "done"},
		},
		Agents: []adw.Agent{
			{Pane: "w1:p1", Status: "blocked", Session: "s1"},
			{Pane: "w1:p2", Status: "working"},
		},
		Snoozes: map[string]float64{"d:d-000002": 9e12},
	}
	m.graphs = map[string]*adw.Graph{"e1": {Epic: "e1", Segments: []adw.Segment{
		{ID: "merged", Status: "merged"},
		{ID: "build", Status: "building"},
		{ID: "ready-pr", Status: "pr-ready"},
	}}}
	m.buildItems()
	return m
}

func keys(m *Model) []string {
	var out []string
	for _, it := range m.items {
		if it.selectable() {
			out = append(out, it.key)
		}
	}
	return out
}

func TestBuildItemsOrdersNeedsYouThenReviewThenTabs(t *testing.T) {
	got := keys(testModel())
	want := []string{"d:d-000001", "x:w1:p1", "s:e1/ready-pr", "s:e1/build", "a:w1:p1", "a:w1:p2"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCursorSkipsHeaders(t *testing.T) {
	m := testModel()
	m.cursor = 0
	m.moveCursor(0)
	if m.current().key != "d:d-000001" {
		t.Fatalf("start on %q", m.current().key)
	}
	m.moveCursor(2) // across the REVIEW header
	if m.current().key != "s:e1/ready-pr" {
		t.Fatalf("landed on %q", m.current().key)
	}
	m.moveCursor(-10)
	if m.current().key != "d:d-000001" {
		t.Fatalf("top is %q", m.current().key)
	}
}

func TestRestoreCursorFollowsKeyAcrossRebuilds(t *testing.T) {
	m := testModel()
	m.restoreCursor("s:e1/build")
	m.board.Decisions[1].Status = "answered"
	m.buildItems()
	m.restoreCursor("s:e1/build")
	if m.current().key != "s:e1/build" {
		t.Fatalf("cursor on %q", m.current().key)
	}
}

func TestEditorCmdLineArgs(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "nvim")
	if got := editorCmd("/x/a.rb", 12).Args; len(got) != 3 || got[1] != "+12" || got[2] != "/x/a.rb" {
		t.Fatalf("nvim args %v", got)
	}
	t.Setenv("EDITOR", "code --wait")
	if got := editorCmd("/x/a.rb", 12).Args; got[len(got)-1] != "/x/a.rb:12" || got[len(got)-2] != "-g" {
		t.Fatalf("code args %v", got)
	}
}
