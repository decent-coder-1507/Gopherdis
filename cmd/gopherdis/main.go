package main

import (
	"log"
	"time"

	"gopherdis/internal/commands"
	"gopherdis/internal/persist"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

func main() {
	const addr = ":6379"
	const dumpPath = "dump.gob"

	st := store.New()
	saver := persist.NewSaver(dumpPath, st)

	// Load saved data before accepting clients. A damaged file stops the
	// server: starting empty would quietly lose the data.
	start := time.Now()
	keys, found, err := saver.Load()
	if err != nil {
		log.Fatalf("cannot load saved data: %v", err)
	}
	if found {
		log.Printf("loaded %d keys from %s in %v", keys, dumpPath, time.Since(start).Round(time.Millisecond))
	}

	stopCleanup := st.StartCleanup()
	defer stopCleanup()

	cmds := commands.NewTable(st)
	cmds.SetSaver(saver)
	srv := server.New(cmds)
	log.Printf("gopherdis listening on %s", addr)
	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatal(err)
	}
}
