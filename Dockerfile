FROM golang:1.23-bullseye AS builder

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o api main.go

FROM gcr.io/distroless/static-debian12:nonroot-amd64

USER nonroot

COPY --chown=nonroot:nonroot --from=builder /app/api /

ENTRYPOINT [ "/api" ]