LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$$(git rev-parse HEAD || echo) -X main.Date=$$(date -u +%Y-%m-%dT%H:%M:%SZ)"

run:
	go mod tidy
	go fmt .
	if [[ -f .env ]]; then set -o allexport; source .env; fi; \
		go run $(LDFLAGS) . --file secrets-bot.yml

# Checkout `-buildvcs` option to `go build`, which is enabled by default.
build: bin
	go build $(LDFLAGS) -v -o bin/gass

build-all: bin
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -v -o bin/gass-linux-amd64
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -v -o bin/gass-darwin-amd64
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -v -o bin/gass-windows-amd64.exe

bin:
	mkdir -pv bin

.PHONY: run build build-all
