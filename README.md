# Meson-client
A simple client for use with the Meson mixnet software


## Tests

Since this library requires to connect to an existing katzenpost mixnet one needs to run the tests inside of a docker container and connect to the mixnet docker network. You can run a mixnet by following the instructions at [https://github.com/hashcloak/Meson](https://github.com/hashcloak/Meson)

```
docker run --rm \
  -v `pwd`:/client \
  --network nonvoting_testnet_nonvoting_test_net \
  -v /tmp/gopath-pkg:/go/pkg \
  -w /client \
  golang:buster \
  /bin/bash -c "GORACE=history_size=7 go test -race" # Test command
```

The above can be de-constructed as following:
- ```-v `pwd`:/client```: Mount the current directory inside the docker container at `/client`
- `--network nonvoting_testnet_nonvoting_test_net`: Connect to the existing docker network mixnet
- `-v /tmp/gopath-pkg:/go/pkg`: Cache the go modules that belong to this container in `/tmp/gopath-pkg`
- `-w /client`: Working directory for the docker image
- `golang:buster`: The docker image to be used
-  `/bin/bash -c "GORACE=history_size=7 go test -race"`: The command to run inside the container

## Debugging

First, build the debugger image and install [delve](https://github.com/go-delve/delve) on your local path:

```
# Install delve on your local $GOPATH
go get github.com/derekparker/delve/cmd/dlv

# Build the delve docker container
docker build -f ./ops/dlv.Dockerfile -t hashcloak/dlv .
```

### Debugging the tests

The `hashcloak/dlv` container runs `delve` in test mode, which is different to the `dlv debug` mode, inside the docker container:

```
docker run --rm \
  -v `pwd`:/app \
  -v /tmp/gopath-pkg:/go/pkg \
  --network nonvoting_testnet_nonvoting_test_net \
  --security-opt=seccomp:unconfined \
  -w /app \
  --publish 2345:2345 \
  hashcloak/dlv
```

Attach to the `delve` server by running the command:

```
dlv connect localhost:2345
```

To learn how to use `delve` [here is an introduction](https://www.melvinvivas.com/debugging-go-applications-using-delve/).
