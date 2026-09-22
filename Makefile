BIN := tunnels-mcp

.PHONY: check build test clean release
check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt:"; gofmt -l .; exit 1; }
	go vet ./...
	go build ./...
	go test ./...

build:
	go build -o ./tmp/$(BIN) ./cmd/$(BIN)

test:
	go test ./... -count=1

release:
	@test -n "$(V)" || { echo "usage: make release V=0.0.2"; exit 2; }
	scripts/release.sh $(V)

clean:
	rm -rf ./tmp ./dist
