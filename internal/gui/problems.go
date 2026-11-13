// @authorship: AI-Assisted
// @architect: AI (Claude Opus 5.5)

package gui

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/tucono/nebula-gui/internal/api"
)

// problem is everything the window shows about one thing that went wrong,
// in words for someone who isn't technical. It is plain data so the whole
// catalogue can be tested without Fyne. See "Friendly errors" in
// docs/m2-plan.md for the wording rules.
type problem struct {
	ID problemID
	// Info marks a message that isn't an error, such as the empty state
	// before the first import.
	Info    bool
	Title   string
	Explain string
	Steps   []string
	// Command is shown under the steps with a Copy button. It is the one
	// place a technical term may appear outside Details.
	Command string
	Fix     *fixAction
	// Details is collapsed by default: the error code, reason and raw
	// message. The view adds the app and service versions.
	Details string
}

// problemID names a catalogue entry, for tests and the GUI log.
type problemID string

// Catalogue entries, in the order of docs/m2-plan.md.
const (
	// Reaching the service.
	probServiceNotRunning    problemID = "service_not_running"
	probNoPermission         problemID = "no_permission"
	probServiceNotResponding problemID = "service_not_responding"
	probServiceTooOld        problemID = "service_too_old"
	probAppTooOld            problemID = "app_too_old"

	// Importing a config.
	probNoConfig       problemID = "no_config"
	probNotAConfig     problemID = "not_a_config"
	probFileMissing    problemID = "file_missing"
	probFileUnreadable problemID = "file_unreadable"
	probFileNotPEM     problemID = "file_not_pem"
	probPKIMissing     problemID = "pki_missing"
	probSSHDEnabled    problemID = "sshd_enabled"
	probCertExpired    problemID = "cert_expired"
	probCAExpired      problemID = "ca_expired"
	probKeyMismatch    problemID = "key_mismatch"
	probWrongCA        problemID = "wrong_ca"
	probKeyEncrypted   problemID = "key_encrypted"
	probConfigTooLarge problemID = "config_too_large"
	probConfigProblem  problemID = "config_problem"

	// Connecting.
	probTunUnavailable problemID = "tun_unavailable"
	probPortInUse      problemID = "port_in_use"
	probNebulaStopped  problemID = "nebula_stopped"
	probNewConfigSaved problemID = "new_config_saved"
	probConnectFailed  problemID = "connect_failed"
)

// fixAction is a button that fixes the problem directly. The window decides
// what each kind does; the catalogue only names it.
type fixAction struct {
	Label string
	Kind  fixKind
	// PKIKey is the pki entry a file fix is for: "ca", "cert" or "key".
	PKIKey string
}

type fixKind int

const (
	fixRetry         fixKind = iota + 1 // contact the service again now
	fixOpenDownloads                    // open the releases page
	fixImport                           // pick a config to import
	fixChooseConfig                     // pick another config file
	fixChooseFile                       // pick the file for PKIKey
	fixDisableSSHD                      // turn sshd off and import again
	fixConnect                          // connect again
	fixReconnect                        // disconnect, then connect
)

// downloadURL is the public releases page. It needs no sign-in.
const downloadURL = "https://github.com/tucono/nebula-gui/releases"

// contact is who the user asks when the app can't fix something.
const contact = "the person who set up your network"

// --- Reaching the service ---

func serviceNotRunning() *problem {
	return &problem{
		ID:      probServiceNotRunning,
		Title:   "The background service isn't running",
		Explain: "The Nebula service runs in the background and needs to be running for this app to work.",
		Steps: []string{
			"Restart your computer.",
			"If this keeps happening, reinstall the app.",
		},
		Fix: &fixAction{Label: "Try again", Kind: fixRetry},
	}
}

// noPermission takes the OS so tests can check every platform's wording.
func noPermission(goos string) *problem {
	p := &problem{
		ID:      probNoPermission,
		Title:   "You don't have permission to use Nebula on this computer",
		Explain: "Only some user accounts on this computer are allowed to control the Nebula connection, and yours isn't one of them yet.",
		Fix:     &fixAction{Label: "Try again", Kind: fixRetry},
	}
	switch goos {
	case "windows":
		p.Steps = []string{"Reinstall the app and choose your user account during setup."}
	default:
		p.Explain = "Your user account needs to be added to the nebula-gui group before it can control the Nebula connection."
		p.Steps = []string{
			"Ask whoever manages this computer to add you, or run the command below.",
			"Log out and back in.",
		}
		if goos == "darwin" {
			p.Command = `sudo dseditgroup -o edit -a "$USER" -t user nebula-gui`
		} else {
			p.Command = `sudo usermod -aG nebula-gui "$USER"`
		}
	}
	return p
}

