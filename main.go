package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

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

var httpClient = &http.Client{Timeout: 5 * time.Second}

// ---- helpers: finance, retry, request id ----

func amortizedMonthlyPayment(loanAmount float64, termMonths int, annualInterestPercent float64) float64 {
	if termMonths <= 0 || loanAmount <= 0 {
		return 0
	}
	monthlyRate := (annualInterestPercent / 100.0) / 12.0
	if monthlyRate == 0 {
		return math.Round((loanAmount/float64(termMonths))*100) / 100
	}
	r := monthlyRate
	n := float64(termMonths)
	p := loanAmount
	power := math.Pow(1+r, n)
	payment := p * (r * power) / (power - 1)
	return math.Round(payment*100) / 100
}

// Non-generic retry helpers for older Go versions
func withRetrySalary(attempts int, sleep time.Duration, fn func() (SalaryResponse, error)) (SalaryResponse, error) {
	var zero SalaryResponse
	var err error
	for i := 0; i < attempts; i++ {
		var v SalaryResponse
		v, err = fn()
		if err == nil {
			return v, nil
		}
		time.Sleep(sleep)
	}
	return zero, err
}

func withRetryCredit(attempts int, sleep time.Duration, fn func() (CreditResponse, error)) (CreditResponse, error) {
	var zero CreditResponse
	var err error
	for i := 0; i < attempts; i++ {
		var v CreditResponse
		v, err = fn()
		if err == nil {
			return v, nil
		}
		time.Sleep(sleep)
	}
	return zero, err
}

func getOrCreateReqID(r *http.Request) string {
	id := r.Header.Get("X-Request-ID")
	if id == "" {
		id = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return id
}

type httpError struct {
	StatusCode int
	Body       string
}

func (e *httpError) Error() string {
	return "http error: " + strconv.Itoa(e.StatusCode) + " - " + e.Body
}

// ---- external calls ----

func callSalaryAPI(baseURL, nationalID, reqID string) (SalaryResponse, error) {
	body, _ := json.Marshal(map[string]string{"national_id": nationalID})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/verify-salary", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", reqID)
	res, err := httpClient.Do(req)
	if err != nil {
		return SalaryResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return SalaryResponse{}, &httpError{StatusCode: res.StatusCode, Body: string(b)}
	}
	var sr SalaryResponse
	if err := json.NewDecoder(res.Body).Decode(&sr); err != nil {
		return SalaryResponse{}, err
	}
	return sr, nil
}

func callCreditAPI(baseURL, nationalID, reqID string) (CreditResponse, error) {
	body, _ := json.Marshal(map[string]string{"national_id": nationalID})
	req, _ := http.NewRequest(http.MethodPost, baseURL+"/check-credit", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", reqID)
	res, err := httpClient.Do(req)
	if err != nil {
		return CreditResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return CreditResponse{}, &httpError{StatusCode: res.StatusCode, Body: string(b)}
	}
	var cr CreditResponse
	if err := json.NewDecoder(res.Body).Decode(&cr); err != nil {
		return CreditResponse{}, err
	}
	return cr, nil
}

// ---- HTTP handler ----

func loanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST requests allowed", http.StatusMethodNotAllowed)
		return
	}

	reqID := getOrCreateReqID(r)
	w.Header().Set("X-Request-ID", reqID)

	var request LoanRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if request.NationalID == "" || request.LoanAmount <= 0 || request.TermMonths <= 0 {
		http.Error(w, "Missing or invalid fields", http.StatusBadRequest)
		return
	}

	salaryURL := os.Getenv("SALARY_API_URL")
	creditURL := os.Getenv("CREDIT_API_URL")
	annualRate := 20.0 // default
	if v := os.Getenv("ANNUAL_INTEREST_PERCENT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			annualRate = f
		}
	}
	if salaryURL == "" || creditURL == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":      "config_error",
			"message":    "Service URLs not configured",
			"request_id": reqID,
		})
		return
	}

	// Salary call with retrySalary
	log.Printf("req_id=%s calling salary %s national_id=%s", reqID, salaryURL, request.NationalID)
	salary, err := withRetrySalary(3, 250*time.Millisecond, func() (SalaryResponse, error) {
		return callSalaryAPI(salaryURL, request.NationalID, reqID)
	})

	if err != nil {
		// If the error is a 404 from salary, treat as a business decline (domain choice).
		if httpErr, ok := err.(*httpError); ok && httpErr.StatusCode == http.StatusNotFound {
			monthly := amortizedMonthlyPayment(request.LoanAmount, request.TermMonths, annualRate)
			reasons := []string{"Salary record not found"}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(LoanResponse{
				Status:          "DECLINED",
				Reason:          reasons[0],
				Reasons:         reasons,
				MonthlyPayment:  monthly,
				AnnualInterest:  annualRate,
				SalaryEcho:      nil, // unknown
				CreditEcho:      nil,
				ApplicationEcho: &request,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":      "salary_service_unavailable",
			"message":    "Failed to verify salary",
			"detail":     err.Error(),
			"request_id": reqID,
		})
		return
	}

	// Credit call with retry

	log.Printf("req_id=%s calling credit %s national_id=%s", reqID, creditURL, request.NationalID)
	credit, err := withRetryCredit(3, 250*time.Millisecond, func() (CreditResponse, error) {
		return callCreditAPI(creditURL, request.NationalID, reqID)
	})

	if err != nil {
		// Treat credit 404 as business decline (no bureau record)
		if httpErr, ok := err.(*httpError); ok && httpErr.StatusCode == http.StatusNotFound {
			monthly := amortizedMonthlyPayment(request.LoanAmount, request.TermMonths, annualRate)
			reasons := []string{"Credit record not found"}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(LoanResponse{
				Status:          "DECLINED",
				Reason:          reasons[0],
				Reasons:         reasons,
				MonthlyPayment:  monthly,
				AnnualInterest:  annualRate,
				SalaryEcho:      &salary, // salary known
				CreditEcho:      nil,
				ApplicationEcho: &request,
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":      "credit_service_unavailable",
			"message":    "Failed to verify credit",
			"detail":     err.Error(),
			"request_id": reqID,
		})
		return
	}

	// Decision rules
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

	w.Header().Set("Content-Type", "application/json")
	if len(reasons) > 0 {
		log.Printf("req_id=%s declined id=%s reasons=%v monthly=%v", reqID, request.NationalID, reasons, monthly)
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

	log.Printf("req_id=%s approved id=%s monthly=%v", reqID, request.NationalID, monthly)
	_ = json.NewEncoder(w).Encode(LoanResponse{
		Status:          "APPROVED",
		MonthlyPayment:  monthly,
		AnnualInterest:  annualRate,
		SalaryEcho:      &salary,
		CreditEcho:      &credit,
		ApplicationEcho: &request,
	})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/apply-loan", loanHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Run server
	go func() {
		log.Println("Eligibility API running on :8080")
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
	log.Println("Eligibility service exited")
}
