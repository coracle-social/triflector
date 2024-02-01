package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

func main() {
	var env = make(map[string]string)

	for _, item := range os.Environ() {
		parts := strings.Split(item, "=")
		env[parts[0]] = parts[1]
	}

	getenv := func(k string, fallback ...string) (v string) {
		v = env[k]

		if v == "" && len(fallback) > 0 {
			v = fallback[0]
		}

		return v
	}

	// create the relay instance
	relay := khatru.NewRelay()

	// set up some basic properties (will be returned on the NIP-11 endpoint)
	relay.Info.Name = getenv("RELAY_NAME")
	relay.Info.PubKey = getenv("RELAY_PUBKEY")
	relay.Info.Description = getenv("RELAY_DESCRIPTION")
	relay.Info.Icon = getenv("RELAY_ICON")

	backend := sqlite3.SQLite3Backend{DatabaseURL: "/tmp/smoxy.sqlite"}
	if err := backend.Init(); err != nil {
		panic(err)
	}

	relay.StoreEvent = append(relay.StoreEvent,
		func(ctx context.Context, event *nostr.Event) error {
			return backend.SaveEvent(ctx, event)
		},
	)

	relay.DeleteEvent = append(relay.StoreEvent,
		func(ctx context.Context, event *nostr.Event) error {
			return backend.DeleteEvent(ctx, event)
		},
	)

	relay.QueryEvents = append(relay.QueryEvents,
		func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
			fmt.Println(khatru.GetAuthed(ctx))
			return backend.QueryEvents(ctx, filter)
		},
	)

	// you can request auth by rejecting an event or a request with the prefix "auth-required: "
	relay.RejectFilter = append(relay.RejectFilter,
		func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			if khatru.GetAuthed(ctx) == "" {
				return true, "auth-required: need to know who you are to proxy successfully"
			}

			return false, ""
		},
	)

	mux := relay.Router()

	// set up other http handlers
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		fmt.Fprintf(w, `This is a nostr relay, please connect using wss://`)
	})

	port := getenv("PORT", "3334")

	// start the server
	fmt.Printf("running on :%s\n", port)
	http.ListenAndServe(fmt.Sprintf(":%s", port), relay)
}
