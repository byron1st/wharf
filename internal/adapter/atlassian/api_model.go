package atlassian

import "encoding/json"

type searchRequest struct {
	JQL           string   `json:"jql"`
	Fields        []string `json:"fields"`
	MaxResults    int      `json:"maxResults"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

type searchResponse struct {
	// Issues stay raw so that one malformed item does not fail the page.
	Issues        []json.RawMessage `json:"issues"`
	NextPageToken string            `json:"nextPageToken"`
	IsLast        bool              `json:"isLast"`
}
