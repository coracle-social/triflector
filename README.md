# Triflector

This is a relay implemented using [Khatru](https://github.com/fiatjaf/khatru) which implements a range of access controls.

## Basic configuration

The following environment variables are required:

- `RELAY_PRIVATE_KEY` - this is the private key of the relay. Avoid re-using private keys from other contexts.

The following environment variables are optional:

- `PORT` - the port to run on
- `RELAY_NAME` - the name of your relay
- `RELAY_ICON` - an icon for your relay
- `RELAY_DESCRIPTION` - your relay's description

## Access control

Several different policies are available for granting access, described below. If _any_ of these checks passes, access will be granted via NIP 42 AUTH for both read and write.

### Pubkey whitelist

To allow a static list of pubkeys, set the `PUBKEY_WHITELIST` env variable to a comma-separated list of pubkeys.

### Arbitrary policy

You can dynamically allow/deny pubkey access by setting the `AUTH_BACKEND` env variable to a URL.

The pubkey in question will be appended to this URL and a GET request will be made against it. A 200 means the key is allowed to read and write to the relay; any other status code will deny access.

For example, providing `AUTH_BACKEND=http://example.com/check-auth?pubkey=` will result in a GET request being made to `http://example.com/check-auth?pubkey=<pubkey>`.

### Group-based access

Triflector supports access controls based on [NIP 87](https://github.com/nostr-protocol/nips/pull/875) group member lists.

To set this up, a few things need to be done:

- Add your relay's url to your group's relay list
- Add your relay's public key to your group's member list

Once a shared key has been published to your Triflector instance, the relay will decrypt any invitations addressed to the relay's pubkey in order to access the shared private keys. Then, using these keys the relay will decrypt all messages, searching for `kind 27` member list events. Any member included in the member list will then be allowed to read and write from your relay.
