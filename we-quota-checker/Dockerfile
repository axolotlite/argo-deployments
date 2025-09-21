FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum main.go ./
RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /we-quota-checker .

FROM gcr.io/distroless/static-debian11

WORKDIR /

COPY --from=builder /we-quota-checker /we-quota-checker

EXPOSE 2222

ENTRYPOINT ["/we-quota-checker"]