func serviceNotResponding() *problem {
	return &problem{
		ID:      probServiceNotResponding,
		Title:   "The background service isn't responding",
		Explain: "The Nebula service is running but didn't answer in time.",
		Steps:   []string{"Click Try again.", "If this keeps happening, restart your computer."},
		Fix:     &fixAction{Label: "Try again", Kind: fixRetry},
	}
}

func serviceTooOld(service, app int) *problem {
	return &problem{
		ID:      probServiceTooOld,
		Title:   "The background service needs an update",
		Explain: "This app is newer than the Nebula service running in the background, so they can't work together.",
		Steps:   []string{"Install the latest version of the app; it updates the service too."},
		Fix:     &fixAction{Label: "Open download page", Kind: fixOpenDownloads},
		Details: fmt.Sprintf("service API version %d, app API version %d", service, app),
	}
}

func appTooOld(service, app int) *problem {
	return &problem{
		ID:      probAppTooOld,
		Title:   "This app needs an update",
		Explain: "The Nebula service running in the background is newer than this app, so they can't work together.",
		Steps:   []string{"Install the latest version of the app."},
		Fix:     &fixAction{Label: "Open download page", Kind: fixOpenDownloads},
		Details: fmt.Sprintf("service API version %d, app API version %d", service, app),
	}
}

// --- Importing a config ---

func noConfig() *problem {
	return &problem{
		ID:      probNoConfig,
		Info:    true,
		Title:   "Let's get you connected",
		Explain: "You should have received a config file, usually called config.yml, from " + contact + ".",
		Steps:   []string{"Click Import config… and pick that file."},
		Fix:     &fixAction{Label: "Import config…", Kind: fixImport},
	}
}

func notAConfig(fileName string) *problem {
	return &problem{
		ID:      probNotAConfig,
		Title:   "That doesn't look like a Nebula config file",
		Explain: fmt.Sprintf("You picked %s.", fileName),
		Steps:   []string{"Pick the config file you received, usually called config.yml."},
		Fix:     &fixAction{Label: "Choose another file…", Kind: fixChooseConfig},
	}
}

func fileMissing(pkiKey, fileName string) *problem {
	return &problem{
		ID:      probFileMissing,
		Title:   "A file is missing: " + fileName,
		Explain: fmt.Sprintf("Your config expects %s at a place that doesn't exist on this computer.", fileName),
		Steps:   []string{fmt.Sprintf("Find the %s you were given, then click Choose the file….", fileName)},
		Fix:     &fixAction{Label: "Choose the file…", Kind: fixChooseFile, PKIKey: pkiKey},
	}
}

func fileUnreadable(pkiKey, fileName string) *problem {
	return &problem{
		ID:      probFileUnreadable,
		Title:   "Can't open " + fileName,
		Explain: "The file exists but can't be opened.",
		Steps:   []string{"Copy it somewhere in your home folder and try again."},
		Fix:     &fixAction{Label: "Choose the file…", Kind: fixChooseFile, PKIKey: pkiKey},
	}
}

func fileNotPEM(pkiKey, fileName string) *problem {
	what := "certificate file"
	if pkiKey == "key" {
		what = "key file"
	}
	return &problem{
		ID:      probFileNotPEM,
		Title:   fmt.Sprintf("%s isn't a %s", fileName, what),
		Explain: "It may have been damaged while copying.",
		Steps:   []string{"Ask " + contact + " for the files again."},
		Fix:     &fixAction{Label: "Choose the file…", Kind: fixChooseFile, PKIKey: pkiKey},
	}
}

func pkiMissing() *problem {
	return &problem{
		ID:      probPKIMissing,
		Title:   "This config is incomplete",
		Explain: "It doesn't say which certificate to use.",
		Steps:   []string{"Ask " + contact + " for a new config."},
	}
}

func sshdEnabled() *problem {
	return &problem{
		ID:      probSSHDEnabled,
		Title:   "This config turns on a remote admin console",
		Explain: "This app doesn't allow it, for safety.",
		Steps:   []string{"Click Turn it off and import. Nothing else in the config changes."},
		Fix:     &fixAction{Label: "Turn it off and import", Kind: fixDisableSSHD},
	}
}

