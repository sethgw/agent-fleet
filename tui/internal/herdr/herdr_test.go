package herdr

import (
	"path/filepath"
	"testing"
)

func TestSplitPane(t *testing.T) {
	for in, want := range map[string][2]string{
		"w1:p2":     {"", "w1:p2"},
		"mac/w3:p4": {"mac", "w3:p4"},
	} {
		if m, id := SplitPane(in); m != want[0] || id != want[1] {
			t.Errorf("SplitPane(%q) = %q, %q", in, m, id)
		}
	}
}

func TestSocketFor(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("ADW_HERDR_REMOTE_DIR", "")
	if got := SocketFor(""); got != filepath.FromSlash("/h/.config/herdr/herdr.sock") {
		t.Errorf("local socket %q", got)
	}
	if got := SocketFor("mac"); got != filepath.FromSlash("/h/.config/herdr/remote/mac.sock") {
		t.Errorf("remote socket %q", got)
	}
	t.Setenv("HERDR_SOCKET_PATH", "/s.sock")
	if got := SocketFor(""); got != "/s.sock" {
		t.Errorf("override %q", got)
	}
}
