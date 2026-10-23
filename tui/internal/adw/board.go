// Package adw reads the ADW state the Python tools write under $ADW_HOME, and drives the existing CLIs
// (adw-board, adw-ask, pr-state) for anything that has rules attached.
package adw

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Home() string {
	if h := os.Getenv("ADW_HOME"); h != "" {
		return h
	}
	return filepath.Join(os.Getenv("HOME"), ".claude/adw")
}

func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(os.Getenv("HOME"), p[1:])
	}
	return p
}

type Watch struct {
	Kind  string   `json:"kind"`
	Text  string   `json:"text"`
	At    float64  `json:"at"`
	Until *float64 `json:"until"`
	State string   `json:"state"`
}

type Agent struct {
	Pane       string   `json:"pane"`
	Kind       string   `json:"kind"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Cwd        string   `json:"cwd"`
	Machine    string   `json:"machine"`
	Epic       string   `json:"epic"`
	Role       string   `json:"role"`
	Session    string   `json:"session"`
	Watch      *Watch   `json:"watch"`
	Attention  string   `json:"attention"`
	Summary    string   `json:"summary"`
	LastActive *float64 `json:"last_active"`
	LeadPane   string   `json:"lead_pane"`
	Context    *int     `json:"context"`
}

type Option struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Answer struct {
	Choice string  `json:"choice"`
	Note   string  `json:"note"`
	At     float64 `json:"at"`
	By     string  `json:"by"`
}

type Decision struct {
	ID        string   `json:"id"`
	Created   float64  `json:"created"`
	Status    string   `json:"status"`
	Title     string   `json:"title"`
	Epic      string   `json:"epic"`
	Body      string   `json:"body"`
	Options   []Option `json:"options"`
	Recommend string   `json:"recommend"`
	Pane      string   `json:"pane"`
	DefaultAt *float64 `json:"default_at"`
	Answer    *Answer  `json:"answer"`
	Delivered any      `json:"delivered"`
}

type EpicSummary struct {
	Epic        string         `json:"epic"`
	Repo        string         `json:"repo"`
	LeadPane    string         `json:"lead_pane"`
	Paused      map[string]any `json:"paused"`
	MaxParallel int            `json:"max_parallel"`
	Mtime       float64        `json:"mtime"`
	Counts      map[string]int `json:"counts"`
	Pending     *struct {
		Items []string `json:"items"`
	} `json:"pending"`
}

type Board struct {
	Generated int64              `json:"generated"`
	Epics     []EpicSummary      `json:"epics"`
	Agents    []Agent            `json:"agents"`
	Decisions []Decision         `json:"decisions"`
	Snoozes   map[string]float64 `json:"snoozes"`
	Mate      *struct {
		Text string  `json:"text"`
		At   float64 `json:"at"`
	} `json:"mate"`
	MatePane string `json:"mate_pane"`
}

// LoadBoard runs `adw-board --json`, which owns tab classification, epic linking and watch state.
func LoadBoard() (*Board, error) {
	out, err := exec.Command("adw-board", "--json").Output()
	if err != nil {
		return nil, cmdErr("adw-board", err)
	}
	var b Board
	if err := json.Unmarshal(out, &b); err != nil {
		return nil, err
	}
	return &b, nil
}

func cmdErr(name string, err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return &CmdError{Name: name, Msg: strings.TrimSpace(string(ee.Stderr))}
	}
	return err
}

type CmdError struct{ Name, Msg string }

func (e *CmdError) Error() string { return e.Name + ": " + e.Msg }

// AnswerDecision goes through adw-ask so locking and delivery to the asking pane stay in one place.
func AnswerDecision(id, choice, note string) (string, error) {
	args := []string{"answer", id, choice}
	if note != "" {
		args = append(args, "--note", note)
	}
	out, err := exec.Command("adw-ask", args...).CombinedOutput()
	if err != nil {
		return "", &CmdError{Name: "adw-ask", Msg: strings.TrimSpace(string(out))}
	}
	return strings.TrimSpace(string(out)), nil
}
