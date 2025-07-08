FROM arm64v8/golang:1.23-bullseye AS builder

WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o api main.go

FROM gcr.io/distroless/static-debian12:nonroot-arm64

USER nonroot

COPY --chown=nonroot:nonroot --from=builder /app/api /

ENTRYPOINT [ "/api" ]