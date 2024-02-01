package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"database/sql"
	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nbd-wtf/go-nostr"
)

func execSql(db *sql.DB, sql string) {
	_, err := db.Exec(sql)
	if err != nil {
		panic(err)
	}
}

func getDb() *sql.DB {
	db, err := sql.Open("sqlite3", "/tmp/smoxy-people.sqlite")
	if err != nil {
		panic(err)
	}

	execSql(db, `
  	CREATE TABLE IF NOT EXISTS kind3 (
    	pubkey TEXT PRIMARY KEY,
      event TEXT
  	);`,
	)

	execSql(db, `
  	CREATE TABLE IF NOT EXISTS kind10003 (
    	pubkey TEXT PRIMARY KEY,
      event TEXT
  	);`,
	)

	return db
}

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

type UserData struct {
	Kind3     *nostr.Event
	Kind10002 *nostr.Event
}

var users = make(map[string]UserData)

func selectLatestEvent(a *nostr.Event, b *nostr.Event) *nostr.Event {
	if a == nil {
		return b
	}

	if b == nil {
		return a
	}

	if a.CreatedAt > b.CreatedAt {
		return a
	} else {
		return b
	}
}

func loadUserData(pubkey string) {
	// Set this so we don't try to fetch user data multiple times
	users[pubkey] = UserData{}

	ctx := context.Background()
	relay, err := nostr.RelayConnect(ctx, "wss://purplepag.es")
	if err != nil {
		panic(err)
	}

	var filters = []nostr.Filter{{
		Kinds:   []int{3, 10002},
		Authors: []string{pubkey},
		Limit:   10,
	}}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		panic(err)
	}

	var kind3 *nostr.Event
	var kind10002 *nostr.Event

	for ev := range sub.Events {
		if ev.Kind == 3 {
			kind3 = selectLatestEvent(kind3, ev)
		}

		if ev.Kind == 10002 {
			kind10002 = selectLatestEvent(kind10002, ev)
		}
	}

	users[pubkey] = UserData{kind3, kind10002}
}

func checkAuth(ctx context.Context) (reject bool, msg string) {
	pubkey := khatru.GetAuthed(ctx)

	if pubkey == "" {
		return true, "auth-required: need to know who you are to proxy successfully"
	}

	if _, ok := users[pubkey]; !ok {
		loadUserData(pubkey)
	}

	return false, ""
}

func main() {
	db := getDb()
	defer db.Close()

	env := getEnv()

	relay := khatru.NewRelay()
	relay.Info.Name = env("RELAY_NAME")
	relay.Info.Icon = env("RELAY_ICON")
	relay.Info.PubKey = env("RELAY_PUBKEY")
	relay.Info.Description = env("RELAY_DESCRIPTION")

	backend := sqlite3.SQLite3Backend{DatabaseURL: "/tmp/smoxy-relay.sqlite"}
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
