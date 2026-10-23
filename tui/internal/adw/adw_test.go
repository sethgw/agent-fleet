package adw

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLedgerFiltersBranchAndSortsNewestFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ADW_HOME", home)
	writeFile(t, filepath.Join(home, "review-ledger.jsonl"), `{"at": 1, "repo": "fluid", "branch": "a", "who": "claude", "verdict": "pass"}
not json
{"at": 3, "repo": "fluid", "branch": "a", "who": "codex", "verdict": "fail", "report": null}
{"at": 2, "repo": "other", "branch": "a", "who": "claude", "verdict": "fail"}
{"at": 4, "repo": "fluid", "branch": "b", "who": "claude", "verdict": "fail"}
`)
	got := LoadLedger("fluid", "a")
	if len(got) != 2 || got[0].At != 3 || got[1].At != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestLatestByReviewerKeepsAFailWithoutAReport(t *testing.T) {
	entries := []LedgerEntry{
		{At: 5, Who: "codex", Verdict: "fail"},
		{At: 4, Who: "claude", Verdict: "pass", Report: "/r/claude.json"},
		{At: 3, Who: "codex", Verdict: "pass", Report: "/r/codex.json"},
	}
	got := LatestByReviewer(entries)
	if len(got) != 2 || got[0].Who != "claude" || got[1].Who != "codex" || got[1].Verdict != "fail" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadGraphStampsEpicAndRepo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ADW_HOME", home)
	writeFile(t, filepath.Join(home, "epics/e1/graph.json"),
		`{"repo": "fluid", "segments": [{"id": "s1", "status": "building", "pr": null, "adopt_pr": 42, "brief": "briefs/s1.md"}]}`)
	g, err := LoadGraph("e1")
	if err != nil {
		t.Fatal(err)
	}
	s := g.Segments[0]
	if s.Epic != "e1" || s.Repo != "fluid" || s.PRNumber() != 42 {
		t.Fatalf("got %+v", s)
	}
	if want := filepath.Join(home, "epics/e1/briefs/s1.md"); s.BriefPath() != want {
		t.Fatalf("brief path %q, want %q", s.BriefPath(), want)
	}
}

func TestSetSnoozeDropsExpiredAndRejectsBadKeys(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ADW_HOME", home)
	writeFile(t, filepath.Join(home, "board-snoozes.json"), `{"d:d-000001": 1}`)
	if err := SetSnooze("d:d-abc123", Tomorrow8()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(home, "board-snoozes.json"))
	if s := string(b); !strings.Contains(s, "d:d-abc123") || strings.Contains(s, "d:d-000001") {
		t.Fatalf("snoozes: %s", s)
	}
	if err := SetSnooze("../evil", Tomorrow8()); err == nil {
		t.Fatal("bad key accepted")
	}
}
