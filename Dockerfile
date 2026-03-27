FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/verificador-citas ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /out/verificador-citas /app/verificador-citas

RUN mkdir -p /app/data

ENV PORT=8080
ENV APP_DATA_DIR=/app/data

EXPOSE 8080

CMD ["/app/verificador-citas"]
