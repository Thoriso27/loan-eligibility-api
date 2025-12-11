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

type SalaryRequest struct {
	NationalID string `json:"national_id"`
}

type SalaryResponse struct {
	NationalID    string  `json:"national_id"`
	MonthlySalary float64 `json:"monthly_salary"`
}

var salaryData = map[string]float64{
	"12345678": 350000,
	"87654321": 120000,
	"99999999": 500000,
}

func verifySalaryHandler(w http.ResponseWriter, r *http.Request) {
	// Echo request ID for traceability
	reqID := r.Header.Get("X-Request-ID")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "method_not_allowed",
		})
		return
	}

	var req SalaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NationalID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "invalid_body",
			"message": "expected {\"national_id\":\"...\"}",
		})
		return
	}

	monthly, exists := salaryData[req.NationalID]
	if !exists {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "not_found",
			"message": "salary record not found",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SalaryResponse{
		NationalID:    req.NationalID,
		MonthlySalary: monthly,
	})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/verify-salary", verifySalaryHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         ":8081",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Run server
	go func() {
		log.Println("Salary Verification API running on :8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
	log.Println("Salary service exited")
}
