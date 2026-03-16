VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o proxy ./cmd/proxy

release:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o proxy ./cmd/proxy

release-arm:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o proxy-arm64 ./cmd/proxy

test:
	go test ./...

clean:
	rm -f proxy proxy-arm64

.PHONY: build release release-arm test clean
