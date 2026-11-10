<!-- @authorship: AI-Assisted -->
<!-- @architect: AI (Claude Opus 5.5) -->

# nebula-gui

A cross-platform desktop app (Windows, Linux, macOS) for managing [Nebula](https://github.com/slackhq/nebula) overlay-network connections: import a config, connect or disconnect, see connected peers, and ping them.

> **Personal project.** Built for a private Nebula network shared with friends and family. The repo is public so they can download releases and updates. You're welcome to use it, but there's no support, and issues or pull requests may go unanswered.

> **Status:** pre-alpha, design phase. No code yet.

## Design branches

Two architectures are being considered. Each branch has a full set of design docs (architecture, IPC API, updates, security, testing, roadmap).

| Branch | Architecture | In short |
|---|---|---|
| [`monolithic`](../../tree/monolithic) | Nebula **built into** the service as a Go library | One engine process, typed peer data, smaller attack surface. Nebula updates ship as new app releases. |
| [`service`](../../tree/service) | Upstream **`nebula` binary** run as a supervised child process | Nebula updates and rollbacks without an app release. Controlled through Nebula's SSH debug console, so it needs per-version parsers. No mobile support. |

Both share the same basics: a Go + [Fyne](https://fyne.io) GUI running as the user, a privileged background service (`nebula-guid`) that owns the tunnel, and HTTP+JSON IPC between them.

`main` will take the chosen design once a decision is made. Until then, start with the README on either branch. The `service` README includes a side-by-side comparison.

## Authorship

Each file starts with two tags recording how it was made:

- `@authorship`: who wrote it.
  - `Human-Primary`: written by the maintainer, with at most minor AI help.
  - `AI-Assisted`: an AI assistant wrote a substantial part of it.
- `@architect`: who designed it.
  - `Philip Thomas`: the maintainer designed its structure, and reviewed and refined any AI-written parts.
  - `AI (<model>)`: an AI assistant proposed the design as well as writing it; the maintainer requested, reviewed and integrated it.

Generated files, files that can't hold comments, and repo configuration files (such as `.gitignore`) aren't tagged.
