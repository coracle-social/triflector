package common

import (
	"fmt"
	"net/http"
	"slices"
	"sync"
	"time"
)

func HasAccess(pubkey string) bool {
	return HasAccessUsingWhitelist(pubkey) || HasAccessUsingClaim(pubkey) || HasAccessUsingBackend(pubkey)
}

func HasAccessUsingWhitelist(pubkey string) bool {
	return slices.Contains(AUTH_WHITELIST, pubkey)
}

func HasAccessUsingClaim(pubkey string) bool {
	return len(getUserClaims(pubkey)) > 0
}

type BackendAccess struct {
	granted bool
	expires time.Time
}

var backend_acl = make(map[string]BackendAccess)
var backend_acl_mu sync.Mutex

func HasAccessUsingBackend(pubkey string) bool {
	backend_acl_mu.Lock()
	defer backend_acl_mu.Unlock()

	// If we don't have a backend, we're done
	if AUTH_BACKEND == "" {
		return false
	}

	// If we have an un-expired entry, use it
	if access, ok := backend_acl[pubkey]; ok && access.expires.After(time.Now()) {
		return access.granted
	}

	// Fetch the url
	res, err := http.Get(fmt.Sprintf("%s%s", AUTH_BACKEND, pubkey))

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
