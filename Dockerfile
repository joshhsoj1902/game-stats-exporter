FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o game-stats-exporter .

FROM alpine:3.20

RUN apk update && apk add ca-certificates

COPY --from=builder /app/game-stats-exporter /game-stats-exporter

ENV STEAM_KEY=""
ENV REDIS_ADDR="localhost:6379"
ENV REDIS_PASSWORD=""
ENV REDIS_DB="0"
ENV POLL_INTERVAL_NORMAL="15m"
ENV POLL_INTERVAL_ACTIVE="5m"
ENV PORT="8000"

EXPOSE 8000

ENTRYPOINT ["/game-stats-exporter"]

