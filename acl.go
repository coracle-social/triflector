package main

import (
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"github.com/nbd-wtf/go-nostr"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

func getUserClaims(pubkey string) []string {
	var stored_claims string
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("claim:" + pubkey))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			stored_claims = string(val)
			return nil
		})
	})

	if err != nil {
		if err != badger.ErrKeyNotFound {
			fmt.Println(err)
		}
		return []string{}
	}

	return strings.Split(stored_claims, ",")
}

func checkAuthUsingEnv(pubkey string) bool {
	return strings.Contains(env("AUTH_WHITELIST"), pubkey)
}

func checkAuthUsingClaim(pubkey string) bool {
	valid_claims := strings.Split(env("RELAY_CLAIMS"), ",")

	for _, claim := range getUserClaims(pubkey) {
		if slices.Contains(valid_claims, claim) {
			return true
		}
	}

	return false
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

func handleAccessRequest(e *nostr.Event) {
	tag := e.Tags.GetFirst([]string{"claim"})

	if tag == nil {
		return
	}

	claims := strings.Split(env("RELAY_CLAIMS"), ",")

	if !slices.Contains(claims, tag.Value()) {
		return
	}

	// Get existing claims
	user_claims := getUserClaims(e.PubKey)

	// Add new claim
	if !slices.Contains(user_claims, tag.Value()) {
		user_claims = append(user_claims, tag.Value())
		claims_str := strings.Join(user_claims, ",")

		// Store updated claims
		if err := db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte("claim:"+e.PubKey), []byte(claims_str))
		}); err != nil {
			fmt.Println(err)
		}
	}
}
