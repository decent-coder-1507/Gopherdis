package main

import (
	"log"

	"gopherdis/internal/commands"
	"gopherdis/internal/server"
	"gopherdis/internal/store"
)

func main() {
	const addr = ":6379"
	srv := server.New(commands.NewTable(store.New()))
	log.Printf("gopherdis listening on %s", addr)
	if err := srv.ListenAndServe(addr); err != nil {
		log.Fatal(err)
	}
}
