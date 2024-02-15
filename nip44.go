package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"

	"git.ekzyis.com/ekzyis/nip44"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/nbd-wtf/go-nostr"
	"github.com/pkg/errors"
)

var nip44_conversation_keys = make(map[[32]byte][]byte)
var nip44_conversation_keys_mu sync.Mutex

func getNip44ConversationKey(sk string, pk string) ([]byte, error) {
	nip44_conversation_keys_mu.Lock()
	defer nip44_conversation_keys_mu.Unlock()

	cache_key := sha256.Sum256([]byte(sk + pk))

	if key, ok := nip44_conversation_keys[cache_key]; ok {
		return key, nil
	} else {
		sk_bytes, _ := hex.DecodeString("02" + sk)
		sk_obj := secp256k1.PrivKeyFromBytes(sk_bytes)
		pk_bytes, _ := hex.DecodeString("02" + pk)
		pk_obj, err := secp256k1.ParsePubKey(pk_bytes)
		if err != nil {
			return nil, errors.Wrap(err, "")
		}

		nip44_conversation_keys[cache_key] = nip44.GenerateConversationKey(sk_obj, pk_obj)

		return nip44_conversation_keys[cache_key], nil
	}
}

func getNip44Rumor(sk string, wrap *nostr.Event) (*nostr.Event, error) {
	wrap_key, wrap_key_err := getNip44ConversationKey(sk, wrap.PubKey)
	if wrap_key_err != nil {
		return nil, wrap_key_err
	}

	seal_json, seal_json_err := nip44.Decrypt([]byte(wrap.Content), string(wrap_key))
	if seal_json_err != nil {
		return nil, errors.Wrap(seal_json_err, "Failed to decrypt nip44 wrapper")
	}

	seal := nostr.Event{}
	if seal_err := json.Unmarshal([]byte(seal_json), &seal); seal_err != nil {
		return nil, errors.Wrap(seal_err, "Failed to unmarshal nip44 seal json")
	}

	seal_key, seal_key_err := getNip44ConversationKey(sk, seal.PubKey)
	if seal_key_err != nil {
		return nil, seal_key_err
	}

	rumor_json, rumor_json_err := nip44.Decrypt([]byte(seal.Content), string(seal_key))
	if rumor_json_err != nil {
		return nil, errors.Wrap(rumor_json_err, "Failed to decrypt nip44 seal")
	}

	rumor := nostr.Event{}
	if rumor_err := json.Unmarshal([]byte(rumor_json), &rumor); rumor_err != nil {
		return nil, errors.Wrap(rumor_err, "Failed to unmarshal nip44 rumor json")
	}

	return &rumor, nil
}
