package tests

import (
	"strings"
	"testing"
)

func TestInfoAndConfigOverNetwork(t *testing.T) {
	ctx := testContext(t)
	addr := startServer(t)
	rdb := newClient(t, addr)
	other := newClient(t, addr)

	rdb.Set(ctx, "a", "1", 0)
	other.Ping(ctx) // a second connection

	info, err := rdb.Info(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"gopherdis_version:test", "connected_clients:2", "keys:1", "uptime_in_seconds:"} {
		if !strings.Contains(info, want) {
			t.Errorf("INFO is missing %q:\n%s", want, info)
		}
	}

	got, err := rdb.ConfigGet(ctx, "port").Result()
	if err != nil || got["port"] != "0" {
		t.Errorf("CONFIG GET port = %v, %v", got, err)
	}
}
