package commands

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"gopherdis/internal/persist"
	"gopherdis/internal/store"
)

func TestConfigGetReal(t *testing.T) {
	tbl := NewTable(store.New(), Env{Config: []Param{
		{"port", "7000"},
		{"bind", "127.0.0.1"},
		{"maxclients", "10"},
		{"save", "60 1"},
	}})
	tests := []struct{ in, want string }{
		{"CONFIG GET port", "*2\r\n$4\r\nport\r\n$4\r\n7000\r\n"},
		{"CONFIG GET PORT", "*2\r\n$4\r\nport\r\n$4\r\n7000\r\n"},
		{"CONFIG GET save", "*2\r\n$4\r\nsave\r\n$4\r\n60 1\r\n"}, // the server's value wins
		{"CONFIG GET appendonly", "*2\r\n$10\r\nappendonly\r\n$2\r\nno\r\n"},
		{"CONFIG GET nosuch", "*0\r\n"},
		{"CONFIG GET max*", "*2\r\n$10\r\nmaxclients\r\n$2\r\n10\r\n"},
		{"CONFIG GET port port", "*2\r\n$4\r\nport\r\n$4\r\n7000\r\n"}, // once, not twice
	}
	for _, tc := range tests {
		if got := run(t, tbl, tc.in); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := arrayReply(t, run(t, tbl, "CONFIG GET *")); len(got) != 10 {
		t.Errorf("CONFIG GET * = %v, want 5 pairs", got)
	}
}

func TestInfo(t *testing.T) {
	st := store.New()
	saver := persist.NewSaver(t.TempDir()+"/dump.gob", st)
	tbl := NewTable(st, Env{
		Saver:            saver,
		Version:          "1.2.3",
		ConnectedClients: func() int { return 4 },
		StartTime:        time.Now().Add(-90 * time.Second),
	})
	run(t, tbl, "SET a 1")
	run(t, tbl, "SET b 2")
	run(t, tbl, "NOSUCHCOMMAND") // not counted
	out := run(t, tbl, "INFO")
	if !strings.HasPrefix(out, "$") {
		t.Fatalf("INFO is not a bulk string: %q", out)
	}
	for _, want := range []string{
		"gopherdis_version:1.2.3\r\n",
		"uptime_in_seconds:90\r\n",
		"connected_clients:4\r\n",
		"total_commands_processed:2\r\n", // the two SETs; INFO itself is counted after
		"keys:2\r\n",
		"last_save_time:" + strconv.FormatInt(saver.LastSave().Unix(), 10) + "\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("INFO is missing %q:\n%s", want, out)
		}
	}
	if got := run(t, tbl, "INFO server"); !strings.Contains(got, "gopherdis_version") {
		t.Errorf("INFO server: %q", got)
	}
}
