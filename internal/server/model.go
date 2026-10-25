package server

type syncResponse struct {
	Upserted int  `json:"upserted"`
	Partial  bool `json:"partial"`
}

type errorResponse struct {
	Error string `json:"error"`
}
