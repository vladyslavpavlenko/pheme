FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git
RUN go install github.com/bufbuild/buf/cmd/buf@latest

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN buf generate
RUN CGO_ENABLED=0 go build -o /pheme ./main

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /pheme /usr/local/bin/pheme
COPY config.example.yaml /etc/pheme/config.yaml

EXPOSE 7946/udp 7947 7948

ENTRYPOINT ["pheme"]
CMD ["-config", "/etc/pheme/config.yaml"]
