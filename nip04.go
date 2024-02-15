package main

import (
	"crypto/sha256"
	"encoding/json"
	"sync"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip04"
	"github.com/pkg/errors"
)

var nip04_conversation_keys = make(map[[32]byte][]byte)
var nip04_conversation_keys_mu sync.Mutex

func getNip04ConversationKey(sk string, pk string) ([]byte, error) {
	nip04_conversation_keys_mu.Lock()
	defer nip04_conversation_keys_mu.Unlock()

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
