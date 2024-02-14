package main

import (
	"crypto/sha256"

	"git.ekzyis.com/ekzyis/nip44"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"
)

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
