# Verified background updates

The homepage DUI card checks the public `shini74744/Dui` releases for stable panel
(`v*`) and DUI core (`vx-*`) releases. Each component shows its installed and
latest compatible version separately. It checks when the homepage is opened,
caches results for one hour, and allows a manual refresh (one request per minute).
The header button checks directly in the card. Panel/core versions, update
actions, errors and background progress appear inline without a dialog. Closing
or reloading the page does not cancel a server-side download.

Only complete releases with the matching architecture archive and SHA-256 file
are offered. A failed check is shown explicitly; it does not imply up-to-date.

Updates require the panel to be running as root under the Linux `x-ui.service`
systemd unit. Other installation methods can inspect versions but must use their
installer. Panel updates replace the embedded panel executable; core updates
replace the Xray executable. They do not overwrite the other component, helper
executables, service unit, shell scripts, certificates or geodata. Update the core
separately when release notes require a newer core. The old core-version selector
uses the same background transaction.

## Transaction

1. An authenticated, same-origin POST creates one root-private server-side job.
   A separate systemd worker downloads the release, surviving page closure and
   panel restarts. A process lock prevents concurrent update transactions.
2. Download into a private staging directory on the target filesystem. Persist
   downloaded/total bytes and phase roughly once per second. No overall download
   deadline; connection/header deadlines and a 90-second stalled-read deadline
   prevent a dead connection from hanging indefinitely. Interrupted downloads
   fail safely; Retry starts a fresh download.
3. Require GitHub's archive size, the exact filename in the SHA-256 sidecar, the
   matching SHA-256, a valid archive, the exact executable version, and a
   successful executable launch. Reject links, traversal and duplicate executable
   entries. Test a staged core against the current runtime configuration before
   installation, without opening listeners.
4. Before stopping the service, keep the previous executable. With the service
   stopped, back up SQLite using VACUUM INTO and copy the runtime core config.
   Replace the executable by rename on the same filesystem, then fsync the
   directory. No live executable is opened for truncation.
5. Restart and wait for panel startup readiness, the expected running executable
   hash, and core readiness when previously running. For a core update, also
   verify the running child executable. Require three consecutive healthy checks.
   Failure restores the previous executable, database and config, then checks
   the restored service. Failure of recovery is reported distinctly.

Download, integrity or preflight failures never replace the installed executable.
Normal installation briefly restarts the panel and core; download does not.
Backups stay in the root-private `.dui-update-*` staging directory next to the
updated executable. `<DB folder>/updates/job.json` records the latest transaction.
If the machine or worker terminates during installation, the UI reports recovery
required; do not start a second transaction over those backups. The worker can be
rerun with its original job ID to restore the previous version. Automatic
rollback cannot guarantee recovery from disk/hardware failure or loss of power.

Checksums protect against incomplete or altered downloads, not compromise of the
GitHub repository or release account. Only the fixed DUI repository is used.
No credentials, configuration contents, node URLs or connection addresses are
included in job status.

## Verification

- `go test ./internal/update ./web/service ./web/controller`
- `node scripts/test-updates-ui.cjs` with Playwright/Chromium
- `python3 scripts/test-updates-integration.py` with DUI_TEST_PANEL/DUI_TEST_CORE

Tests cover slow/interrupted downloads, length and checksum failures, archive
validation, wrong executable versions, exclusive jobs, rollback, authenticated
API access, cross-origin rejection, refresh persistence, 13 locales and mobile/
desktop light/dark layouts.

An opt-in systemd end-to-end check is available as
`DUI_UPDATE_SYSTEMD_TEST=1 python3 scripts/test-updates-worker.py` with the same
binary environment variables. It creates and removes an isolated test unit,
loopback-only panel and disposable DB. It tests the real download/install/restart
cycle using the published vx-26.6 core and intentionally installs panel v26.9.46
inside that test unit to exercise readiness failure and recovery. It never
replaces the installed business panel or core.
