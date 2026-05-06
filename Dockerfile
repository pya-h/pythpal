FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /priceservice ./cmd/server/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /priceservice /priceservice

EXPOSE 3000
ENTRYPOINT ["/priceservice"]
