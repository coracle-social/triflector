package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	eventstore "github.com/fiatjaf/eventstore/badger"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/khatru/blossom"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/afero"
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

func isBlossomAllowed(pubkey string) bool {
	shouldCheck := env("AUTH_RESTRICT_AUTHOR", "false") == "true" || env("AUTH_RESTRICT_USER", "true") == "true"

	return !shouldCheck || isAllowed(pubkey)
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

var relay *khatru.Relay
var db *badger.DB
var env func(k string, fallback ...string) (v string)

func main() {
	env = getEnv()

	// Set up our database
	var err error
	db, err = badger.Open(badger.DefaultOptions("./db.badger"))
	if err != nil {
		log.Fatal("Failed to open badger db:", err)
	}

	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for range ticker.C {
		again:
			err := db.RunValueLogGC(0.7)
			if err == nil {
				goto again
			}
		}
	}()

	defer ticker.Stop()
	defer db.Close()

	// Set up our relay
	relay = khatru.NewRelay()
	relay.Info.Name = env("RELAY_NAME")
	relay.Info.Icon = env("RELAY_ICON")
	relay.Info.PubKey = env("RELAY_PUBKEY")
	relay.Info.Description = env("RELAY_DESCRIPTION")

	// Set up our relay backend
	backend := &eventstore.BadgerBackend{Path: "./events.badger"}
	if err := backend.Init(); err != nil {
		log.Fatal("Failed to initialize backend:", err)
	}

	relay.OnConnect = append(relay.OnConnect, khatru.RequestAuth)

	relay.StoreEvent = append(relay.StoreEvent, backend.SaveEvent)

	relay.DeleteEvent = append(relay.DeleteEvent, backend.DeleteEvent)

	relay.QueryEvents = append(relay.QueryEvents, backend.QueryEvents)

	relay.RejectEvent = append(relay.RejectEvent,
		func(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
			if event.Kind == AUTH_JOIN {
				handleAccessRequest(event)

				if env("AUTH_RESTRICT_USER", "true") == "true" && !isAllowed(event.PubKey) {
					return true, "restricted: failed to validate invite code"
				}

				return false, ""
			}

			if env("AUTH_RESTRICT_AUTHOR", "false") == "true" && !isAllowed(event.PubKey) {
				return true, "restricted: event author is not a member of this relay"
			}

			return checkAuth(khatru.GetAuthed(ctx))
		},
	)

	relay.RejectFilter = append(relay.RejectFilter,
		func(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
			if slices.Contains(filter.Kinds, AUTH_JOIN) {
				return true, "restricted: join events cannot be queried"
			}

			return checkAuth(khatru.GetAuthed(ctx))
		},
	)

	// Blossom

	fs := afero.NewOsFs()
	blossomPath := "./blossom/"

	if err := fs.MkdirAll(blossomPath, 0755); err != nil {
		log.Fatal("🚫 error creating blossom path:", err)
	}

	bldb := &eventstore.BadgerBackend{Path: "./blossom.badger"}
	if err := bldb.Init(); err != nil {
		log.Fatal("Failed to initialize blossom backend:", err)
	}

	bl := blossom.New(relay, "https://"+env("RELAY_URL", "localhost:3334"))

	bl.Store = blossom.EventStoreBlobIndexWrapper{Store: bldb, ServiceURL: bl.ServiceURL}

	bl.StoreBlob = append(bl.StoreBlob, func(ctx context.Context, sha256 string, body []byte) error {
		file, err := fs.Create(blossomPath + sha256)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, bytes.NewReader(body)); err != nil {
			return err
		}

		return nil
	})

	bl.LoadBlob = append(bl.LoadBlob, func(ctx context.Context, sha256 string) (io.ReadSeeker, error) {
		return fs.Open(blossomPath + sha256)
	})

	bl.DeleteBlob = append(bl.DeleteBlob, func(ctx context.Context, sha256 string) error {
		return fs.Remove(blossomPath + sha256)
	})

	bl.RejectUpload = append(bl.RejectUpload, func(ctx context.Context, auth *nostr.Event, size int, ext string) (bool, string, int) {
		if size > 10*1024*1024 {
			return true, "file too large", 413
		}

		if auth == nil || !isBlossomAllowed(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, ext, size
	})

	bl.RejectGet = append(bl.RejectGet, func(ctx context.Context, auth *nostr.Event, sha256 string) (bool, string, int) {
		if auth == nil || !isBlossomAllowed(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, "", 200
	})

	bl.RejectList = append(bl.RejectList, func(ctx context.Context, auth *nostr.Event, pubkey string) (bool, string, int) {
		if auth == nil || !isBlossomAllowed(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, "", 200
	})

	bl.RejectDelete = append(bl.RejectDelete, func(ctx context.Context, auth *nostr.Event, sha256 string) (bool, string, int) {
		if auth == nil || !isBlossomAllowed(auth.PubKey) {
			return true, "unauthorized", 403
		}

		return false, "", 200
	})

	// Merge everything into a single handler and start the server

	mux := relay.Router()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		fmt.Fprintf(w, `This is a nostr relay, please connect using wss://`)
	})

	port := env("PORT", "3334")

	fmt.Printf("running on :%s\n", port)

	http.ListenAndServe(fmt.Sprintf(":%s", port), relay)
}
