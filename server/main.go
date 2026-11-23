package main

import (
	"log"

	"lixr/server/internals"
)

func main() {
	if err := internals.Run(); err != nil {
		log.Fatal(err)
	}
}
