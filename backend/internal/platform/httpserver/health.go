package httpserver

import "net/http"

func (h *handlers) registerHealth(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}
