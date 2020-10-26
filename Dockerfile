# build stage
FROM golang:1.15 as builder
ENV GO111MODULE=on

WORKDIR /app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o domainik-binary

FROM alpine:3.10 as certs
RUN apk add ca-certificates && update-ca-certificates

# final stage
FROM scratch
COPY --from=builder /app/domainik-binary /app/
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ENTRYPOINT ["/app/domainik-binary"]