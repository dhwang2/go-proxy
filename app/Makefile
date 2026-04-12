VERSION ?= dev
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o gproxy ./cmd/gproxy

release:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o gproxy ./cmd/gproxy

release-arm:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o gproxy-arm64 ./cmd/gproxy

test:
	OVERLAY=$$(./scripts/generate-test-overlay.sh); go test -overlay "$$OVERLAY" ./...

clean:
	rm -f gproxy gproxy-arm64 .test-overlay.json

.PHONY: build release release-arm test clean
