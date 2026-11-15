// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"context"
	"errors"
	"net/netip"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/tucono/nebula-gui/internal/api"
)

const (
	peersNoteRunning = "Not connected means there's no connection right now. Ping to check whether it's reachable."
	peersNoteStopped = "Connect to see who's online."
	peersEmpty       = "This list shows the computers on your Nebula network that this one is talking to. " +
		"Give them names you recognise, and they stay in the list even when they're not connected."
	// copiedFor is how long a row says "Copied" after Copy IP.
	copiedFor = 2 * time.Second
	// renameWidth keeps the Rename dialog from growing with its text.
	renameWidth = 380
)

// peerStateText is the state block of a row, such as
// "Connected · direct · 23 ms".
func peerStateText(r peerRow) string {
	switch r.Status {
	case peerConnected:
		s := "Connected · direct"
		if r.Relayed {
			s = "Connected · relayed"
		}
		if r.LastPingMs != nil {
			s += " · " + formatMs(*r.LastPingMs)
		}
		return s
	case peerConnecting:
		return "Connecting…"
	default:
		return "Not connected"
	}
}

// formatMs rounds a round-trip time to whole milliseconds.
func formatMs(ms float64) string {
	if ms < 1 {
		return "<1 ms"
	}
	return strconv.FormatFloat(ms, 'f', 0, 64) + " ms"
}

// peerNameMessage words a cleanPeerName error for the user.
func peerNameMessage(err error) string {
	switch {
	case errors.Is(err, errPeerNameLong):
		return "That name is too long. Names can be at most " + strconv.Itoa(maxPeerNameLen) + " characters."
	case errors.Is(err, errPeerNameCtrl):
		return "Names can't contain line breaks or tabs."
	default:
		return "Give it a name."
	}
}

// peerRowItem is one row of the list: the name and the IP below it, the
// state, and the ⋯ button. Long names are cut off so the window never
// grows.
type peerRowItem struct {
	widget.BaseWidget
	title, sub, state *widget.Label
	more              *widget.Button
	content           fyne.CanvasObject
}

func newPeerRowItem() *peerRowItem {
	it := &peerRowItem{
		title: widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		sub:   widget.NewLabel(""),
		state: widget.NewLabel(""),
		more:  widget.NewButtonWithIcon("", theme.MoreHorizontalIcon(), nil),
	}
	it.title.Truncation = fyne.TextTruncateEllipsis
	it.sub.Truncation = fyne.TextTruncateEllipsis
	it.sub.SizeName = theme.SizeNameCaptionText
	it.sub.Importance = widget.LowImportance
	it.more.Importance = widget.LowImportance
	names := container.New(layout.NewCustomPaddedVBoxLayout(0), it.title, it.sub)
	it.content = container.NewBorder(nil, nil, nil,
		container.NewHBox(container.NewCenter(it.state), container.NewCenter(it.more)), names)
	it.ExtendBaseWidget(it)
	return it
}

func (it *peerRowItem) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(it.content)
}

// peersView is the Peers tab: a note, the list, and an empty state when
// the list is empty. Only the UI goroutine touches it.
type peersView struct {
	rows    []peerRow
	note    *widget.Label
	addB    *widget.Button
	list    *widget.List
	empty   *fyne.Container
	root    fyne.CanvasObject
	onMore  func(r peerRow, from fyne.CanvasObject)
	copied  netip.Addr // the row that says "Copied", if any
	copySeq int
	// pinging are the rows with a ping running.
	pinging map[netip.Addr]bool
	// result is the latest ping's result, above the list.
	result *widget.Label
}

// newPeersView returns the Peers tab. onMore opens a row's ⋯ menu; onAdd
// opens the Add peer dialog.
func newPeersView(onMore func(r peerRow, from fyne.CanvasObject), onAdd func()) *peersView {
	v := &peersView{onMore: onMore, pinging: map[netip.Addr]bool{}}
	v.note = widget.NewLabel(peersNoteStopped)
	v.note.Wrapping = fyne.TextWrapWord
	v.note.Importance = widget.LowImportance
	v.addB = widget.NewButtonWithIcon("Add peer…", theme.ContentAddIcon(), onAdd)
	v.result = widget.NewLabel("")
	v.result.Wrapping = fyne.TextWrapWord
	v.result.Hide()

	v.list = widget.NewList(
		func() int { return len(v.rows) },
		func() fyne.CanvasObject { return newPeerRowItem() },
		func(i widget.ListItemID, o fyne.CanvasObject) { v.update(i, o.(*peerRowItem)) },
	)
	// Rows aren't selectable; the ⋯ button holds the actions.
	v.list.OnSelected = func(widget.ListItemID) { v.list.UnselectAll() }

	emptyText := widget.NewLabel(peersEmpty)
	emptyText.Wrapping = fyne.TextWrapWord
	v.empty = container.NewVBox(emptyText)
	v.empty.Hide()

	top := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewCenter(v.addB), v.note),
		v.result,
	)
	v.root = container.NewPadded(container.NewBorder(top, nil, nil, nil,
		container.NewStack(v.list, v.empty)))
	return v
}

