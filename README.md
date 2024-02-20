# Triflector

This is a relay based on [Khatru](https://github.com/fiatjaf/khatru) which implements a range of access controls.

## Basic configuration

The following environment variables are optional:

- `PORT` - the port to run on
- `RELAY_NAME` - the name of your relay
- `RELAY_ICON` - an icon for your relay
- `RELAY_PUBKEY` - the public key of your relay
- `RELAY_DESCRIPTION` - your relay's description
- `RELAY_CLAIMS` - a comma-separated list of claims to auto-approve for relay access
- `AUTH_WHITELIST` - a comma-separate list of pubkeys to allow access for
- `AUTH_BACKEND` - a url to delegate authorization to
- `GROUP_MEMBER_SK` - the private key of a group member, used to decrypt group messages and build member lists
- `GROUP_ADMIN_SK` - the admin private key of a group, used to auto-approve group access requests
- `GROUP_CLAIMS` - a comma-separated list of claims to auto-approve for group access

## Access control

Several different policies are available for granting access, described below. If _any_ of these checks passes, access will be granted via NIP 42 AUTH for both read and write.

### Pubkey whitelist

To allow a static list of pubkeys, set the `AUTH_WHITELIST` env variable to a comma-separated list of pubkeys.

### Arbitrary policy

You can dynamically allow/deny pubkey access by setting the `AUTH_BACKEND` env variable to a URL.

The pubkey in question will be appended to this URL and a GET request will be made against it. A 200 means the key is allowed to read and write to the relay; any other status code will deny access.

For example, providing `AUTH_BACKEND=http://example.com/check-auth?pubkey=` will result in a GET request being made to `http://example.com/check-auth?pubkey=<pubkey>`.

### Relay claims

A user may send a `kind 28934` claim event to this relay. If the `claim` tag is in the `GROUP_CLAIMS` list, the pubkey which signed the event will be granted access to the relay.

### Group-based access

Triflector supports access controls based on [NIP 87](https://github.com/nostr-protocol/nips/pull/875) group member lists.

To set this up, a few things need to be done:

- Authorize your group's admin key using another access control method so that it can publish events, even when the relay has an empty database. This is needed to bootstrap the relay's group-based access control.
- Add your relay's url to your group's relay list. This tells clients to use your relay to read and write group events.
- Provide any group member's private key using the `GROUP_MEMBER_SK` environment variable. This allows your relay to receive shared keys and decrypt group events.
- Publish a kind 10002 using your relay's `GROUP_MEMBER_SK` pointing to your relay's url (to make sure clients using the outbox model correctly deliver messages to your relay).

Once a shared key has been published to your Triflector instance, the relay will decrypt any invitations addressed to the relay's `GROUP_MEMBER_SK` in order to access the shared private keys. Then, using these keys the relay will decrypt all messages, searching for `kind 27` member list events. Any member included in the member list will then be allowed to read and write from your relay.

If auto-approval of group access requests is desired, `GROUP_ADMIN_SK` may be set to the group's admin key. This allows the relay to decrypt messages sent to the group admin, and auto-approve group access requests using a claim defined in `GROUP_CLAIMS`.

# TODO

- Use https://github.com/puzpuzpuz/xsync
