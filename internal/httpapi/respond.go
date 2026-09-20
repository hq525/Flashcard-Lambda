package httpapi

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"flashcard_lambda/internal/persistence"
)

type errorBody struct {
	Message string `json:"message"`
}

// API Gateway proxy responses encode the JSON body a second time as a string.
// Reserve space for headers and the outer envelope below Lambda's 6 MiB limit.
const maxEncodedProxyBodyBytes = (6 << 20) - (32 << 10)

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		writeError(w, http.StatusInternalServerError)
		return
	}
	if status >= 200 && status < 300 && len(body) > persistence.MaxResultBytes {
		serverError(w, persistence.ErrResultLimit)
		return
	}
	if status >= 200 && status < 300 && len(body) > (maxEncodedProxyBodyBytes-2)/2 {
		// Small, already JSON-escaped bodies cannot exceed the envelope limit.
		// Measure large ones so escape-heavy card text returns a useful 413.
		encoded, err := json.Marshal(string(body))
		if err != nil || len(encoded) > maxEncodedProxyBodyBytes {
			serverError(w, persistence.ErrResultLimit)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// writeError sends a JSON error body. CORS headers are already on the
// response via the middleware, so browser clients can read the status.
func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorBody{Message: http.StatusText(status)})
}

func serverError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrParentNotFound) {
		writeJSON(w, http.StatusBadRequest, errorBody{Message: "The referenced parent does not exist."})
		return
	}
	if errors.Is(err, persistence.ErrResultLimit) {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{Message: "The complete result exceeds the supported item, page, or response size limit."})
		return
	}
	log.Println(err.Error())
	writeError(w, http.StatusInternalServerError)
}
