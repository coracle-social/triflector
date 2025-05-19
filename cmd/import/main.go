package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"

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

	// Read events from stdin line by line
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Parse JSON into nostr.Event
		var event nostr.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			log.Printf("Failed to parse event JSON: %v\n", err)
			continue
		}

		// Validate the event
		if ok, err := event.CheckSignature(); !ok {
			log.Printf("Invalid event signature: %v (%s)\n", err, event.ID)
			continue
		}

		// Save the event
		if err := backend.SaveEvent(nil, &event); err != nil {
			log.Printf("%v (%s)\n", err, event.ID)
			continue
		}

		log.Printf("Imported event %s\n", event.ID)
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("Error reading from stdin:", err)
	}
}
