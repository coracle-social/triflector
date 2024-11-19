package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/fiatjaf/eventstore/postgresql"
	"github.com/fiatjaf/khatru"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
)

var AUTH_JOIN = 28934

func isAllowed(pubkey string) bool {
	if checkAuthUsingEnv(pubkey) {
		return true
	}

	if checkAuthUsingClaim(pubkey) {
		return true
	}

	if checkAuthUsingBackend(pubkey) {
		return true
	}

	return false
}

func checkAuth(pubkey string) (reject bool, msg string) {
	if env("AUTH_RESTRICT_USER", "true") == "true" {
		if pubkey == "" {
			return true, "auth-required: authentication is required for access"
		}

		if !isAllowed(pubkey) {
			return true, "restricted: access denied"
		}
	}

	return false, ""
}

func migrate(db *sqlx.DB) {
	db.MustExec(`
    CREATE TABLE IF NOT EXISTS claim (
      pubkey char(64) NOT NULL,
      claim text NOT NULL
    );

    CREATE UNIQUE INDEX IF NOT EXISTS claim__pubkey_claim ON claim (pubkey, claim);
  `)
}

var relay *khatru.Relay
var backend postgresql.PostgresBackend
var env func(k string, fallback ...string) (v string)

func main() {
	env = getEnv()

	relay = khatru.NewRelay()
	relay.Info.Name = env("RELAY_NAME")
	relay.Info.Icon = env("RELAY_ICON")
	relay.Info.PubKey = env("RELAY_PUBKEY")
	relay.Info.Description = env("RELAY_DESCRIPTION")

	backend = postgresql.PostgresBackend{DatabaseURL: env("DATABASE_URL")}
	if err := backend.Init(); err != nil {
		panic(err)
	}

	migrate(backend.DB)

	relay.StoreEvent = append(relay.StoreEvent, backend.SaveEvent)

	relay.DeleteEvent = append(relay.DeleteEvent, backend.DeleteEvent)

	relay.QueryEvents = append(relay.QueryEvents, backend.QueryEvents)

	relay.RejectEvent = append(relay.RejectEvent,
		func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
			if event.Kind == AUTH_JOIN {
				handleAccessRequest(event)
			}

			if env("AUTH_RESTRICT_AUTHOR", "false") == "true" && !isAllowed(event.PubKey) {
				return true, "restricted: event author is not in the ACL"
			}

			return checkAuth(khatru.GetAuthed(ctx))
		},
	)

	relay.RejectFilter = append(relay.RejectFilter,
		func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			if slices.Contains(filter.Kinds, 28934) {
				return true, "restricted: access denied"
			}

			return checkAuth(khatru.GetAuthed(ctx))
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
