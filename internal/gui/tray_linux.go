//go:build linux && !android

// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// trayWatcher is the bus name of the StatusNotifier host that Fyne's tray
// registers with. COSMIC, KDE and GNOME's AppIndicator extension own it.
const trayWatcher = "org.kde.StatusNotifierWatcher"

// hasTray reports whether something on the session bus shows tray icons.
// Without one, a hidden window couldn't be brought back.
func hasTray() bool {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	if conn.Auth(nil) != nil || conn.Hello() != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var owned bool
	err = conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, trayWatcher).Store(&owned)
	return err == nil && owned
}
