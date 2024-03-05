package main

import (
	"context"
	"fmt"
	"github.com/nbd-wtf/go-nostr"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

func checkAuthUsingEnv(pubkey string) bool {
	// Group admin can always access the group relay
	return strings.Contains(env("AUTH_WHITELIST"), pubkey)
}

func checkAuthUsingClaim(pubkey string) bool {
	var claim string

	err := backend.DB.Get(&claim, "SELECT claim FROM claim WHERE pubkey = $1", pubkey)

	if err != nil {
		fmt.Println(err)
	}

	return slices.Contains(strings.Split(env("RELAY_CLAIMS"), ","), claim)
}

type BackendAccess struct {
	granted bool
	expires time.Time
}

var backend_acl = make(map[string]BackendAccess)
var backend_acl_mu sync.Mutex

func checkAuthUsingBackend(pubkey string) bool {
	backend_acl_mu.Lock()
	defer backend_acl_mu.Unlock()

	url := env("AUTH_BACKEND")

	// If we don't have a backend, we're done
	if url == "" {
		return false
	}

	// If we have an un-expired entry, use it
	if access, ok := backend_acl[pubkey]; ok && access.expires.After(time.Now()) {
		return access.granted
	}

	// Fetch the url
	res, err := http.Get(fmt.Sprintf("%s%s", url, pubkey))

	// If we get a 200, consider it good
	if err == nil {
		expire_after, _ := time.ParseDuration("1m")

		backend_acl[pubkey] = BackendAccess{
			granted: res.StatusCode == 200,
			expires: time.Now().Add(expire_after),
		}
	} else {
		fmt.Println(err)
	}

	return backend_acl[pubkey].granted
}

func handleAccessRequest(e *nostr.Event, claims []string, claim_type string) {
	tag := e.Tags.GetFirst([]string{"claim"})

	if tag == nil {
		return
	}

	if !slices.Contains(claims, tag.Value()) {
		return
	}

	backend.DB.MustExec(
		"INSERT INTO claim (pubkey, claim, type) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
		e.PubKey,
		tag.Value(),
		claim_type,
	)
}

func handleRelayAccessRequest(e *nostr.Event) {
	claims := strings.Split(env("RELAY_CLAIMS"), ",")

	handleAccessRequest(e, claims, "relay")
}

func handleGroupAccessRequest(e *nostr.Event) {
	claims := strings.Split(env("GROUP_CLAIMS"), ",")

	handleAccessRequest(e, claims, "group")
}

var shared_keys = make(map[string]string)
var shared_keys_mu sync.RWMutex

func handleSharedKeyEvent(e *nostr.Event) {
	shared_keys_mu.Lock()
	defer shared_keys_mu.Unlock()

	shared_sk := e.Tags.GetFirst([]string{"privkey"}).Value()

	if shared_sk != "" {
		shared_pk, err := nostr.GetPublicKey(shared_sk)

		if err == nil {
			shared_keys[shared_pk] = shared_sk
		}
	}
}

func syncSharedKeys() {
	sk := env("GROUP_ADMIN_SK")
	pk, err := nostr.GetPublicKey(sk)

	if err != nil {
		panic(err)
	}

	for _, query := range relay.QueryEvents {
		ch, err := query(context.Background(), nostr.Filter{
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
				handleSharedKeyEvent(rumor)
			}
		}
	}
}

var member_list_acl = make(map[string]bool)
var member_list_acl_mu sync.RWMutex
var latest_member_list = time.Unix(0, 0)
var latest_member_list_mu sync.RWMutex

func checkAuthUsingMemberList(pubkey string) bool {
	member_list_acl_mu.RLock()
	defer member_list_acl_mu.RUnlock()
	return member_list_acl[pubkey]
}

func handleMemberListEvent(e *nostr.Event) {
	member_list_acl_mu.Lock()
	defer member_list_acl_mu.Unlock()
	latest_member_list_mu.Lock()
	defer latest_member_list_mu.Unlock()

	created_at := e.CreatedAt.Time()

	if created_at.After(latest_member_list) {
		latest_member_list = created_at

		op := e.Tags.GetFirst([]string{"op"}).Value()

		if op == "set" {
			member_list_acl = make(map[string]bool)
		}

		for _, tag := range e.Tags.GetAll([]string{"p"}) {
			member_list_acl[tag.Value()] = op != "remove"
		}
	}
}

func syncMemberList() {
	shared_keys_mu.RLock()
	defer shared_keys_mu.RUnlock()
	latest_member_list_mu.RLock()
	defer latest_member_list_mu.RUnlock()

	for _, query := range relay.QueryEvents {
		since := nostr.Timestamp(latest_member_list.Unix())
		ch, err := query(context.Background(), nostr.Filter{
			Tags:  nostr.TagMap{"#p": keys(shared_keys)},
			Kinds: []int{1059, 1060},
			Since: &since,
		})

		if err != nil {
			fmt.Printf("%+v", err)
			continue
		}

		var events []*nostr.Event
		for e := range ch {
			events = append(events, e)
		}

		sort.Slice(events, func(i, j int) bool {
			return events[i].CreatedAt.Time().Before(events[j].CreatedAt.Time())
		})

		for _, e := range events {
			pk := e.Tags.GetFirst([]string{"p"}).Value()
			rumor, err := getRumor(shared_keys[pk], e)

			if err != nil {
				fmt.Printf("%+v", err)
			} else if rumor.Kind == 27 {
				handleMemberListEvent(rumor)
			}
		}
	}
}

func keepMemberListInSync() {
	syncSharedKeys()
	syncMemberList()

	for range time.Tick(time.Minute * 5) {
		syncSharedKeys()
		syncMemberList()
	}
}
