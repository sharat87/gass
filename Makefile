LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$$(git rev-parse HEAD || echo) -X main.Date=$$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Checkout `-buildvcs` option to `go build`, which is enabled by default.
build: bin
	go build $(LDFLAGS) -v -o bin/gass

build-all: bin
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -v -o bin/gass-linux-amd64
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -v -o bin/gass-darwin-amd64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -v -o bin/gass-windows-amd64.exe

run: fmt
	if [[ -f .env ]]; then set -o allexport; source .env; fi; \
		go run $(LDFLAGS) . --file secrets.yml

test: fmt
	go vet ./...
	go test ./...

fmt:
	go mod tidy
	go fmt .

bin:
	mkdir -pv bin

.PHONY: run test fmt build build-all
