package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// ---------- Data Types ----------
type LoanRequest struct {
	NationalID string  `json:"national_id"`
	LoanAmount float64 `json:"loan_amount"`
	TermMonths int     `json:"term_months"`
}

type LoanResponse struct {
	Status          string          `json:"status"` // APPROVED or DECLINED
	Reason          string          `json:"reason,omitempty"`
	Reasons         []string        `json:"reasons,omitempty"`
	MonthlyPayment  float64         `json:"monthly_payment,omitempty"`
	AnnualInterest  float64         `json:"annual_interest_percent,omitempty"`
	SalaryEcho      *SalaryResponse `json:"salary,omitempty"`
	CreditEcho      *CreditResponse `json:"credit,omitempty"`
	ApplicationEcho *LoanRequest    `json:"application,omitempty"`
}

type SalaryResponse struct {
	NationalID    string  `json:"national_id"`
	MonthlySalary float64 `json:"monthly_salary"`
}

type CreditResponse struct {
	NationalID     string `json:"national_id"`
	CreditScore    int    `json:"credit_score"`
	ActiveDefaults int    `json:"active_defaults"`
	ActiveLoans    int    `json:"active_loans"`
}

// ---------- Mock Salary Data ----------
var salaryData = map[string]float64{
	"12345678": 350000,
	"87654321": 120000,
	"99999999": 500000,
}

// ---------- Mock Credit Data ----------
var creditData = map[string]CreditResponse{
	"12345678": {NationalID: "12345678", CreditScore: 650, ActiveDefaults: 0, ActiveLoans: 2},
	"87654321": {NationalID: "87654321", CreditScore: 540, ActiveDefaults: 0, ActiveLoans: 1},
	"99999999": {NationalID: "99999999", CreditScore: 720, ActiveDefaults: 1, ActiveLoans: 4},
}

// ---------- Helpers ----------
func amortizedMonthlyPayment(loanAmount float64, termMonths int, annualInterestPercent float64) float64 {
	if termMonths <= 0 || loanAmount <= 0 {
		return 0
	}
	monthlyRate := (annualInterestPercent / 100.0) / 12.0
	if monthlyRate == 0 {
		return math.Round((loanAmount/float64(termMonths))*100) / 100
	}
	power := math.Pow(1+monthlyRate, float64(termMonths))
	payment := loanAmount * (monthlyRate * power) / (power - 1)
	return math.Round(payment*100) / 100
}

// ---------- Mock Salary API ----------
func verifySalaryHandler(w http.ResponseWriter, r *http.Request) {
	reqID := r.Header.Get("X-Request-ID")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NationalID string `json:"national_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NationalID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	monthly, ok := salaryData[req.NationalID]
	if !ok {
		http.Error(w, "Salary record not found", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(SalaryResponse{
		NationalID:    req.NationalID,
		MonthlySalary: monthly,
	})
}

// ---------- Mock Credit API ----------
func checkCreditHandler(w http.ResponseWriter, r *http.Request) {
	reqID := r.Header.Get("X-Request-ID")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NationalID string `json:"national_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NationalID == "" {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	credit, ok := creditData[req.NationalID]
	if !ok {
		http.Error(w, "Credit record not found", http.StatusNotFound)
		return
	}

	_ = json.NewEncoder(w).Encode(credit)
}

