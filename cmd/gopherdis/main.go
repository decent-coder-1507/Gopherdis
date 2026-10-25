package main

import (
	"log"

	"gopherdis/internal/commands"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

func main() {
	const addr = ":6379"
	st := store.New()
	stopCleanup := st.StartCleanup()
	defer stopCleanup()

	srv := server.New(commands.NewTable(st))
	log.Printf("gopherdis listening on %s", addr)
	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatal(err)
	}
}
