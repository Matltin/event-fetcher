package main

import (
	"log"

	"github.com/Matltin/event-fetcher/eventsdb"
)

func main() {
	cfg := eventsdb.LoadConfig()

	service := eventsdb.NewIndexerService(cfg)

	if err := service.Start(); err != nil {
		log.Fatal(err)
	}
}