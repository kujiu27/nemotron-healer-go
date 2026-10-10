.PHONY: all build test bench check eval-mock demo clean cross

BINARY_NAME=nemotron-healer
VERSION=0.7.21

all: test build

build:
	@mkdir -p bin
	go build -ldflags="-s -w -X github.com/kujiu27/nemotron-healer-go/internal/cli.Version=v$(VERSION)" -o bin/$(BINARY_NAME) ./cmd/$(BINARY_NAME)

test:
	go test -v -race ./...

bench:
	bash scripts/bench.sh

# Offline end-to-end pipeline run against the LABELED mock
# (scripts/mock_nebius.py). Every response carries X-Nemotron-Healer: MOCK.
# Writes NO receipts — this is a wiring demo, not evidence.
eval-mock:
	@bash scripts/run_eval_mock.sh
demo: build
	@bash demo/run_pitch_demo.sh
cross:
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)_darwin_arm64 ./cmd/$(BINARY_NAME)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)_darwin_amd64 ./cmd/$(BINARY_NAME)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)_linux_amd64 ./cmd/$(BINARY_NAME)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/$(BINARY_NAME)_windows_amd64.exe ./cmd/$(BINARY_NAME)
	@ls -lh dist/

clean:
	rm -rf bin/ dist/

check:
	python3 scripts/check_links.py