func (v *peersView) update(i widget.ListItemID, it *peerRowItem) {
	if i < 0 || i >= len(v.rows) {
		return
	}
	r := v.rows[i]
	it.title.SetText(r.Title)
	it.sub.SetText(r.Subtitle)
	if v.copied.IsValid() && v.copied == r.IP {
		it.state.Importance = widget.MediumImportance
		it.state.SetText("Copied")
	} else if v.pinging[r.IP] {
		it.state.Importance = widget.MediumImportance
		it.state.SetText(pingTextPinging)
	} else {
		switch r.Status {
		case peerConnected:
			it.state.Importance = widget.SuccessImportance
		case peerConnecting:
			it.state.Importance = widget.MediumImportance
		default:
			it.state.Importance = widget.LowImportance
		}
		it.state.SetText(peerStateText(r))
	}
	it.more.OnTapped = func() { v.onMore(r, it.more) }
}

// set shows rows. running picks the note above the list.
func (v *peersView) set(rows []peerRow, running bool) {
	v.rows = rows
	if running {
		v.note.SetText(peersNoteRunning)
	} else {
		v.note.SetText(peersNoteStopped)
		v.result.Hide() // a result from before disconnecting is stale
	}
	v.empty.Hidden = len(rows) > 0
	v.list.Hidden = len(rows) == 0
	v.empty.Refresh()
	v.list.Refresh()
}

// setPinging marks ip's row as pinging or not.
func (v *peersView) setPinging(ip netip.Addr, on bool) {
	if on {
		v.pinging[ip] = true
	} else {
		delete(v.pinging, ip)
	}
	v.list.Refresh()
}

// showResult shows a ping's result above the list.
func (v *peersView) showResult(text string, k pingKind) {
	switch k {
	case pingAnswered:
		v.result.Importance = widget.SuccessImportance
	case pingNoAnswer, pingError:
		v.result.Importance = widget.WarningImportance
	default:
		v.result.Importance = widget.MediumImportance
	}
	v.result.SetText(text)
	v.result.Show()
}

// showCopied makes ip's row say "Copied" for a moment.
func (v *peersView) showCopied(ip netip.Addr) {
	v.copied = ip
	v.copySeq++
	seq := v.copySeq
	v.list.Refresh()
	time.AfterFunc(copiedFor, func() {
		fyne.Do(func() {
			if v.copySeq == seq {
				v.copied = netip.Addr{}
				v.list.Refresh()
			}
		})
	})
}

// peersShowing reports whether the Peers tab is the selected one.
func (m *mainWindow) peersShowing() bool {
	return m.tabs.Selected() != nil && m.tabs.Selected().Content == m.peers.root
}

// running reports whether the latest status says Nebula is running.
func (m *mainWindow) running() bool {
	return m.in.Status != nil && m.in.Status.State == api.StateRunning
}

// renderPeers rebuilds the Peers tab from the latest poll and the saved
// peers.
func (m *mainWindow) renderPeers() {
	running := m.running()
	m.peers.set(buildPeerRows(m.in.Peers, m.saved.Peers(), statusOverlays(m.in.Status), running), running)
}

// peerMenu returns the ⋯ menu items for r.
func (m *mainWindow) peerMenu(r peerRow) []*fyne.MenuItem {
	items := []*fyne.MenuItem{}
	if r.Actions.Ping {
		ping := fyne.NewMenuItem("Ping", func() { m.pingPeer(r.IP, r.Title) })
		ping.Disabled = m.peers.pinging[r.IP]
		items = append(items, ping)
	}
	items = append(items, fyne.NewMenuItem("Copy IP", func() { m.copyPeerIP(r) }))
	if r.Saved() {
		items = append(items, fyne.NewMenuItem("Rename…", func() { m.renamePeer(r) }))
	} else {
		items = append(items, fyne.NewMenuItem("Give it a name…", func() { m.renamePeer(r) }))
	}
	if r.Actions.Remove {
		items = append(items, fyne.NewMenuItem("Remove from list", func() { m.removePeer(r) }))
	}
	if r.Actions.Close {
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Close connection", func() { m.confirmClosePeer(r) }))
	}
	return items
}

// showPeerMenu opens r's ⋯ menu below the button.
func (m *mainWindow) showPeerMenu(r peerRow, from fyne.CanvasObject) {
	c := m.w.Canvas()
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(from).AddXY(0, from.Size().Height)
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", m.peerMenu(r)...), c, pos)
}

