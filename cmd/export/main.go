package main

import (
	"context"
	"fmt"
	"log"

	_ "github.com/joho/godotenv/autoload"
	eventstore "github.com/fiatjaf/eventstore/badger"
	"github.com/nbd-wtf/go-nostr"
)

func main() {
	// Set up our relay backend
	backend := &eventstore.BadgerBackend{Path: "./data/events"}
	if err := backend.Init(); err != nil {
		log.Fatal("Failed to initialize backend:", err)
	}

	ctx := context.Background()

	events, err := backend.QueryEvents(ctx, nostr.Filter{})
	if err != nil {
  	log.Fatal("Failed to query events:", err)
	}

	for evt := range events {
		fmt.Println(evt)
	}
}
