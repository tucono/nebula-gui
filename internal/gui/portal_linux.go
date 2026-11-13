// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/godbus/dbus/v5"
)

// platformOpen is the XDG Desktop Portal's file chooser: the desktop's own
// picker (GNOME, KDE, COSMIC, …), reached over the session bus. It works
// for apps outside a sandbox too, and then returns the real path of the
// file, so relative pki paths next to the config still resolve.
var platformOpen nativeOpenFunc = portalOpenFile

const (
	portalDest     = "org.freedesktop.portal.Desktop"
	portalPath     = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalOpenCall = "org.freedesktop.portal.FileChooser.OpenFile"
	portalRequest  = "org.freedesktop.portal.Request"
	portalResponse = portalRequest + ".Response"
)

// portalFilter is the portal's filter type, a(us) rules under a name:
// (sa(us)). godbus sends a struct as a D-Bus struct.
type portalFilter struct {
	Name  string
	Rules []portalRule
}

type portalRule struct {
	Type    uint32 // 0: glob pattern, 1: MIME type
	Pattern string
}

// portalOpenFile shows the portal's file chooser and waits for the answer.
//
// It uses its own connection, so its signal subscription can't interfere
// with anything else on the session bus, and it subscribes to the
// request's Response signal before calling OpenFile, using the request
// path the portal will use for its handle_token. Otherwise a quick answer
// could be missed.
func portalOpenFile(parent string, req openRequest) (string, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return "", &errNoNativePicker{Err: err}
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Auth(nil); err != nil {
		return "", &errNoNativePicker{Err: err}
	}
	if err := conn.Hello(); err != nil {
		return "", &errNoNativePicker{Err: err}
	}

	token, err := handleToken()
	if err != nil {
		return "", err
	}
	want := requestPath(conn.Names()[0], token)
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface(portalRequest),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return "", &errNoNativePicker{Err: err}
	}

	var handle dbus.ObjectPath
	call := conn.Object(portalDest, portalPath).Call(portalOpenCall, 0, parent, req.Title, portalOptions(req, token))
	if err := call.Store(&handle); err != nil {
		// No portal, or one without a file chooser.
		return "", &errNoNativePicker{Err: err}
	}

	for sig := range signals {
		// Portals before 0.9 ignore handle_token; the handle they return
		// is the request path then.
		if sig.Name != portalResponse || (sig.Path != want && sig.Path != handle) {
			continue
		}
		return parseResponse(sig.Body)
	}
	return "", errors.New("file chooser: session bus connection closed")
}

// portalOptions builds OpenFile's options dictionary.
func portalOptions(req openRequest, token string) map[string]dbus.Variant {
	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"modal":        dbus.MakeVariant(true),
	}
	if f := portalFilterFor(req.Extensions); f != nil {
		opts["filters"] = dbus.MakeVariant([]portalFilter{*f})
		opts["current_filter"] = dbus.MakeVariant(*f)
	}
	if req.StartDir != "" {
		// A byte string with a trailing NUL, as the portal requires.
		opts["current_folder"] = dbus.MakeVariant(append([]byte(req.StartDir), 0))
	}
	return opts
}

// portalFilterFor matches the extensions in either case, since portal
// globs are case-sensitive.
func portalFilterFor(exts []string) *portalFilter {
	if len(exts) == 0 {
		return nil
	}
	f := &portalFilter{Name: "Config files (" + strings.Join(exts, ", ") + ")"}
	for _, e := range exts {
		f.Rules = append(f.Rules,
			portalRule{Pattern: "*" + strings.ToLower(e)},
			portalRule{Pattern: "*" + strings.ToUpper(e)},
		)
	}
	return f
}

// requestPath is the object path the portal gives the request: the
// caller's unique name without the colon and with dots as underscores,
// then the token.
func requestPath(uniqueName, token string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(uniqueName, ":"), ".", "_")
	return dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
}

func handleToken() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "nebulagui_" + hex.EncodeToString(b), nil
}

// parseResponse reads the Response signal's (u a{sv}) body: a status, and
// the chosen files as file:// URIs.
func parseResponse(body []any) (string, error) {
	if len(body) != 2 {
		return "", fmt.Errorf("file chooser: unexpected response %v", body)
	}
	status, ok := body[0].(uint32)
	if !ok {
		return "", fmt.Errorf("file chooser: unexpected status %v", body[0])
	}
	if status != 0 {
		// 1 is cancelled; 2 means the dialog ended some other way, such
		// as being closed with its parent. Neither is an error to show.
		return "", nil
	}
	results, _ := body[1].(map[string]dbus.Variant)
	uris, _ := results["uris"].Value().([]string)
	if len(uris) == 0 {
		return "", nil
	}
	return pathFromURI(uris[0])
}

// pathFromURI turns a file:// URI into a path. url.Parse decodes %XX but,
// unlike a query decoder, leaves "+" alone, so "alice+bob.yml" survives.
func pathFromURI(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("file chooser: %w", err)
	}
	if u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return "", fmt.Errorf("file chooser: %s isn't a local file", s)
	}
	return u.Path, nil
}
