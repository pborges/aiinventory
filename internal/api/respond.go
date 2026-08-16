package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
)

const maxJSONBodyBytes = 1 << 20 // 1 MiB is ample even for prompt overrides.

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// decodeJSON applies one consistent, bounded JSON contract to every API
// endpoint. It writes the error response itself so handlers cannot
// accidentally turn an oversized request into a generic 400.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONBody(w, r, v, false)
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeJSONBody(w, r, v, true)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any, allowEmpty bool) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if allowEmpty && errors.Is(err, io.EOF) {
			return true
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return false
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
