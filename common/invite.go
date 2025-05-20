package common

func generateInvite(author string) string {
	claim := randomString(8)

	PutItem("invite", claim, author)

	return claim
}

func consumeInvite(claim string) string {
	author := GetItem("invite", claim)

	if author != "" {
		DeleteItem("invite", claim)
	}

	return author
}
