
# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
# Copy module files (improves build cache if you later add go.sum)
COPY go.mod ./
COPY . .
# Build the eligibility API (root main.go)
RUN go build -o eligibility ./main.go

# Run stage (small image)
FROM alpine:3.19
WORKDIR /app
COPY --from=build /app/eligibility ./
EXPOSE 8080
CMD ["./eligibility"]

