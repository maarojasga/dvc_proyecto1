package httpserver

import (
	"encoding/json"
	"net/http"
)

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, ErrBadRequest)
		return false
	}
	return true
}
