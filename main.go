package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	"github.com/jmoiron/sqlx"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
)

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

	if checkAuthUsingMemberList(pubkey) {
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
      pubkey string NOT NULL,
      claim string NOT NULL,
      type string NOT NULL
    );

    CREATE UNIQUE INDEX IF NOT EXISTS claim__pubkey_claim ON claim (pubkey, claim, type);
  `)
}

var relay *khatru.Relay
var backend sqlite3.SQLite3Backend
var env func(k string, fallback ...string) (v string)

func main() {
	env = getEnv()

	group_admin_sk := env("GROUP_ADMIN_SK")
	group_admin_pk, _ := nostr.GetPublicKey(group_admin_sk)

	relay = khatru.NewRelay()
	relay.Info.Name = env("RELAY_NAME")
	relay.Info.Icon = env("RELAY_ICON")
	relay.Info.PubKey = env("RELAY_PUBKEY")
	relay.Info.Description = env("RELAY_DESCRIPTION")

	backend = sqlite3.SQLite3Backend{DatabaseURL: "./relay.db"}
	if err := backend.Init(); err != nil {
		panic(err)
	}

	migrate(backend.DB)

	relay.StoreEvent = append(relay.StoreEvent, backend.SaveEvent)

	relay.DeleteEvent = append(relay.DeleteEvent, backend.DeleteEvent)

	relay.QueryEvents = append(relay.QueryEvents, backend.QueryEvents)

	relay.RejectEvent = append(relay.RejectEvent,
		func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
			if event.Kind == 28934 {
				handleRelayAccessRequest(event)
			}

			if tag := event.Tags.GetFirst([]string{"p"}); tag != nil && (event.Kind == 1059 || event.Kind == 1060) {
				pk := tag.Value()

				var sk string
				if pk == group_admin_pk {
					sk = group_admin_sk
				} else {
					shared_keys_mu.RLock()

					if shared_sk, ok := shared_keys[pk]; ok {
						sk = shared_sk
					}

					shared_keys_mu.RUnlock()
				}

				if sk != "" {
					rumor, err := getRumor(sk, event)

					if err != nil {
						fmt.Println(err)
					} else if rumor.Kind == 24 {
						handleSharedKeyEvent(rumor)
					} else if rumor.Kind == 25 {
						handleGroupAccessRequest(rumor)
					} else if rumor.Kind == 27 {
						handleMemberListEvent(rumor)
					}
				}
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
