package common

import (
	"slices"
	"strings"
)

func isValidClaim(claim string) bool {
	return slices.Contains(RELAY_CLAIMS, claim)
}

func getUserClaims(pubkey string) []string {
  return split(GetItem("claim", pubkey), ",")
}

func addUserClaim(pubkey string, claim string) {
	claims := getUserClaims(pubkey)

	if !slices.Contains(claims, claim) {
		claims = append(claims, claim)

		PutItem("claim", pubkey, strings.Join(claims, ","))
	}
}