// certExpired takes the expiry date when it's known.
func certExpired(notAfter *time.Time) *problem {
	return &problem{
		ID:      probCertExpired,
		Title:   "Your network certificate has expired",
		Explain: "Certificates are only valid for a limited time." + ranOut(notAfter),
		Steps:   []string{"Ask " + contact + " for a new config."},
	}
}

func caExpired(notAfter *time.Time) *problem {
	return &problem{
		ID:      probCAExpired,
		Title:   "Your network's certificate authority has expired",
		Explain: "Every certificate on your network depends on it, so none of them work any more." + ranOut(notAfter),
		Steps:   []string{"Ask " + contact + " for a new config."},
	}
}

func ranOut(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return " It ran out on " + formatDate(*t) + "."
}

// formatDate writes a date the way people say it: "3 March 2027".
func formatDate(t time.Time) string {
	return t.Local().Format("2 January 2006")
}

func keyMismatch(keyName, certName string) *problem {
	return &problem{
		ID:      probKeyMismatch,
		Title:   "Your key file doesn't match your certificate",
		Explain: "The config may be mixing files from two computers.",
		Steps: []string{
			fmt.Sprintf("Make sure %s and %s came together.", orDefault(keyName, "the key file"), orDefault(certName, "the certificate")),
			"If you're not sure, ask " + contact + " for a new config.",
		},
	}
}

func wrongCA() *problem {
	return &problem{
		ID:      probWrongCA,
		Title:   "Your certificate belongs to a different network",
		Explain: "It wasn't issued by the network this config is for.",
		Steps:   []string{"Ask " + contact + " for a new config."},
	}
}

func keyEncrypted(keyName string) *problem {
	return &problem{
		ID:      probKeyEncrypted,
		Title:   "Your key file is password-protected",
		Explain: fmt.Sprintf("This app can't use password-protected keys, and %s is one.", orDefault(keyName, "yours")),
		Steps:   []string{"Ask " + contact + " for an unprotected key."},
	}
}

func configTooLarge() *problem {
	return &problem{
		ID:      probConfigTooLarge,
		Title:   "This config file is too large",
		Explain: "Config files are usually a few kilobytes.",
		Steps:   []string{"Make sure you picked the right file."},
		Fix:     &fixAction{Label: "Choose another file…", Kind: fixChooseConfig},
	}
}

func configProblem() *problem {
	return &problem{
		ID:      probConfigProblem,
		Title:   "This config file has a problem",
		Explain: "Nebula can't use it as it is.",
		Steps:   []string{"Click Get help… and send the report to " + contact + "."},
	}
}

// --- Connecting ---

func tunUnavailable(goos string) *problem {
	p := &problem{
		ID:      probTunUnavailable,
		Title:   "Couldn't create the network adapter",
		Explain: "The background service doesn't have the permission it needs.",
		Steps:   []string{"Reinstall the app."},
	}
	if goos == "windows" {
		p.Explain = "The background service couldn't set up the network adapter; the Wintun driver may be missing."
	}
	return p
}

func portInUse() *problem {
	return &problem{
		ID:      probPortInUse,
		Title:   "Another program is using Nebula's port",
		Explain: "Usually this means Nebula is already running on this computer outside this app.",
		Steps:   []string{"Close the other Nebula, then click Connect."},
		Fix:     &fixAction{Label: "Connect", Kind: fixConnect},
	}
}

func nebulaStopped() *problem {
	return &problem{
		ID:      probNebulaStopped,
		Title:   "The connection stopped unexpectedly",
		Explain: "Nebula stopped on its own.",
		Steps:   []string{"Click Connect again.", "If it keeps stopping, click Get help…."},
		Fix:     &fixAction{Label: "Connect again", Kind: fixConnect},
	}
}

func newConfigSaved() *problem {
	return &problem{
		ID:      probNewConfigSaved,
		Info:    true,
		Title:   "New config saved",
		Explain: "You're still connected with the old one.",
		Steps:   []string{"Click Reconnect to use it."},
		Fix:     &fixAction{Label: "Reconnect to use it", Kind: fixReconnect},
	}
}

func connectFailed() *problem {
	return &problem{
		ID:      probConnectFailed,
		Title:   "Couldn't connect",
		Explain: "Something went wrong while starting the connection.",
		Steps:   []string{"Click Try again.", "If it fails again, click Get help…."},
		Fix:     &fixAction{Label: "Try again", Kind: fixConnect},
	}
}

// --- Choosing an entry ---

// op is what the user was doing when an error came back. The same service
// error can mean different things during an import and a connect.
type op int

const (
	opOther op = iota
	opImport
	opConnect
)

