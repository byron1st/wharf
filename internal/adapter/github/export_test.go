package github

// NewClientWith returns Releases that calls apiBase and downloadBase instead
// of GitHub, so tests can serve them.
func NewClientWith(apiBase, downloadBase string) Releases {
	return newClient(apiBase, downloadBase, "")
}

// NewClientWithToken is NewClientWith with a token.
func NewClientWithToken(apiBase, downloadBase, token string) Releases {
	return newClient(apiBase, downloadBase, token)
}
