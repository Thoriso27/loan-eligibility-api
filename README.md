
# Loan Eligibility API (Go)

Evaluates loan eligibility by calling two mock services:

- **Salary Verification API** (`POST /verify-salary`, :8081)
- **Credit Bureau API** (`POST /check-credit`, :8082)

### Decision Rules
- Monthly salary ≥ 3× monthly repayment
- Credit score ≥ 600
- No active defaults
- ≤ 3 active loans

Monthly repayment uses an **amortized** payment formula. Configure annual interest via `ANNUAL_INTEREST_PERCENT`.
 for rendering purposes F **Salary Verification** and **Credit Bureau** services are embedded as mock endpoints inside the main service

---

## Project Structure
# Loan Eligibility API

This project implements a **Loan Eligibility API** in Go. It allows checking loan eligibility based on salary and credit information. For the **Salary Verification** and **Credit Bureau** services are embedded as mock endpoints inside the main service, making it fully self contained and deployable.

---

## **Endpoints**

### 1. Apply for Loan
- **URL:** `/apply-loan`
- **Method:** `POST`
- **Request Body:**
```json
{
  "national_id": "12345678",
  "loan_amount": 50000,
  "term_months": 12
}

##** TEST DATA **
Salary Data
National ID	Monthly Salary
12345678	350,000
87654321	120,000
99999999	500,000
Credit Data
National ID	Credit Score	Active Defaults	Active Loans
12345678	650	0	2
87654321	540	0	1
99999999	720	1	4

## **Error Handling and Business Logic**

- The **Loan Eligibility API** evaluates loan applications based on:
  - Salary ≥ 3 × monthly repayment
  - Credit score ≥ 600
  - No active defaults
  - Maximum 3 active loans

- **Embedded Salary and Credit Mocks**:
  - For deployment and grading purposes, the salary (`/verify-salary`) and credit (`/check-credit`) services are included as internal mocks.
  - If a `national_id` is not found, the loan is declined with a clear reason.
  - Internal errors are handled gracefully with appropriate HTTP status codes.

- **Original Services**:
  - The `salary-api` and `credit-api` folders in the repository contain the **full original microservices**.
  - They include **full error handling** and **retry logic** for real external service calls.
  - These are included for reference and demonstrate how your microservices would work in a production environment.

- This setup ensures the **embedded API works fully self-contained** for grading, while the original services remain as a reference for proper production-grade error handling.

