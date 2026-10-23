// Command adw-tui is a terminal, review-first view over the ADW fleet: decisions that need you, each
// epic's in-flight segments with their diff, reviewer verdicts and PR state, and every herdr agent tab.
package main

import (
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sethgw/agent-fleet/tui/internal/herdr"
	"github.com/sethgw/agent-fleet/tui/internal/ui"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("usage: adw-tui\n\nReads adw-board --json, $ADW_HOME and the herdr socket; press ? inside for keys.")
		return
	}
	if _, err := exec.LookPath("adw-board"); err != nil {
		fmt.Fprintln(os.Stderr, "adw-tui: adw-board is not on PATH; run the agent-fleet install.sh first")
		os.Exit(1)
	}
	if path := os.Getenv("ADW_TUI_LOG"); path != "" {
		if f, err := tea.LogToFile(path, "adw-tui"); err == nil {
			defer f.Close()
		}
	}
	p := tea.NewProgram(ui.New(), tea.WithAltScreen())
	stop := make(chan struct{})
	go herdr.Watch(stop, func() { p.Send(ui.HerdrChangedMsg{}) })
	_, err := p.Run()
	close(stop)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adw-tui:", err)
		os.Exit(1)
	}
}
