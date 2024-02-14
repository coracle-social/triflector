package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"git.ekzyis.com/ekzyis/nip44"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/fiatjaf/eventstore/sqlite3"
	"github.com/fiatjaf/khatru"
	_ "github.com/joho/godotenv/autoload"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/pkg/errors"
)

var EXPIRE_AFTER, _ = time.ParseDuration("10m")

func filter[T any](ss []T, test func(T) bool) (ret []T) {
	for _, s := range ss {
		if test(s) {
			ret = append(ret, s)
		}
	}

	return
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

var nip04_conversation_keys = make(map[[32]byte][]byte)

func getNip04ConversationKey(sk string, pk string) ([]byte, error) {
	cache_key := sha256.Sum256([]byte(sk + pk))

	if key, ok := nip04_conversation_keys[cache_key]; ok {
		return key, nil
	} else {
		key, err := nip04.ComputeSharedSecret(pk, sk)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to compute nip04 conversation key")
		}

		nip04_conversation_keys[cache_key] = key

		return key, nil
	}
}

func getNip04Rumor(sk string, wrap *nostr.Event) (*nostr.Event, error) {
	wrap_key, wrap_key_err := getNip04ConversationKey(sk, wrap.PubKey)
	if wrap_key_err != nil {
		return nil, wrap_key_err
	}

	seal_json, seal_json_err := nip04.Decrypt(wrap.Content, wrap_key)
	if seal_json_err != nil {
		return nil, errors.Wrap(seal_json_err, "Failed to decrypt nip04 wrapper")
	}

	seal := nostr.Event{}
	if seal_err := json.Unmarshal([]byte(seal_json), &seal); seal_err != nil {
		return nil, errors.Wrap(seal_err, "Failed to unmarshal nip04 seal json")
	}

	seal_key, seal_key_err := getNip04ConversationKey(sk, seal.PubKey)
	if seal_key_err != nil {
		return nil, seal_key_err
	}

	rumor_json, rumor_json_err := nip04.Decrypt(seal.Content, seal_key)
	if rumor_json_err != nil {
		return nil, errors.Wrap(rumor_json_err, "Failed to decrypt nip04 seal")
	}

	rumor := nostr.Event{}
	if rumor_err := json.Unmarshal([]byte(rumor_json), &rumor); rumor_err != nil {
		return nil, errors.Wrap(rumor_err, "Failed to unmarshal nip04 rumor json")
	}

	return &rumor, nil
}

var nip44_conversation_keys = make(map[[32]byte][]byte)

func getNip44ConversationKey(sk string, pk string) ([]byte, error) {
	cache_key := sha256.Sum256([]byte(sk + pk))

	if key, ok := nip44_conversation_keys[cache_key]; ok {
		return key, nil
	} else {
		sk_obj := secp256k1.PrivKeyFromBytes([]byte(sk))
		pk_obj, err := secp256k1.ParsePubKey([]byte(pk))
		if err != nil {
			return nil, errors.Wrap(err, "")
		}

		nip44_conversation_keys[cache_key] = nip44.GenerateConversationKey(sk_obj, pk_obj)

		return nip44_conversation_keys[cache_key], nil
	}
}

func getNip44Rumor(sk string, e *nostr.Event) (*nostr.Event, error) {
	// key, err := getNip44ConversationKey(sk, e.Pubkey)
	return nil, errors.New("Not implemented")
}

func getRumor(sk string, e *nostr.Event) (*nostr.Event, error) {
	if e.Kind == 1059 {
		return getNip44Rumor(sk, e)
	}

	if e.Kind == 1060 {
		return getNip04Rumor(sk, e)
	}

	return nil, fmt.Errorf("Invalid wrapper kind: %d", e.Kind)
}

var member_list_expires = time.Unix(0, 0)
var whitelist = make(map[string]time.Time)

