package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type CreditRequest struct {
	NationalID string `json:"national_id"`
}

type CreditResponse struct {
	NationalID     string `json:"national_id"`
	CreditScore    int    `json:"credit_score"`
	ActiveDefaults int    `json:"active_defaults"`
	ActiveLoans    int    `json:"active_loans"`
}

var creditData = map[string]CreditResponse{
	"12345678": {
		NationalID:     "12345678",
		CreditScore:    650,
		ActiveDefaults: 0,
		ActiveLoans:    2,
	},
	"87654321": {
		NationalID:     "87654321",
		CreditScore:    540,
		ActiveDefaults: 0,
		ActiveLoans:    1,
	},
	"99999999": {
		NationalID:     "99999999",
		CreditScore:    720,
		ActiveDefaults: 1,
		ActiveLoans:    4,
	},
}

func checkCreditHandler(w http.ResponseWriter, r *http.Request) {
	reqID := r.Header.Get("X-Request-ID")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method_not_allowed"})
		return
	}

	var req CreditRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NationalID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_body",
			"message": "expected {\"national_id\":\"...\"}",
		})
		return
	}

	record, exists := creditData[req.NationalID]
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "not_found",
			"message": "credit record not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(record)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/check-credit", checkCreditHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         ":8082",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("Credit Bureau API running on :8082")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	log.Println("Credit service exited")
}
