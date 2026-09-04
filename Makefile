BIN := tunnels-mcp

.PHONY: check build test clean
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt:"; gofmt -l .; exit 1; }
	go vet ./...
	go build ./...
	go test ./...

build:
	go build -o ./tmp/$(BIN) ./cmd/$(BIN)

test:
	go test ./... -count=1

clean:
	rm -rf ./tmp
