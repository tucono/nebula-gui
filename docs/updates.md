<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# Updates

New versions are installed by hand, the same way as the first install ([README → Install](../README.md#install)). From v1.0 (M5) the app checks GitHub for a newer release and says so; it never downloads or installs anything. See ADR-009 in [decisions.md](decisions.md), which replaces ADR-006's self-updating design.

Nebula is built into `nebula-guid` (ADR-002), so a new Nebula version arrives as a new nebula-gui release with a newer `github.com/slackhq/nebula` pinned.

## Installing a new version

Download the new installer from the releases page and run it over the old one. Every installer upgrades in place and keeps the imported config, the settings and who has access:

| Platform | What the upgrade does |
|---|---|
| Windows | The Inno Setup installer stops the service, ends the app in every session, replaces the files, keeps the service's registration, and starts the service again if it was running (ADR-008). |
| macOS | `preinstall` boots the daemon out and quits the app; `postinstall` bootstraps it again. |
| Linux | The package's scripts run `systemctl try-restart`, so a stopped or disabled service stays that way. |

The tunnel goes down while the service restarts. With **Connect automatically when this computer starts** on, the service reconnects; otherwise the user clicks **Connect** again.

## The update check (M5)

The M5 plan settles the details. The outline:

- **What it asks:** `GET https://api.github.com/repos/tucono/nebula-gui/releases/latest`, without authentication. "Latest" skips drafts and pre-releases. The tag's version is compared with the running one as semver; development builds (`0.0.0-dev…`) don't check.
- **What the user sees:** when a newer version is out, a notice in the window and a tray notification, such as *"nebula-gui 0.2.0 is available"*, with a button that opens the release page. Settings has a switch to turn the check off.
- **Who checks:** the GUI, as the signed-in user, at start and about once a day while it runs. The service stays out of it: it needs no internet access beyond Nebula's own, and gains no endpoints. A computer where nobody opens the app doesn't check, which is acceptable for a notice.
- **Limits:** unauthenticated requests are limited to 60 an hour per IP address, far more than one a day needs. Responses are cached with their ETag. A failed check is logged and retried at the next interval; it never shows a problem.

## Privacy

The check is a plain HTTPS request to GitHub's API. It sends no identifiers, config details or telemetry; GitHub sees the computer's public IP address and the app's User-Agent (`nebula-gui/<version>`). It can be turned off.
