package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
)

func getEnv() func(k string, fallback ...string) (v string) {
	var env = make(map[string]string)

	for _, item := range os.Environ() {
		parts := strings.Split(item, "=")
		env[parts[0]] = parts[1]
	}

	return func(k string, fallback ...string) (v string) {
		v = env[k]

		if v == "" && len(fallback) > 0 {
			v = fallback[0]
		}

		return v
	}
}

var whitelist = make(map[string]bool)

func checkAuth(ctx context.Context) (reject bool, msg string) {
	pubkey := khatru.GetAuthed(ctx)

	if pubkey == "" {
		return true, "auth-required: authentication is required for access"
	}

	if _, ok := whitelist[pubkey]; !ok {
		res, err := http.Get(fmt.Sprintf("%s%s", env("AUTH_BACKEND"), pubkey))
		if err != nil {
			panic(err)
		}

		whitelist[pubkey] = res.StatusCode == 200
	}

	if !whitelist[pubkey] {
		return true, "restricted: access denied"
	}

	return false, ""
}

var env func(k string, fallback ...string) (v string)

func main() {
	env = getEnv()

	relay := khatru.NewRelay()
	relay.Info.Name = env("RELAY_NAME")
	relay.Info.Icon = env("RELAY_ICON")
	relay.Info.PubKey = env("RELAY_PUBKEY")
	relay.Info.Description = env("RELAY_DESCRIPTION")

	backend := sqlite3.SQLite3Backend{DatabaseURL: "/tmp/grail-relay.sqlite"}
	if err := backend.Init(); err != nil {
		panic(err)
	}

	relay.StoreEvent = append(relay.StoreEvent,
		func(ctx context.Context, event *nostr.Event) error {
			return backend.SaveEvent(ctx, event)
		},
	)

	relay.DeleteEvent = append(relay.DeleteEvent,
		func(ctx context.Context, event *nostr.Event) error {
			return backend.DeleteEvent(ctx, event)
		},
	)

	relay.QueryEvents = append(relay.QueryEvents,
		func(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
			return backend.QueryEvents(ctx, filter)
		},
	)

	relay.RejectEvent = append(relay.RejectEvent,
		func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
			return checkAuth(ctx)
		},
	)

	relay.RejectFilter = append(relay.RejectFilter,
		func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			return checkAuth(ctx)
		},
	)

	mux := relay.Router()

	// set up other http handlers
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		fmt.Fprintf(w, `This is a nostr relay, please connect using wss://`)
	})

	port := env("PORT", "3334")

	// start the server
	fmt.Printf("running on :%s\n", port)
	http.ListenAndServe(fmt.Sprintf(":%s", port), relay)
}
