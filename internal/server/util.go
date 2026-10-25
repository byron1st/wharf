package server

import (
	"encoding/json"
	"net/http"
)

// writeInternalError hides the cause from the client; it is logged instead.
func writeInternalError(w http.ResponseWriter, err error) {
	msg := "internal server error"
	if err != nil {
		msg = err.Error()
	}

	writeJSON(w, http.StatusInternalServerError, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// The status is already sent, so an encoding or write failure, such as a
	// closed connection, cannot be reported to the client.
	_ = json.NewEncoder(w).Encode(body)
}
