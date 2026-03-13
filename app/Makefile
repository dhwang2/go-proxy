GO ?= go
VERSION ?= v0.0.0-dev

.PHONY: build release release-arm test lint clean

build:
	$(GO) build -ldflags="-X main.buildVersion=$(VERSION)" -o proxy ./cmd/proxy

release:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -ldflags="-s -w -X main.buildVersion=$(VERSION)" -o proxy ./cmd/proxy

release-arm:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -ldflags="-s -w -X main.buildVersion=$(VERSION)" -o proxy-arm64 ./cmd/proxy

test:
	$(GO) test ./...

lint:
	golangci-lint run

clean:
	rm -f proxy proxy-arm64
