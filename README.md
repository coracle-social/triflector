# Frith

This is a relay based on [Khatru](https://github.com/fiatjaf/khatru) which implements a range of access controls.

## Basic configuration

The following environment variables are optional:

- `PORT` - the port to run on
- `RELAY_URL` - the url of your relay
- `RELAY_NAME` - the name of your relay
- `RELAY_ICON` - an icon for your relay
- `RELAY_PUBKEY` - the public key of your relay
- `RELAY_DESCRIPTION` - your relay's description
- `RELAY_CLAIMS` - a comma-separated list of claims to auto-approve for relay access
- `AUTH_BACKEND` - a url to delegate authorization to
- `AUTH_WHITELIST` - a comma-separate list of pubkeys to allow access for
- `AUTH_RESTRICT_USER` - whether to only accept events published by authenticated users. Defaults to `true`. If `false`, no AUTH challenge will be sent.
- `AUTH_RESTRICT_AUTHOR` - whether to only accept events signed by authorized users. Defaults to `false`.

## Access control

Several different policies are available for granting access, described below. If _any_ of these checks passes, access will be granted via NIP 42 AUTH for both read and write.

### Pubkey whitelist

To allow a static list of pubkeys, set the `AUTH_WHITELIST` env variable to a comma-separated list of pubkeys.

### Arbitrary policy

You can dynamically allow/deny pubkey access by setting the `AUTH_BACKEND` env variable to a URL.

The pubkey in question will be appended to this URL and a GET request will be made against it. A 200 means the key is allowed to read and write to the relay; any other status code will deny access.

For example, providing `AUTH_BACKEND=http://example.com/check-auth?pubkey=` will result in a GET request being made to `http://example.com/check-auth?pubkey=<pubkey>`.

### Relay claims

A user may send a `kind 28934` claim event to this relay. If the `claim` tag is in the `RELAY_CLAIMS` list, the pubkey which signed the event will be granted access to the relay.

## Docker

You can use Docker Compose or Portainer Stacks to run a container:

```
services:

  frith:
    image: ghcr.io/coracle-social/frith
    container_name: frith
    restart: unless-stopped
    networks:
      - frithnet
    ports:
      - 3334:3334
    environment:
      - DATABASE_URL=postgres://frith:YOUR_PASSWORD_HERE@database:5432/frith?sslmode=disable

  database:
    image: postgres
    container_name: frith_db
    restart: unless-stopped
    networks:
      - frithnet
    volumes:
      - frithdata:/var/lib/postgresql/data
    environment:
      - POSTGRES_DB=frith
      - POSTGRES_USER=frith
      - POSTGRES_PASSWORD=YOUR_PASSWORD_HERE

networks:

  frithnet:

volumes:

  frithdata:
```

Make sure to change the example postgres password in both DATABASE_URL and POSTGRES_PASSWORD.

You can add the environment variables from [Basic configuration](#basic-configuration) to the `environment:` section under `frith:` to customize your relay.
