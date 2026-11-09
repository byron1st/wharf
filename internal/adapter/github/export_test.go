package github

// NewClientWith returns Releases that calls apiBase and downloadBase instead
// of GitHub, so tests can serve them.
func NewClientWith(apiBase, downloadBase string) Releases {
	return newClient(apiBase, downloadBase)
}
