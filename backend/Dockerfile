FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags="-s -w" -o /app/server ./cmd/server && \
    go build -ldflags="-s -w" -o /app/dtu-simulator ./cmd/dtu-simulator && \
    go build -ldflags="-s -w" -o /app/plc-simulator ./cmd/plc-simulator

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata && \
    update-ca-certificates

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY --from=builder /app/server /app/server
COPY --from=builder /app/dtu-simulator /app/dtu-simulator
COPY --from=builder /app/plc-simulator /app/plc-simulator
COPY --from=builder /app/config.yaml /app/config.yaml

EXPOSE 8080 6060

ENTRYPOINT ["/app/server"]
