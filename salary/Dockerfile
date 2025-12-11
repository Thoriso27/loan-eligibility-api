
# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
# Copy entire repo because go.mod is at the root and the code imports from there
COPY ../go.mod ./go.mod
COPY .. .
# Build the salary service
RUN go build -o salary ./salary/main.go

# Run stage
FROM alpine:3.19
WORKDIR /app
COPY --from=build /app/salary ./
EXPOSE 8081
CMD ["./salary"]
