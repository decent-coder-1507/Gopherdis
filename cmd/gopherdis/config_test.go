package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	c, err := parseFlags(nil, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	want := config{port: 6379, bind: "127.0.0.1", dir: ".", dbfilename: "dump.gob",
		saveInterval: 60 * time.Second, maxclients: 10000, loglevel: "info"}
	if *c != want {
		t.Fatalf("defaults = %+v, want %+v", *c, want)
	}
	if c.addr() != "127.0.0.1:6379" {
		t.Errorf("addr = %q", c.addr())
	}
}

func TestFlags(t *testing.T) {
	dir := t.TempDir()
	c, err := parseFlags([]string{"--port", "7000", "--dir", dir, "--bind=0.0.0.0", "-save-interval", "5m",
		"--requirepass", "secret", "--maxclients", "5", "--loglevel", "debug", "--dbfilename", "data.gob"}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if c.port != 7000 || c.dir != dir || c.bind != "0.0.0.0" || c.saveInterval != 5*time.Minute ||
		c.requirepass != "secret" || c.maxclients != 5 || c.loglevel != "debug" || c.dbfilename != "data.gob" {
		t.Fatalf("got %+v", *c)
	}
	if c.addr() != "0.0.0.0:7000" || !strings.HasSuffix(c.dumpPath(), "data.gob") {
		t.Errorf("addr %q, dump path %q", c.addr(), c.dumpPath())
	}
}

func TestBadFlags(t *testing.T) {
	cases := [][]string{
		{"--port", "0"},
		{"--port", "70000"},
		{"--port", "abc"},
		{"--bind", "not an ip"},
		{"--dbfilename", "sub/dump.gob"},
		{"--dbfilename", ""},
		{"--save-interval", "-1s"},
		{"--save-interval", "60"}, // needs a unit
		{"--maxclients", "0"},
		{"--loglevel", "loud"},
		{"--dir", "/no/such/folder/here"},
		{"--nosuchflag"},
		{"stray-argument"},
	}
	for _, args := range cases {
		if _, err := parseFlags(args, &bytes.Buffer{}); err == nil {
			t.Errorf("%v: accepted", args)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	// --version works even with settings that would otherwise be refused.
	c, err := parseFlags([]string{"--version", "--dir", "/no/such/folder"}, &bytes.Buffer{})
	if err != nil || !c.showVersion {
		t.Fatalf("--version: %+v, %v", c, err)
	}
}

func TestHelp(t *testing.T) {
	var out bytes.Buffer
	_, err := parseFlags([]string{"--help"}, &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("err = %v", err)
	}
	for _, name := range []string{"--port", "--bind", "--dir", "--dbfilename", "--save-interval",
		"--requirepass", "--maxclients", "--loglevel", "--version"} {
		if !strings.Contains(out.String(), name) {
			t.Errorf("help is missing %s:\n%s", name, out.String())
		}
	}
}

func TestParams(t *testing.T) {
	c, _ := parseFlags([]string{"--port", "7000"}, &bytes.Buffer{})
	got := map[string]string{}
	for _, p := range c.params() {
		got[p.Name] = p.Value
	}
	if got["port"] != "7000" || got["save"] != "60 1" || got["appendonly"] != "no" || got["maxclients"] != "10000" {
		t.Errorf("params = %v", got)
	}
	c, _ = parseFlags([]string{"--save-interval", "0"}, &bytes.Buffer{})
	for _, p := range c.params() {
		if p.Name == "save" && p.Value != "" {
			t.Errorf("save with auto-save off = %q", p.Value)
		}
	}
}
