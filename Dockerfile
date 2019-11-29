FROM golang:buster AS builder
RUN update-ca-certificates
WORKDIR /
COPY . .
RUN go build -o /client ./cmd/client/main.go

# Image to use
FROM debian:buster-slim
COPY --from=builder /client /Meson-client
ADD ./client.toml /client.toml
 
ENTRYPOINT /Meson-client
