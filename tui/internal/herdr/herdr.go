// Package herdr talks to herdr's newline-delimited JSON socket API (protocol 19; `herdr api schema --json`).
package herdr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var seq atomic.Uint64

// SocketFor returns the socket path for a machine name; "" is this machine.
// Remote machines follow board/asks.py: $ADW_HERDR_REMOTE_DIR/<name>.sock.
func SocketFor(machine string) string {
	if machine != "" {
		dir := os.Getenv("ADW_HERDR_REMOTE_DIR")
		if dir == "" {
			dir = filepath.Join(os.Getenv("HOME"), ".config/herdr/remote")
		}
		return filepath.Join(dir, machine+".sock")
	}
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".config/herdr/herdr.sock")
}

// SplitPane turns a board pane id ("mac/w1:p2" or "w1:p2") into machine and herdr pane id.
func SplitPane(pane string) (machine, id string) {
	if m, rest, ok := strings.Cut(pane, "/"); ok {
		return m, rest
	}
	return "", pane
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *apiError       `json:"error"`
}

// Call sends one request on a fresh connection and decodes result into out (if non-nil).
func Call(machine, method string, params any, out any) error {
	if params == nil {
		params = struct{}{}
	}
	c, err := net.DialTimeout("unix", SocketFor(machine), 2*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	id := fmt.Sprintf("adw-tui:%d", seq.Add(1))
	req, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if _, err := c.Write(append(req, '\n')); err != nil {
		return err
	}
	r := bufio.NewReaderSize(c, 1<<20)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return err
	}
	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("herdr %s: %s", method, resp.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(resp.Result, out)
	}
	return nil
}

type Session struct {
	Value string `json:"value"`
}

type Pane struct {
	PaneID      string   `json:"pane_id"`
	TabID       string   `json:"tab_id"`
	WorkspaceID string   `json:"workspace_id"`
	Agent       string   `json:"agent"`
	AgentStatus string   `json:"agent_status"`
	Cwd         string   `json:"cwd"`
	Title       string   `json:"terminal_title_stripped"`
	Session     *Session `json:"agent_session"`
	Focused     bool     `json:"focused"`
}

type Snapshot struct {
	FocusedPaneID string `json:"focused_pane_id"`
	Panes         []Pane `json:"panes"`
	Agents        []Pane `json:"agents"`
}

func GetSnapshot(machine string) (*Snapshot, error) {
	var r struct {
		Snapshot Snapshot `json:"snapshot"`
	}
	if err := Call(machine, "session.snapshot", nil, &r); err != nil {
		return nil, err
	}
	return &r.Snapshot, nil
}

// ReadPane returns a pane's recent output with ANSI styling kept.
func ReadPane(pane string, lines int) (string, error) {
	m, id := SplitPane(pane)
	var r struct {
		Read struct {
			Text string `json:"text"`
		} `json:"read"`
	}
	p := map[string]any{"pane_id": id, "source": "recent", "lines": lines, "format": "ansi", "strip_ansi": false}
	if err := Call(m, "pane.read", p, &r); err != nil {
		return "", err
	}
	if strings.TrimSpace(r.Read.Text) == "" {
		p["source"] = "visible"
		if err := Call(m, "pane.read", p, &r); err != nil {
			return "", err
		}
	}
	return strings.ReplaceAll(r.Read.Text, "\r\n", "\n"), nil
}

func AgentStatus(pane string) (string, error) {
	m, id := SplitPane(pane)
	var r struct {
		Agent Pane `json:"agent"`
	}
	if err := Call(m, "agent.get", map[string]any{"target": id}, &r); err != nil {
		return "", err
	}
	return r.Agent.AgentStatus, nil
}

func Focus(pane string) error {
	m, id := SplitPane(pane)
	return Call(m, "agent.focus", map[string]any{"target": id}, nil)
}

func Prompt(pane, text string) error {
	m, id := SplitPane(pane)
	return Call(m, "agent.prompt", map[string]any{"target": id, "text": text}, nil)
}

// Watch subscribes to pane lifecycle and update events on this machine and calls changed whenever an
// agent appears, disappears or changes status. It reconnects until stop is closed.
func Watch(stop <-chan struct{}, changed func()) {
	subs := []map[string]string{
		{"type": "pane.created"}, {"type": "pane.closed"}, {"type": "pane.exited"},
		{"type": "pane.agent_detected"}, {"type": "pane.updated"},
	}
	status := map[string]string{}
	for {
		select {
		case <-stop:
			return
		default:
		}
		c, err := net.DialTimeout("unix", SocketFor(""), 2*time.Second)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		done := make(chan struct{})
		go func() {
			select {
			case <-stop:
				c.Close()
			case <-done:
			}
		}()
		req, _ := json.Marshal(map[string]any{"id": "adw-tui:watch", "method": "events.subscribe",
			"params": map[string]any{"subscriptions": subs}})
		if _, err := c.Write(append(req, '\n')); err == nil {
			r := bufio.NewReaderSize(c, 1<<20)
			for {
				line, err := r.ReadBytes('\n')
				if err != nil {
					break
				}
				var ev struct {
					Event string `json:"event"`
					Data  struct {
						Pane *Pane `json:"pane"`
					} `json:"data"`
				}
				if json.Unmarshal(line, &ev) != nil || ev.Event == "" {
					continue
				}
				// pane.updated fires on every output; only a status change is worth a refresh.
				if p := ev.Data.Pane; p != nil && ev.Event == "pane_updated" {
					if status[p.PaneID] == p.AgentStatus {
						continue
					}
					status[p.PaneID] = p.AgentStatus
				}
				changed()
			}
		}
		close(done)
		c.Close()
		time.Sleep(2 * time.Second)
	}
}
