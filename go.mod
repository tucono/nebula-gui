module github.com/tucono/nebula-gui

go 1.27.3

toolchain go1.27.4

require (
	fyne.io/fyne/v2 v2.8.2
	github.com/Microsoft/go-winio v0.6.4
	github.com/godbus/dbus/v5 v5.2.2
	github.com/slackhq/nebula v1.12.0
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0
)

require (
	dario.cat/mergo v1.0.2 // indirect
	filippo.io/bigmod v0.1.0 // indirect
	fyne.io/systray v1.12.3-0.20260810170012-af4e8e793ec4 // indirect
	github.com/BurntSushi/toml v1.6.0 // indirect
	github.com/DefinedNet/dnsconfig v0.0.0-20261028201825-617c971e7a6a // indirect
	github.com/DefinedNet/netlink v1.3.2-dn.1 // indirect
	github.com/FyshOS/fancyfs v0.0.1 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/anthonynsimon/bild v0.14.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.2.0 // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fyne-io/gl-js v0.2.1-0.20260315212741-029c47fd27e8 // indirect
	github.com/fyne-io/glfw-js v0.4.0 // indirect
	github.com/fyne-io/image v0.1.1 // indirect
	github.com/fyne-io/oksvg v0.2.1-0.20260918172519-a9af55fa95d9 // indirect
	github.com/gaissmai/bart v0.30.0 // indirect
	github.com/go-gl/gl v0.0.0-20260331235117-4566fea9a276 // indirect
	github.com/go-gl/glfw/v3.4/glfw v0.1.0-pre.2 // indirect
	github.com/go-text/render v0.2.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/hack-pad/go-indexeddb v0.3.2 // indirect
	github.com/hack-pad/safejs v0.1.0 // indirect
	github.com/jeandeaual/go-locale v0.0.0-20250612000132-0ef82f21eade // indirect
	github.com/jsummers/gobmp v0.0.0-20230614200233-a9de23ed2e25 // indirect
	github.com/mattn/go-runewidth v0.0.24 // indirect
	github.com/miekg/dns v1.1.73 // indirect
	github.com/miekg/pkcs11 v1.1.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nbrownus/go-metrics-prometheus v0.0.0-20210712211119-974a6260965f // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/nicksnyder/go-i18n/v2 v2.5.1 // indirect
	github.com/prometheus/client_golang v1.25.0 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.72.0 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20201227073835-cf1acfcdf475 // indirect
	github.com/rymdport/portal v0.4.2 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/stefanberger/go-pkcs11uri v0.0.0-20230803200340-78284954bff6 // indirect
	github.com/stretchr/testify v1.12.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/yuin/goldmark v1.8.2 // indirect
	golang.org/x/image v0.24.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/term v0.47.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2 // indirect
	golang.zx2c4.com/wireguard v0.0.0-20230325221338-052af4a8072b // indirect
	golang.zx2c4.com/wireguard/windows v1.1.1 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gvisor.dev/gvisor v0.0.0-20261014193016-04d31b385980 // indirect
)

tool (
	github.com/slackhq/nebula/cmd/nebula
	github.com/slackhq/nebula/cmd/nebula-cert
)
