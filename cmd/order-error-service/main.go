package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	commerceerrors "github.com/example/commerce-error-groups"
)

type captureRequest struct {
	EventID string                 `json:"event_id"`
	Failure commerceerrors.Failure `json:"failure"`
}

func main() {
	client := commerceerrors.NewClient(os.Getenv("INFRAI_API_KEY"))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /order-errors", func(w http.ResponseWriter, r *http.Request) {
		var input captureRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		result, err := client.Capture(r.Context(), input.Failure, input.EventID)
		if err != nil {
			http.Error(w, "capture failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"captured": true, "data": result.Data, "metadata": result.Metadata})
	})

	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("order error service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
