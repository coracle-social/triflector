package main

import (
	"fmt"
	"github.com/nbd-wtf/go-nostr"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

func checkAuthUsingEnv(pubkey string) bool {
	return strings.Contains(env("AUTH_WHITELIST"), pubkey)
}

func checkAuthUsingClaim(pubkey string) bool {
	var valid_claims = strings.Split(env("RELAY_CLAIMS"), ",")
	var claims []string

	err := backend.DB.Select(&claims, "SELECT claim FROM claim WHERE pubkey = $1", pubkey)

	if err != nil {
		fmt.Println(err)
	}

	for _, claim := range claims {
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

	backend.DB.MustExec(
		"INSERT INTO claim (pubkey, claim) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
		e.PubKey,
		tag.Value(),
	)
}