func checkAuth(ctx context.Context) (reject bool, msg string) {
	pubkey := khatru.GetAuthed(ctx)

	if pubkey == "" {
		return true, "auth-required: authentication is required for access"
	}

	if expires, ok := whitelist[pubkey]; !ok || expires.After(time.Now()) {
		// Group admin can always access the group relay
		if id := env("AUTH_GROUP_ID"); strings.Contains(id, pubkey) {
			whitelist[pubkey] = time.Now().Add(EXPIRE_AFTER)
		}

		// Fetch our member list and check it
		if sk := env("AUTH_GROUP_KEY"); sk != "" && member_list_expires.Before(time.Now()) {
			shared_pubkeys := make([]string, 0)
			shared_privkeys := make(map[string]string)
			pk, err := nostr.GetPublicKey(sk)

			if err != nil {
				panic(err)
			}

			// Get shared keys sent to the relay's pubkey
			for _, query := range relay.QueryEvents {
				ch, err := query(ctx, nostr.Filter{
					Tags:  nostr.TagMap{"#p": []string{pk}},
					Kinds: []int{1059, 1060},
				})

				if err != nil {
					fmt.Printf("%+v", err)
					continue
				}

				for e := range ch {
					rumor, err := getRumor(sk, e)

					if err != nil {
						fmt.Printf("%+v", err)
					} else if rumor.Kind == 24 {
						shared_sk := rumor.Tags.GetFirst([]string{"privkey"}).Value()

						if shared_sk != "" {
							shared_pk, err := nostr.GetPublicKey(shared_sk)

							if err == nil {
								shared_privkeys[shared_pk] = shared_sk
								shared_pubkeys = append(shared_pubkeys, shared_pk)
							}
						}
					}
				}
			}

			// Get member list using shared keys
			for _, query := range relay.QueryEvents {
				since := nostr.Timestamp(member_list_expires.Unix())
				ch, err := query(ctx, nostr.Filter{
					Tags:  nostr.TagMap{"#p": shared_pubkeys},
					Kinds: []int{1059, 1060},
					Since: &since,
				})

				if err != nil {
					fmt.Printf("%+v", err)
					continue
				}

				members := make([]string, 0)

				var events []*nostr.Event
				for e := range ch {
					events = append(events, e)
				}

				sort.Slice(events, func(i, j int) bool {
					return events[i].CreatedAt.Time().Before(events[j].CreatedAt.Time())
				})

				for _, e := range events {
					pk := e.Tags.GetFirst([]string{"p"}).Value()
					rumor, err := getRumor(shared_privkeys[pk], e)

					if err != nil {
						fmt.Printf("%+v", err)
					} else if rumor.Kind == 27 {
						op := rumor.Tags.GetFirst([]string{"op"}).Value()

						if op == "set" {
							members = nil
						}

						for _, tag := range rumor.Tags.GetAll([]string{"p"}) {
							pk = tag.Value()

							if op == "remove" {
								members = filter(members, func(member string) bool {
									return pk != member
								})
							} else {
								members = append(members, pk)
							}
						}
					}
				}

				for _, pubkey := range members {
					whitelist[pubkey] = time.Now().Add(EXPIRE_AFTER)
				}
			}

			member_list_expires = time.Now().Add(EXPIRE_AFTER)
		}

		// If we have custom logic, use it
		if url := env("AUTH_BACKEND"); url != "" {
			res, err := http.Get(fmt.Sprintf("%s%s", url, pubkey))

			if err == nil {
				if res.StatusCode == 200 {
					whitelist[pubkey] = time.Now().Add(EXPIRE_AFTER)
				}
			} else {
				fmt.Println(err)
			}
		}
	}

	if _, ok := whitelist[pubkey]; !ok {
		return true, "restricted: access denied"
	}

	return false, ""
}

var relay *khatru.Relay
var env func(k string, fallback ...string) (v string)

func main() {
	env = getEnv()

	relay = khatru.NewRelay()
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
