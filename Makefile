run:
	go mod tidy
	go fmt .
	if [[ -f .env ]]; then set -o allexport; source .env; fi; \
		go run .

# Checkout `-buildvcs` option to `go build`, which is enabled by default.
build: bin
	go build -v -o bin/gass

build-all: bin
	GOOS=darwin  GOARCH=amd64 go build -v -o bin/gass-darwin-amd64
	GOOS=linux   GOARCH=amd64 go build -v -o bin/gass-linux-amd64
	GOOS=windows GOARCH=amd64 go build -v -o bin/gass-windows-amd64.exe

bin:
	mkdir -pv bin

.PHONY: run build build-all
