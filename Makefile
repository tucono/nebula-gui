# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
LDFLAGS    := -X main.version=$(VERSION)
DEV_SOCKET ?= $(XDG_RUNTIME_DIR)/nebula-guid.sock

.PHONY: build service gui-bin dev-service gui test lint fmt tidy clean

## build: build both binaries into bin/
build:
	go build -ldflags "$(LDFLAGS)" -o bin/ ./cmd/...

service:
	go build -ldflags "$(LDFLAGS)" -o bin/ ./cmd/nebula-guid

gui-bin:
	go build -ldflags "$(LDFLAGS)" -o bin/ ./cmd/nebula-gui

## dev-service: build the service with -tags dev, give it CAP_NET_ADMIN (Linux), run it
dev-service:
	go build -tags dev -ldflags "$(LDFLAGS)" -o bin/nebula-guid-dev ./cmd/nebula-guid
	sudo setcap cap_net_admin,cap_net_raw+ep bin/nebula-guid-dev
	./bin/nebula-guid-dev --dev --listen "$(DEV_SOCKET)"

## gui: run the GUI against the dev service
gui:
	NEBULA_GUI_SOCKET="$(DEV_SOCKET)" go run -ldflags "$(LDFLAGS)" ./cmd/nebula-gui

test:
	go test ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

clean:
	rm -rf bin/ dist/
