
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

---

## Project Structure
