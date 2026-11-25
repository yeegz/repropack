# ADR 5: desktop stack and capture

**Status:** accepted, 25 Nov 2026

**Stack.** Wails v2, a Go backend with the system WebView, and Svelte 5 with TypeScript. This gives one language for the processing engine and the app, a small bundle (the app is about 14 MB), a native window and menus, and a fast, accessible UI layer. SQLite comes from `modernc.org/sqlite`, which is pure Go, so there's no cgo toolchain for contributors.

**Capture on macOS.** The app uses the system's `/usr/sbin/screencapture` in interactive modes only:
- `-i -w` to choose a window
- `-i -s` to choose an area
- `-i -J video -U` for a recording, with the system toolbar, a menu-bar stop button and a 5-minute cap

Each capture requires the person to choose in an OS-owned picker, macOS's own recording indicator appears, and permission is handled by the OS's Screen Recording consent. The app adds its own always-visible recording bar. There is no keystroke, clipboard or accessibility capture. "Watching an app" reads only `ps` CPU and RSS for the chosen PID.

**Not yet.** Windows (Graphics Capture API) and Linux (xdg-desktop-portal ScreenCast) backends. The capture code is isolated in `capture.go` behind `capabilities()`, and the UI degrades to "add files you already have".

**CSP lesson.** A strict `script-src 'self'` meta tag blanks the app, because Wails injects inline bootstrap scripts. The policy is served as an HTTP header from asset-server middleware instead. It allows inline scripts but no remote origins, no frames, no objects and no forms. Report content is never rendered as HTML.
