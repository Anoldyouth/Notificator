FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/notificator ./cmd/notificator

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app && apk add --no-cache ca-certificates && mkdir -p /data && chown -R app:app /data

WORKDIR /app

COPY --from=builder /out/notificator /app/notificator

USER app

ENTRYPOINT ["/app/notificator"]
