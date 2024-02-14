package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
)

func checkAuth(ctx context.Context) (reject bool, msg string) {
	pubkey := khatru.GetAuthed(ctx)

	if pubkey == "" {
		return true, "auth-required: authentication is required for access"
	}

	if checkAuthUsingEnv(pubkey) {
		return false, ""
	}

	if checkAuthUsingBackend(pubkey) {
		return false, ""
	}

	if checkAuthUsingMemberList(pubkey) {
		return false, ""
	}

	return true, "restricted: access denied"
}

var relay *khatru.Relay
var env func(k string, fallback ...string) (v string)

func main() {
	env = getEnv()

	relay_privkey := env("RELAY_PRIVATE_KEY")
	relay_pubkey, err := nostr.GetPublicKey(relay_privkey)

	if err != nil {
		fmt.Println("A valid hex RELAY_PRIVATE_KEY is required")
	}

	relay = khatru.NewRelay()
	relay.Info.PubKey = relay_pubkey
	relay.Info.Name = env("RELAY_NAME")
	relay.Info.Icon = env("RELAY_ICON")
	relay.Info.Description = env("RELAY_DESCRIPTION")

	backend := sqlite3.SQLite3Backend{DatabaseURL: "/tmp/triflector-relay.sqlite"}
	if err := backend.Init(); err != nil {
		panic(err)
	}

	relay.StoreEvent = append(relay.StoreEvent, backend.SaveEvent)
	relay.StoreEvent = append(relay.StoreEvent,
		func(ctx context.Context, event *nostr.Event) error {
			pk := event.Tags.GetFirst([]string{"p"}).Value()

			var sk string
			if pk == relay_pubkey {
				sk = relay_privkey
			} else if shared_sk, ok := shared_keys[pk]; ok {
				sk = shared_sk
			}

			if sk != "" {
				rumor, err := getRumor(sk, event)

				if err != nil {
					return err
				} else if rumor.Kind == 24 {
					handleSharedKeyEvent(rumor)
				} else if rumor.Kind == 27 {
					handleMemberListEvent(rumor)
				}
			}

			return nil
		},
	)

	relay.DeleteEvent = append(relay.DeleteEvent, backend.DeleteEvent)

	relay.QueryEvents = append(relay.QueryEvents, backend.QueryEvents)

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

	go keepMemberListInSync()

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
