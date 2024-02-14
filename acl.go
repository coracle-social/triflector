package main

import (
	"context"
	"fmt"
	"github.com/nbd-wtf/go-nostr"
	"net/http"
	"sort"
	"strings"
	"time"
)

func checkAuthUsingGroupId(pubkey string) bool {
	// Group admin can always access the group relay
	return strings.Contains(env("AUTH_GROUP_ID"), pubkey)
}

type BackendAccess struct {
	granted bool
	expires time.Time
}

var backend_acl = make(map[string]BackendAccess)

func checkAuthUsingBackend(pubkey string) bool {
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

var shared_keys = make(map[string]string)
var shared_keys_last_sync = time.Unix(0, 0)

func syncSharedKeys() {
	sk := env("AUTH_GROUP_KEY")
	pk, err := nostr.GetPublicKey(sk)

	if err != nil {
		panic(err)
	}

	for _, query := range relay.QueryEvents {
		since := nostr.Timestamp(shared_keys_last_sync.Unix())
		ch, err := query(context.Background(), nostr.Filter{
			Tags:  nostr.TagMap{"#p": []string{pk}},
			Kinds: []int{1059, 1060},
			Since: &since,
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
						shared_keys[shared_pk] = shared_sk
					}
				}
			}
		}
	}
}

var member_list_acl = make(map[string]bool)
var member_list_last_sync = time.Unix(0, 0)

func checkAuthUsingMemberList(pubkey string) bool {
	return member_list_acl[pubkey]
}

func syncMemberList() {
	for _, query := range relay.QueryEvents {
		since := nostr.Timestamp(member_list_last_sync.Unix())
		ch, err := query(context.Background(), nostr.Filter{
			Tags:  nostr.TagMap{"#p": keys(shared_keys)},
			Kinds: []int{1059, 1060},
			Since: &since,
		})

		if err != nil {
			fmt.Printf("%+v", err)
			continue
		}

		member_list_last_sync = time.Now()

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
				op := rumor.Tags.GetFirst([]string{"op"}).Value()

				if op == "set" {
					member_list_acl = make(map[string]bool)
				}

				for _, tag := range rumor.Tags.GetAll([]string{"p"}) {
					member_list_acl[tag.Value()] = op != "remove"
				}
			}
		}
	}
}

func keepMemberListInSync() {
	if env("AUTH_GROUP_KEY") != "" {
		syncSharedKeys()
		syncMemberList()

		fmt.Println(member_list_acl)

		for range time.Tick(time.Minute * 5) {
			syncSharedKeys()
			syncMemberList()
		}
	}
}
