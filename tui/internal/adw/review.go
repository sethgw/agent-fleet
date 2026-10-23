package adw

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Segment struct {
	Epic     string   `json:"-"`
	Repo     string   `json:"-"`
	ID       string   `json:"id"`
	Brief    string   `json:"brief"`
	Deps     []string `json:"deps"`
	Status   string   `json:"status"`
	PR       *int     `json:"pr"`
	Builder  string   `json:"builder"`
	AdoptPR  *int     `json:"adopt_pr"`
	Phase    any      `json:"phase"`
	Note     string   `json:"note"`
	Worktree string   `json:"worktree"`
}

// PRNumber prefers the segment's own PR and falls back to the one it adopts.
func (s Segment) PRNumber() int {
	if s.PR != nil {
		return *s.PR
	}
	if s.AdoptPR != nil {
		return *s.AdoptPR
	}
	return 0
}

func (s Segment) BriefPath() string {
	if s.Brief == "" {
		return ""
	}
	return filepath.Join(Home(), "epics", s.Epic, s.Brief)
}

type Graph struct {
	Epic        string    `json:"epic"`
	Goal        string    `json:"goal"`
	Repo        string    `json:"repo"`
	MaxParallel int       `json:"max_parallel"`
	Segments    []Segment `json:"segments"`
}

func LoadGraph(epic string) (*Graph, error) {
	b, err := os.ReadFile(filepath.Join(Home(), "epics", epic, "graph.json"))
	if err != nil {
		return nil, err
	}
	var g Graph
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	if g.Epic == "" {
		g.Epic = epic
	}
	for i := range g.Segments {
		g.Segments[i].Epic, g.Segments[i].Repo = g.Epic, g.Repo
	}
	return &g, nil
}

type RepoConf struct {
	Path       string `json:"path"`
	BaseBranch string `json:"base_branch"`
}

func LoadManifest() map[string]RepoConf {
	var m struct {
		Repos map[string]RepoConf `json:"repos"`
	}
	b, err := os.ReadFile(filepath.Join(Home(), "manifest.json"))
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	for k, r := range m.Repos {
		r.Path = ExpandHome(r.Path)
		if r.BaseBranch == "" {
			r.BaseBranch = "main"
		}
		m.Repos[k] = r
	}
	return m.Repos
}

// LedgerEntry is one line of $ADW_HOME/review-ledger.jsonl, written by review-receipt.
type LedgerEntry struct {
	At      float64 `json:"at"`
	Repo    string  `json:"repo"`
	Branch  string  `json:"branch"`
	SHA     string  `json:"sha"`
	Who     string  `json:"who"`
	Verdict string  `json:"verdict"`
	Found   *int    `json:"found"`
	Unique  *int    `json:"unique"`
	Sev     string  `json:"sev"`
	Notes   string  `json:"notes"`
	Report  string  `json:"report"`
	// Unverified marks a verdict review-receipt recorded without a real review's report behind it.
	Unverified bool `json:"unverified"`
}

func LoadLedger(repo, branch string) []LedgerEntry {
	f, err := os.Open(filepath.Join(Home(), "review-ledger.jsonl"))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []LedgerEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var e LedgerEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if e.Branch == branch && (repo == "" || e.Repo == repo) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	return out
}

// LatestByReviewer keeps each reviewer's newest entry (entries are newest first), whether or not it left a
// report file: a fail recorded without one must still show. Sorted by reviewer name.
func LatestByReviewer(entries []LedgerEntry) []LedgerEntry {
	seen := map[string]bool{}
	var out []LedgerEntry
	for _, e := range entries {
		if !seen[e.Who] {
			seen[e.Who] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Who < out[j].Who })
	return out
}

type Finding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`
	Confidence string `json:"confidence"`
	Claim      string `json:"claim"`
	Fix        string `json:"fix"`
	Evidence   string `json:"evidence"`
}

type Verdict struct {
	Who      string         `json:"who"`
	SHA      string         `json:"sha"`
	Verdict  string         `json:"verdict"`
	Counts   map[string]int `json:"counts"`
	Blocking int            `json:"blocking"`
	Findings []Finding      `json:"findings"`
}

func LoadVerdict(path string) (*Verdict, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v Verdict
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func git(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return "", cmdErr("git "+args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

type Worktree struct {
	Path      string
	Branch    string
	Head      string
	MergeBase string
	Dirty     int
	Ahead     int
}

// InspectWorktree reads branch, head and the merge base against origin/<base>.
func InspectWorktree(path, base string) (*Worktree, error) {
	if path == "" {
		return nil, fmt.Errorf("no worktree recorded")
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("worktree %s is gone", path)
	}
	w := &Worktree{Path: path}
	var err error
	if w.Branch, err = git(path, "rev-parse", "--abbrev-ref", "HEAD"); err != nil {
		return nil, err
	}
	w.Head, _ = git(path, "rev-parse", "HEAD")
	for _, ref := range []string{"origin/" + base, base} {
		if mb, err := git(path, "merge-base", ref, "HEAD"); err == nil {
			w.MergeBase = mb
			break
		}
	}
	if st, err := git(path, "status", "--porcelain"); err == nil && st != "" {
		w.Dirty = len(strings.Split(st, "\n"))
	}
	if w.MergeBase != "" {
		if n, err := git(path, "rev-list", "--count", w.MergeBase+"..HEAD"); err == nil {
			fmt.Sscan(n, &w.Ahead)
		}
	}
	return w, nil
}

// Diff is the worktree (committed and uncommitted) against its merge base.
func (w *Worktree) Diff(stat bool) (string, error) {
	if w.MergeBase == "" {
		return "", fmt.Errorf("no merge base with the base branch")
	}
	args := []string{"-C", w.Path, "diff", "--no-color", "--no-ext-diff"}
	if stat {
		args = append(args, "--stat=200")
	}
	out, err := exec.Command("git", append(args, w.MergeBase)...).Output()
	if err != nil {
		return "", cmdErr("git diff", err)
	}
	return string(out), nil
}

// PRDiff falls back to GitHub when there is no local worktree.
func PRDiff(repoDir string, pr int) (string, error) {
	cmd := exec.Command("gh", "pr", "diff", fmt.Sprint(pr), "--color=never")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", cmdErr("gh pr diff", err)
	}
	return string(out), nil
}

type PRState struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Head   string `json:"head"`
	CI     struct {
		Total   int   `json:"total"`
		OK      int   `json:"ok"`
		Pending int   `json:"pending"`
		Failed  []any `json:"failed"`
		Nonpass []any `json:"nonpass"`
	} `json:"ci"`
	Bot struct {
		Login       string `json:"login"`
		Status      string `json:"status"`
		Rounds      int    `json:"rounds"`
		Approved    bool   `json:"approved"`
		HasVerdict  bool   `json:"has_verdict"`
		TopFindings []any  `json:"top_findings"`
	} `json:"bot"`
	ReviewDecision string `json:"review_decision"`
	MergeState     string `json:"merge_state"`
	OpenThreads    int    `json:"open_threads"`
}

// LoadPRState runs pr-state from the repo checkout so it resolves the right GitHub repo.
func LoadPRState(repoDir string, pr int) (*PRState, error) {
	cmd := exec.Command("pr-state", fmt.Sprint(pr), "--json")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, cmdErr("pr-state", err)
	}
	var s PRState
	if err := json.Unmarshal(bytes.TrimSpace(out), &s); err != nil {
		return nil, err
	}
	return &s, nil
}
