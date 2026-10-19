FROM golang:1.23 AS builder

WORKDIR /database_service_app

# Копируем go.mod и go.sum из корня проекта
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код Database_Service
COPY . .

# Сборка бинарного файла
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64
RUN go build -o database_service ./cmd/main.go

FROM alpine:latest

# Устанавливаем bash и openjdk11
RUN apk add --no-cache bash openjdk11

WORKDIR /database_service_app

# Копируем бинарный файл из builder-слоя
COPY --from=builder /database_service_app/database_service /database_service_app/database_service

# Копируем конфигурационные и миграционные файлы
COPY configs /database_service_app/configs
COPY migrations /database_service_app/migrations
COPY logs /database_service_app/logs

# Копируем secrets.env
COPY configs/secrets.env /database_service_app/configs/secrets.env

# Открываем порт 8080
EXPOSE 8080

# Устанавливаем переменные окружения
ENV CONFIG_PATH=/app/configs/config.yaml
ENV DB_CONFIG_PATH=/app/configs/db.yaml
ENV SECRETS_PATH=/app/secrets.env

#RUN ls -l /database_service_app/
#RUN chmod +x /database_service_app/database_service

CMD ["./database_service"]
