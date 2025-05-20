package common

import (
	"context"
	"log"
	"slices"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

func QueryEvents(ctx context.Context, filter nostr.Filter) (chan *nostr.Event, error) {
	ch := make(chan *nostr.Event)
	pubkey := khatru.GetAuthed(ctx)


	go func() {
    if !GENERATE_CLAIMS || !HasAccess(pubkey) || !slices.Contains(filter.Kinds, AUTH_INVITE) {
  		close(ch)
  		return
    }

  	claim := generateInvite(pubkey)
		evt := nostr.Event{
			Kind:      AUTH_INVITE,
			CreatedAt: nostr.Now(),
			Tags: nostr.Tags{
				nostr.Tag{"claim", claim},
			},
		}

		if err := evt.Sign(RELAY_SECRET); err != nil {
			log.Fatal("Failed to sign event:", err)
		} else {
			ch <- &evt
		}

		close(ch)
	}()

	return ch, nil
}

func RejectEvent(ctx context.Context, event *nostr.Event) (reject bool, msg string) {
	pubkey := khatru.GetAuthed(ctx)

	if event.Kind == AUTH_JOIN && event.PubKey == pubkey {
		handleAccessRequest(event)

		if !HasAccess(pubkey) {
			return true, "restricted: failed to validate invite code"
		}
	}

	if pubkey == "" {
		return true, "auth-required: authentication is required for access"
	}

	if AUTH_RESTRICT_USER && !HasAccess(pubkey) {
		return true, "restricted: you are not a memeber of this relay"
	}

	if AUTH_RESTRICT_AUTHOR && !HasAccess(event.PubKey) {
		return true, "restricted: event author is not a member of this relay"
	}

	return false, ""
}

func RejectFilter(ctx context.Context, filter nostr.Filter) (reject bool, msg string) {
	if slices.Contains(filter.Kinds, AUTH_JOIN) {
		return true, "restricted: join events cannot be queried"
	}

	pubkey := khatru.GetAuthed(ctx)

	if pubkey == "" {
		return true, "auth-required: authentication is required for access"
	}

	if AUTH_RESTRICT_USER && !HasAccess(pubkey) {
		return true, "restricted: you are not a memeber of this relay"
	}

	return false, ""
}

func handleAccessRequest(e *nostr.Event) {
	tag := e.Tags.GetFirst([]string{"claim"})

	if tag == nil {
		return
	}

	claim := tag.Value()

	if isValidClaim(claim) || HasAccess(consumeInvite(claim)) {
		addUserClaim(e.PubKey, claim)
	}
}
