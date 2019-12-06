FROM golang:buster
RUN go get github.com/derekparker/delve/cmd/dlv

CMD /go/bin/dlv \
  test \
  --headless \
  --listen=:2345 \
  --api-version=2
