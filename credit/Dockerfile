
# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY ../go.mod ./go.mod
COPY .. .
RUN go build -o credit ./credit/main.go

# Run stage
FROM alpine:3.19
WORKDIR /app
COPY --from=build /app/credit ./
EXPOSE 8082
CMD ["./credit"]
