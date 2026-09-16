BIN := bin/ag-mux

.PHONY: build test fmt clean demo demo-clean

build:
	go build -o $(BIN) ./cmd/ag-mux

test:
	go test ./...

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf bin

demo: build
	@./scripts/demo.sh

demo-clean:
	@tmux kill-session -t ag-mux-demo 2>/dev/null || true
