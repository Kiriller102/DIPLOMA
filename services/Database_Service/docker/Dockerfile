FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY services/Database_Service services/Database_Service
RUN CGO_ENABLED=0 go build -trimpath -o /database-service ./services/Database_Service/cmd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 messenger
WORKDIR /app
COPY --from=builder /database-service /app/database-service
USER messenger
EXPOSE 8080 5050
ENTRYPOINT ["/app/database-service"]
