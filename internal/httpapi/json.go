package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const maxJSONRequestBytes = 128 * 1024

// decodeJSONRequest accepts exactly one JSON value and never includes body
// content in an error or log. The reader limit also covers trailing whitespace.
func decodeJSONRequest(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	if r.ContentLength > maxBytes {
		writeError(w, http.StatusRequestEntityTooLarge)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeJSONDecodeError(w, err)
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSONDecodeError(w, err)
		return false
	}
	return true
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	var oversized *http.MaxBytesError
	if errors.As(err, &oversized) {
		writeError(w, http.StatusRequestEntityTooLarge)
		return
	}
	writeError(w, http.StatusUnprocessableEntity)
}

func validResourceID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 128
}