func (m *mainWindow) copyPeerIP(r peerRow) {
	fyne.CurrentApp().Clipboard().SetContent(r.IP.String())
	m.peers.showCopied(r.IP)
}

// renamePeer asks for a name and saves it. A peer without a saved name
// starts from its cert name.
func (m *mainWindow) renamePeer(r peerRow) {
	title, initial := "Give it a name", r.CertName
	if r.Saved() {
		title, initial = "Rename", r.SavedName
	}
	e := newDialogEntry()
	e.SetText(initial)
	e.SetPlaceHolder("For example, Brother's PC")
	e.Validator = func(s string) error {
		if _, err := cleanPeerName(s); err != nil {
			return errors.New(peerNameMessage(err))
		}
		return nil
	}
	ip := widget.NewLabel(r.IP.String())
	d := dialog.NewForm(title, "Save", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Address", ip),
		widget.NewFormItem("Name", e),
	}, func(ok bool) {
		if ok {
			m.savePeerName(r.IP, e.Text)
		}
	}, m.w)
	// Enter saves, unless the name is invalid. Submit checks only the
	// Save button, which Fyne may not have disabled yet.
	e.OnSubmitted = func(string) {
		if e.Validate() == nil {
			d.Submit()
		}
	}
	e.onEscape = d.Hide
	d.Resize(fyne.NewSize(renameWidth, d.MinSize().Height))
	d.Show()
	m.w.Canvas().Focus(e)
}

// savePeerName gives ip the name, whether or not it had one.
func (m *mainWindow) savePeerName(ip netip.Addr, name string) {
	err := m.saved.Rename(ip, name)
	if errors.Is(err, errPeerNotSaved) {
		err = m.saved.Add(ip, name)
	}
	if err != nil {
		m.peerSaveFailed(err)
	}
	m.renderPeers()
}

func (m *mainWindow) removePeer(r peerRow) {
	if err := m.saved.Remove(r.IP); err != nil {
		m.peerSaveFailed(err)
	}
	m.renderPeers()
}

// peerSaveFailed tells the user the list couldn't be changed. The GUI log
// gets the error but never the name.
func (m *mainWindow) peerSaveFailed(err error) {
	msg := "Your list couldn't be saved, so nothing changed. " + err.Error()
	switch {
	case errors.Is(err, errTooManyPeers):
		msg = "Your list is full. It can hold at most " + strconv.Itoa(maxSavedPeers) + " peers; remove one first."
	case errors.Is(err, errNoPeerName), errors.Is(err, errPeerNameLong), errors.Is(err, errPeerNameCtrl):
		msg = peerNameMessage(err)
	default:
		m.log.Add("couldn't save peer names", "error", err.Error())
	}
	dialog.ShowInformation("Couldn't change your list", msg, m.w)
}

// confirmClosePeer asks before closing r's tunnel.
func (m *mainWindow) confirmClosePeer(r peerRow) {
	d := dialog.NewConfirm("Close connection?",
		"Close the connection to "+r.Title+"? It reconnects the next time either computer sends something.",
		func(ok bool) {
			if ok {
				m.closePeer(r.IP)
			}
		}, m.w)
	d.SetConfirmText("Close connection")
	d.Show()
}

// closePeer closes the tunnel to ip off the UI goroutine, then polls so
// the row changes. A tunnel that's already gone isn't an error.
func (m *mainWindow) closePeer(ip netip.Addr) {
	m.spawn(func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		err := m.c.ClosePeer(ctx, ip.String())
		fyne.Do(func() {
			switch code := api.ErrorCodeOf(err); {
			case err == nil, code == api.CodeNotFound, code == api.CodeNotRunning:
				m.log.Add("closed connection", "ip", ip.String())
			default:
				p := problemFor(err, problemContext{GOOS: m.in.GOOS})
				m.log.Add("close connection failed", "ip", ip.String(), "problem", string(p.ID), "error", errorDetails(err))
				msg := p.Title
				if p.Explain != "" {
					msg += "\n\n" + p.Explain
				}
				dialog.ShowInformation("Couldn't close the connection", msg, m.w)
			}
			m.poller.Poke()
		})
	})
}

// dialogEntry is an entry for a form dialog: Escape runs onEscape, which
// closes the dialog. Fyne's dialogs don't close on Escape by themselves,
// and the focused entry gets the key.
type dialogEntry struct {
	widget.Entry
	onEscape func()
}

func newDialogEntry() *dialogEntry {
	e := &dialogEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *dialogEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape && e.onEscape != nil {
		e.onEscape()
		return
	}
	e.Entry.TypedKey(k)
}
