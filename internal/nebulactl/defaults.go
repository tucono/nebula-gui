// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package nebulactl

import (
	"runtime"

	"github.com/slackhq/nebula/config"
)

// windowsTunDev is the adapter name used on Windows when the config gives
// none. Nebula 1.12 leaves tun.dev empty there, and Wintun refuses an empty
// name ("The parameter is incorrect"). Linux and macOS pick a name
// themselves.
const windowsTunDev = "nebula1"

// defaultTunDev is the tun.dev to fill in on goos, or "" for none.
func defaultTunDev(goos string) string {
	if goos == "windows" {
		return windowsTunDev
	}
	return ""
}

// setDefaults fills in settings Nebula needs on this platform but doesn't
// default itself.
func setDefaults(c *config.C) {
	setTunDev(c, defaultTunDev(runtime.GOOS))
}

// setTunDev sets tun.dev to dev when the config leaves it empty and the tun
// device is enabled. A tun section that isn't a mapping is left for Nebula
// to reject.
func setTunDev(c *config.C, dev string) {
	if dev == "" || c.GetBool("tun.disabled", false) || c.GetString("tun.dev", "") != "" {
		return
	}
	switch tun := c.Settings["tun"].(type) {
	case map[string]any:
		tun["dev"] = dev
	case nil:
		c.Settings["tun"] = map[string]any{"dev": dev}
	}
}
