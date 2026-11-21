// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// addPeerWidth keeps the Add peer dialog from growing with its messages.
const addPeerWidth = 420

// addPeerDialog is the open Add peer dialog. Its fields are for tests.
type addPeerDialog struct {
	d             *dialog.CustomDialog
	name, ip      *dialogEntry
	msg           *widget.Label
	add, addPing  *widget.Button
	nameTouched   bool
	ipTouched     bool
	submitDefault func()
}

// showAddPeer opens the Add peer dialog: a name and an overlay IP, checked
// as the user types. Add and ping needs Nebula running.
func (m *mainWindow) showAddPeer() {
	a := &addPeerDialog{
		name: newDialogEntry(),
		ip:   newDialogEntry(),
		msg:  widget.NewLabel(""),
	}
	a.name.SetPlaceHolder("For example, Brother's PC")
	a.ip.SetPlaceHolder("For example, " + exampleAddr(statusOverlays(m.in.Status)))
	// A wrapping label rather than the form's hint: the hint is one line
	// and would widen the dialog.
	a.msg.Wrapping = fyne.TextWrapWord
	a.msg.Importance = widget.DangerImportance
	a.msg.Hide()

	form := widget.NewForm(
		widget.NewFormItem("Name", a.name),
		widget.NewFormItem("Address", a.ip),
	)
	a.d = dialog.NewCustomWithoutButtons("Add peer", container.NewVBox(form, a.msg), m.w)

	submit := func(ping bool) {
		if m.checkAddPeer(a.name.Text, a.ip.Text) != nil {
			return
		}
		a.d.Hide()
		m.addPeer(a.name.Text, a.ip.Text, ping && m.running())
	}
	a.add = widget.NewButton("Add", func() { submit(false) })
	a.addPing = widget.NewButton("Add and ping", func() { submit(true) })
	cancel := widget.NewButton("Cancel", a.d.Hide)
	if m.running() {
		a.addPing.Importance = widget.HighImportance
		a.submitDefault = func() { submit(true) }
	} else {
		a.add.Importance = widget.HighImportance
		a.submitDefault = func() { submit(false) }
	}
	a.d.SetButtons([]fyne.CanvasObject{cancel, a.add, a.addPing})

	refresh := func() {
		m.refreshAddPeer(a)
		a.d.Resize(fyne.NewSize(addPeerWidth, a.d.MinSize().Height))
	}
	a.name.OnChanged = func(string) { a.nameTouched = true; refresh() }
	a.ip.OnChanged = func(string) { a.ipTouched = true; refresh() }
	for _, e := range []*dialogEntry{a.name, a.ip} {
		e.OnSubmitted = func(string) { a.submitDefault() }
		e.onEscape = a.d.Hide
	}

	m.addPeerD = a
	a.d.SetOnClosed(func() {
		if m.addPeerD == a {
			m.addPeerD = nil
		}
	})
	m.refreshAddPeer(a)
	a.d.Resize(fyne.NewSize(addPeerWidth, a.d.MinSize().Height))
	a.d.Show()
	m.w.Canvas().Focus(a.name)
}

// checkAddPeer checks the dialog's fields against the latest status and
// the saved peers.
func (m *mainWindow) checkAddPeer(name, ip string) error {
	return checkNewPeer(name, ip, m.in.Status, m.saved.Peers())
}

// refreshAddPeer enables the buttons when both fields are fine, and shows
// what's wrong with a field the user has typed in. The name comes first.
func (m *mainWindow) refreshAddPeer(a *addPeerDialog) {
	err := m.checkAddPeer(a.name.Text, a.ip.Text)
	msg := ""
	if _, nerr := cleanPeerName(a.name.Text); nerr != nil && a.nameTouched {
		msg = peerNameMessage(nerr)
	} else if ierr := checkNewPeerIP(a.ip.Text, m.in.Status, m.saved.Peers()); ierr != nil && a.ipTouched && a.ip.Text != "" {
		var pe *peerInputError
		if errors.As(ierr, &pe) {
			msg = pe.Msg
		}
	}
	a.msg.SetText(msg)
	a.msg.Hidden = msg == ""
	a.msg.Refresh()
	setEnabled(a.add, err == nil)
	setEnabled(a.addPing, err == nil && m.running())
}

// addPeer saves a new peer and, if asked, pings it. The GUI log gets the
// IP only.
func (m *mainWindow) addPeer(name, ipText string, ping bool) {
	ip, err := parsePeerAddr(ipText)
	if err == nil {
		err = m.saved.Add(ip, name)
	}
	if err != nil {
		m.peerSaveFailed(err)
		return
	}
	m.log.Add("peer added", "ip", ip.String())
	m.renderPeers()
	if ping {
		title, _ := cleanPeerName(name)
		m.pingPeer(ip, title)
	}
}
