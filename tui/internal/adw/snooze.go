package adw

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Same keys and file as board.py's set_snooze, so the web board and this UI share snoozes.
var snoozeKey = regexp.MustCompile(`^(d:d-[0-9a-f]{6}|p:[A-Za-z0-9:_-]{1,64})$`)

func SetSnooze(key string, until time.Time) error {
	if !snoozeKey.MatchString(key) {
		return fmt.Errorf("bad snooze key %q", key)
	}
	path := filepath.Join(Home(), "board-snoozes.json")
	sn := map[string]float64{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &sn)
	}
	now := float64(time.Now().Unix())
	for k, v := range sn {
		if v <= now {
			delete(sn, k)
		}
	}
	sn[key] = float64(until.Unix())
	b, _ := json.MarshalIndent(sn, "", "  ")
	rnd := make([]byte, 4)
	_, _ = rand.Read(rnd)
	tmp := filepath.Join(filepath.Dir(path), fmt.Sprintf(".board-snoozes.json.%d.%s.tmp", os.Getpid(), hex.EncodeToString(rnd)))
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Tomorrow8 matches board.py's "tomorrow": 8am local, at least an hour from now.
func Tomorrow8() time.Time {
	now := time.Now()
	t := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
	if t.Before(now.Add(time.Hour)) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
