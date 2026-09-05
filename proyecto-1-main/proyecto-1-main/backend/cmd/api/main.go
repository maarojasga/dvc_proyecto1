// Command api expone la API REST (/api/v1) del monolito modular.
package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/DES-SOLUCIONES-CLOUD/proyecto-1/backend/internal/config"
)

func main() {
	cfg := config.Load()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", healthHandler)

	log.Printf("api escuchando en :%s (env=%s)", cfg.HTTPPort, cfg.Env)
	if err := http.ListenAndServe(":"+cfg.HTTPPort, mux); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
