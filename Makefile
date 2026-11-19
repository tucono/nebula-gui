# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.0.0-dev)
LDFLAGS    := -X main.version=$(VERSION)
DEV_SOCKET ?= $(XDG_RUNTIME_DIR)/nebula-guid.sock
DEV_STATE  ?= .dev-state

# Release builds (packages) take their version from the tag; see
# packaging/version.sh.
RELEASE_VERSION ?= $(shell packaging/version.sh)
PKG_VERSION     ?= $(shell packaging/version.sh --package)
RELEASE_LDFLAGS := -s -w -X main.version=$(RELEASE_VERSION)
# Windows builds keep their symbols: Defender's machine-learning scan flags
# stripped, unsigned Go programs.
WIN_LDFLAGS     := -X main.version=$(RELEASE_VERSION)
NFPM            ?= go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.49.0
ISCC            ?= C:/Program Files (x86)/Inno Setup 6/ISCC.exe
WINRES          ?= go run github.com/tc-hib/go-winres@v0.3.3

.PHONY: build service gui-bin dev-service gui test lint fmt tidy clean lab-up lab-down lab-import icons notices package-linux package-macos package-windows

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
	./bin/nebula-guid-dev --dev --listen "$(DEV_SOCKET)" --state-dir "$(DEV_STATE)"

## gui: run the GUI against the dev service
gui:
	NEBULA_GUI_SOCKET="$(DEV_SOCKET)" go run -ldflags "$(LDFLAGS)" ./cmd/nebula-gui

test:
	go test ./...

## lab-up / lab-down: start or stop the local Nebula test lab (docs/testing.md)
lab-up:
	testlab/lab.sh up

lab-down:
	testlab/lab.sh down

## lab-import: send the lab's dev config to the dev service
lab-import:
	testlab/lab.sh import "$(DEV_SOCKET)"

## icons: render the app icon's SVG to the packages' PNG, ICO and ICNS files in dist/icons
icons:
	go run ./packaging/icons -src internal/gui/assets -out dist/icons

## notices: write dist/THIRD_PARTY_NOTICES.txt, the licenses of what's built in (for this GOOS)
notices:
	go run ./packaging/notices -out dist/THIRD_PARTY_NOTICES.txt

## package-linux: build the .deb and .rpm into dist/ (amd64; run on Linux)
package-linux: icons notices
	CGO_ENABLED=0 go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o dist/linux/nebula-guid ./cmd/nebula-guid
	go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o dist/linux/nebula-gui ./cmd/nebula-gui
	PKG_VERSION="$(PKG_VERSION)" $(NFPM) package --config packaging/linux/nfpm.yaml --packager deb --target dist/
	PKG_VERSION="$(PKG_VERSION)" $(NFPM) package --config packaging/linux/nfpm.yaml --packager rpm --target dist/

## package-macos: build the universal .pkg into dist/ (run on a Mac with the Xcode command line tools)
package-macos: icons notices
	for arch in amd64 arm64; do \
		CGO_ENABLED=0 GOARCH=$$arch go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o dist/macos/$$arch/nebula-guid ./cmd/nebula-guid && \
		CGO_ENABLED=1 GOARCH=$$arch MACOSX_DEPLOYMENT_TARGET=11.0 go build -trimpath -ldflags "$(RELEASE_LDFLAGS)" -o dist/macos/$$arch/nebula-gui ./cmd/nebula-gui || exit 1; \
	done
	packaging/macos/build-pkg.sh "$(RELEASE_VERSION)"

## package-windows: build the setup .exe into dist/ (amd64; run on Windows with Inno Setup 6, from bash)
package-windows: icons notices
	powershell -NoProfile -ExecutionPolicy Bypass -File packaging/windows/fetch-wintun.ps1 -OutDir dist/windows
	CGO_ENABLED=0 go build -trimpath -ldflags "$(WIN_LDFLAGS)" -o dist/windows/nebula-guid.exe ./cmd/nebula-guid
	go build -trimpath -ldflags "$(WIN_LDFLAGS) -H=windowsgui" -o dist/windows/nebula-gui.exe ./cmd/nebula-gui
	# Version details, icon and manifest, from packaging/windows/*.winres.json.
	for exe in nebula-gui nebula-guid; do \
		$(WINRES) patch --no-backup --in packaging/windows/$$exe.winres.json \
			--file-version "$(RELEASE_VERSION)" --product-version "$(RELEASE_VERSION)" \
			dist/windows/$$exe.exe || exit 1; \
	done
	MSYS2_ARG_CONV_EXCL="*" "$(ISCC)" /Q /DAppVersion=$(RELEASE_VERSION) /DNumVersion=$(firstword $(subst -, ,$(RELEASE_VERSION))) packaging/windows/nebula-gui.iss

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

clean:
	rm -rf bin/ dist/