// ---------- Loan Eligibility Handler ----------
func loanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	reqID := strconv.FormatInt(time.Now().UnixNano(), 36)
	w.Header().Set("X-Request-ID", reqID)

	var request LoanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.NationalID == "" || request.LoanAmount <= 0 || request.TermMonths <= 0 {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	annualRate := 20.0

	// Call mock Salary API
	salaryBody, _ := json.Marshal(map[string]string{"national_id": request.NationalID})
	salaryReq, _ := http.NewRequest(http.MethodPost, "http://localhost:8080/verify-salary", bytes.NewBuffer(salaryBody))
	salaryReq.Header.Set("Content-Type", "application/json")
	salaryReq.Header.Set("X-Request-ID", reqID)
	salaryResp, err := http.DefaultClient.Do(salaryReq)
	var salary SalaryResponse
	if err != nil || salaryResp.StatusCode != http.StatusOK {
		if salaryResp != nil && salaryResp.StatusCode == http.StatusNotFound {
			monthly := amortizedMonthlyPayment(request.LoanAmount, request.TermMonths, annualRate)
			_ = json.NewEncoder(w).Encode(LoanResponse{
				Status:          "DECLINED",
				Reason:          "Salary record not found",
				Reasons:         []string{"Salary record not found"},
				MonthlyPayment:  monthly,
				AnnualInterest:  annualRate,
				SalaryEcho:      nil,
				CreditEcho:      nil,
				ApplicationEcho: &request,
			})
			return
		}
		http.Error(w, "Failed to verify salary", http.StatusBadGateway)
		return
	}
	_ = json.NewDecoder(salaryResp.Body).Decode(&salary)
	defer salaryResp.Body.Close()

	// Call mock Credit API
	creditBody, _ := json.Marshal(map[string]string{"national_id": request.NationalID})
	creditReq, _ := http.NewRequest(http.MethodPost, "http://localhost:8080/check-credit", bytes.NewBuffer(creditBody))
	creditReq.Header.Set("Content-Type", "application/json")
	creditReq.Header.Set("X-Request-ID", reqID)
	creditResp, err := http.DefaultClient.Do(creditReq)
	var credit CreditResponse
	if err != nil || creditResp.StatusCode != http.StatusOK {
		if creditResp != nil && creditResp.StatusCode == http.StatusNotFound {
			monthly := amortizedMonthlyPayment(request.LoanAmount, request.TermMonths, annualRate)
			_ = json.NewEncoder(w).Encode(LoanResponse{
				Status:          "DECLINED",
				Reason:          "Credit record not found",
				Reasons:         []string{"Credit record not found"},
				MonthlyPayment:  monthly,
				AnnualInterest:  annualRate,
				SalaryEcho:      &salary,
				CreditEcho:      nil,
				ApplicationEcho: &request,
			})
			return
		}
		http.Error(w, "Failed to verify credit", http.StatusBadGateway)
		return
	}
	_ = json.NewDecoder(creditResp.Body).Decode(&credit)
	defer creditResp.Body.Close()

	// Decision logic
	monthly := amortizedMonthlyPayment(request.LoanAmount, request.TermMonths, annualRate)
	reasons := []string{}
	if salary.MonthlySalary < 3*monthly {
		reasons = append(reasons, "Monthly salary is less than 3x the amortized monthly repayment")
	}
	if credit.CreditScore < 600 {
		reasons = append(reasons, "Credit score below 600")
	}
	if credit.ActiveDefaults > 0 {
		reasons = append(reasons, "Active defaults present")
	}
	if credit.ActiveLoans > 3 {
		reasons = append(reasons, "More than 3 active loans")
	}

	if len(reasons) > 0 {
		_ = json.NewEncoder(w).Encode(LoanResponse{
			Status:          "DECLINED",
			Reason:          reasons[0],
			Reasons:         reasons,
			MonthlyPayment:  monthly,
			AnnualInterest:  annualRate,
			SalaryEcho:      &salary,
			CreditEcho:      &credit,
			ApplicationEcho: &request,
		})
		return
	}

	_ = json.NewEncoder(w).Encode(LoanResponse{
		Status:          "APPROVED",
		MonthlyPayment:  monthly,
		AnnualInterest:  annualRate,
		SalaryEcho:      &salary,
		CreditEcho:      &credit,
		ApplicationEcho: &request,
	})
}

// ---------- Main ----------
func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/apply-loan", loanHandler)
	mux.HandleFunc("/verify-salary", verifySalaryHandler)
	mux.HandleFunc("/check-credit", checkCreditHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Println("Loan Eligibility API (with Salary & Credit mocks) running on :8080")
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
	log.Println("Service exited")
}
