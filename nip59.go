package main

import (
	"fmt"

	"github.com/nbd-wtf/go-nostr"
)

func getRumor(sk string, e *nostr.Event) (*nostr.Event, error) {
	if e.Kind == 1059 {
		return getNip44Rumor(sk, e)
	}

	if e.Kind == 1060 {
		return getNip04Rumor(sk, e)
	}

	return nil, fmt.Errorf("Invalid wrapper kind: %d", e.Kind)
}
