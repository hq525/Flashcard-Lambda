package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
)

type errorBody struct {
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		writeError(w, http.StatusInternalServerError)
		return
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
	log.Println(err.Error())
	writeError(w, http.StatusInternalServerError)
}