// problemContext is what the catalogue needs to fill in names and dates.
type problemContext struct {
	Op op
	// ConfigName is the file name of the config being imported.
	ConfigName string
	// KeyName and CertName are the file names from pki.key and pki.cert,
	// when the config gave paths.
	KeyName, CertName string
	// Cert gives the expiry dates for cert_expired and ca_expired.
	Cert *api.CertInfo
	GOOS string
}

// problemFor turns any error from an api.Client call into a catalogue
// entry: a service error by its code and reason, a transport error by
// what it says about the service. It never returns nil for a non-nil err.
func problemFor(err error, pc problemContext) *problem {
	if err == nil {
		return nil
	}
	if pc.GOOS == "" {
		pc.GOOS = runtime.GOOS
	}
	var p *problem
	var ae *api.Error
	if errors.As(err, &ae) {
		p = fromAPIError(ae, pc)
	} else {
		p = fromTransportError(err, pc.GOOS)
	}
	p.Details = joinDetails(p.Details, errorDetails(err))
	return p
}

func fromAPIError(e *api.Error, pc problemContext) *problem {
	var certNotAfter, caNotAfter *time.Time
	if pc.Cert != nil {
		certNotAfter, caNotAfter = &pc.Cert.NotAfter, pc.Cert.CANotAfter
	}
	// Reasons first: they are what the service knows for sure. Unknown
	// reasons fall through to the code, as docs/ipc-api.md requires.
	switch e.Reason {
	case api.ReasonCertExpired:
		return certExpired(certNotAfter)
	case api.ReasonCAExpired:
		return caExpired(caNotAfter)
	case api.ReasonKeyMismatch:
		return keyMismatch(pc.KeyName, pc.CertName)
	case api.ReasonWrongCA:
		return wrongCA()
	case api.ReasonKeyEncrypted:
		return keyEncrypted(pc.KeyName)
	case api.ReasonSSHDEnabled:
		return sshdEnabled()
	case api.ReasonPKIMissing:
		return pkiMissing()
	case api.ReasonYAMLSyntax:
		return notAConfig(orDefault(pc.ConfigName, "a file"))
	case api.ReasonTunUnavailable:
		return tunUnavailable(pc.GOOS)
	case api.ReasonPortInUse:
		return portInUse()
	case api.ReasonNebulaStopped:
		return nebulaStopped()
	}
	switch {
	case e.Code == api.CodeInvalidConfig:
		return configProblem()
	case e.Code == api.CodeNotConfigured:
		return noConfig()
	case pc.Op == opImport && e.Code == api.CodeBadRequest && strings.Contains(e.Message, "too large"):
		return configTooLarge()
	case pc.Op == opImport:
		return configProblem()
	default:
		return connectFailed()
	}
}

// statusProblem returns the problem the status itself shows, even when no
// call failed: an error state, or a cert that has expired since import.
// It returns nil when there is nothing to show.
func statusProblem(st *api.StatusResponse, goos string) *problem {
	if st == nil {
		return nil
	}
	if !st.Configured {
		return noConfig()
	}
	if st.Cert != nil {
		switch {
		case st.Cert.CAExpired:
			return caExpired(st.Cert.CANotAfter)
		case st.Cert.Expired:
			return certExpired(&st.Cert.NotAfter)
		}
	}
	if st.State != api.StateError {
		return nil
	}
	e := &api.Error{Code: api.CodeInternal, Reason: st.ErrorReason, Message: st.Error}
	p := fromAPIError(e, problemContext{Op: opConnect, Cert: st.Cert, GOOS: goos})
	p.Details = joinDetails(p.Details, errorDetails(e))
	return p
}

// versionProblem returns nil when the service speaks this app's API.
func versionProblem(v *api.VersionResponse) *problem {
	switch {
	case v.APIVersion > api.APIVersion:
		return appTooOld(v.APIVersion, api.APIVersion)
	case v.APIVersion < api.APIVersion:
		return serviceTooOld(v.APIVersion, api.APIVersion)
	default:
		return nil
	}
}

// errorDetails is the technical line under Details: "code / reason:
// message" for a service error, the Go error otherwise.
func errorDetails(err error) string {
	var ae *api.Error
	if !errors.As(err, &ae) {
		return err.Error()
	}
	s := string(ae.Code)
	if ae.Reason != "" {
		s += " / " + string(ae.Reason)
	}
	if ae.Message != "" {
		s += ": " + ae.Message
	}
	return s
}

func joinDetails(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "\n" + b
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
