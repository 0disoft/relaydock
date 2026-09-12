# Windows Settings and Development Smoke

Verified on 2026-09-12 using Windows 11, Go 1.26.4, Wails
v3.0.0-alpha2.119, WebView2 152.0.4191.66, and the 0.5.13-dev candidate.

## Executed checks

- Five frontend tests passed, including settings loading/editing/saving/failure/reset
  guards and invalid numeric inputs. Svelte reported zero errors and warnings.
- The native Wails window rendered the overview and the redesigned settings page.
  Checkboxes no longer stretched across the form. The long repository path stayed
  within its input, and the save/reset controls were disabled for an unchanged draft.
- A temporary sidebar label appeared in the running native app after a source edit,
  without restarting the desktop process. The source edit was reverted; Vite logged
  both hot updates. No temporary marker is included in the committed UI.
- The supervised desktop process tree was stopped and a second session successfully
  rendered the overview using the same isolated data directory.

## Defects found by running the app

The initial cold Vite startup exceeded Wails' connection wait. A bounded readiness
helper now requires a successful HTTP response before launching the desktop binary.

Vite bound `localhost` to IPv6 while Wails' asset proxy dialed IPv4, leaving the app
unable to load its frontend. The development task now binds `127.0.0.1`, and the
readiness helper checks that same IPv4 address for a localhost development URL.

## Scope and limits

The bounded `scripts/desktop-smoke.ps1` runner uses isolated app data, refuses an
existing RelayDock instance or autostart registration, and cleans up only its own
supervisor process tree. Its fixed local frontend port is 19245. The test never
starts the API gateway or submits provider credentials.

Native save/reset clicks and graceful tray-menu Quit were not completed: concurrent
user input changed or hid the selected window. Their behavior must not be inferred
from screenshots or the forced process-tree restart. Save-state guards are covered
by the frontend tests; full native persistence, autostart, installers, other operating
systems, and release certification remain separate checks.
