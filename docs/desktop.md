# Desktop app: design and accessibility

## Identity

The app is built around a sealed evidence bag: what's inside, when it happened, and proof it hasn't been touched.

- **Palette.**
  - Neutral paper `#F5F6F4` with white surfaces and ink `#1A1C1F`.
  - Dark mode is a deep `#0B0C0F` canvas.
  - One **seal teal** (`#0B7A66` light, `#3CCBAA` dark), used only for integrity and primary actions.
  - Errors, crashes and warnings each have their own colour (red, crimson, orange). Recording uses a separate signal red.
- **Type.** Schibsted Grotesk for everything people read. IBM Plex Mono only for timestamps, hashes and code. Both are bundled locally and never fetched.
- **The one bold element: the evidence tape.** A lane timeline (crashes, errors, warnings, your marks, actions, logs, requests, plus metric sparklines) with screenshots and notes pinned above. It grows live while you capture, and in the viewer you brush it to filter events. Everything else is quiet.
- **No generic dashboard chrome.** No dot status indicators, no all-caps labels, no identical card grids. Capture actions are rows with plain verbs. Status is written in words: "Untouched since saved", "Recording your screen".

## Flows

1. **Describe.** Title, what happened, what you expected, steps (Return adds a step, ⌥↑/↓ reorders), and how often.
2. **Capture.**
   - Window or area screenshots through the macOS picker.
   - Screen recording with the system toolbar plus the app's own red bar and timer.
   - Add existing files; logs are parsed onto the timeline.
   - Watch an app's CPU and memory.
   - "It happened just now", and notes.
3. **Review.** Findings are grouped by rule, each with masked context and a Hide/Keep choice. Every image and recording must be confirmed. "Hide parts" blacks out areas for good.
4. **Share.** Save, then the app verifies and re-scans. You get a seal confirmation with size, contents, hidden count and fingerprint, plus "Show in Finder", an optional web page, and a prefilled issue link.

The viewer has four tabs:
- **Overview:** headline, facts, tape, the reporter's story, notes.
- **Events:** compact tape, filters, a windowed list and an inspector.
- **Evidence.**
- **System and privacy:** environment, redaction record, integrity table.

## Accessibility checklist

| Area | Implementation |
|---|---|
| Keyboard | Every action reachable with Tab; ⌘N, ⌘O and ⌘, shortcuts. The stepper uses `aria-current="step"`. Tabs follow the WAI-ARIA tabs pattern (arrow keys, roving tabindex). The event list is a listbox with `aria-activedescendant` and ↑↓, PageUp/PageDown, Home/End. The markup editor has H, O and A tool keys, ⌘Z and Escape. |
| Screen readers | The tape is a `slider` with a full text description and an `aria-valuetext` such as "0:42, 2 errors, 1 warning. Press Enter to focus here." Toasts are announced politely, errors as alerts. The recording bar is a live status. Icons are hidden or labelled. |
| Focus | Visible 2 px seal-coloured ring. The dialog keeps focus inside and returns it on close. Moving between steps focuses the step heading. |
| Motion | Only functional motion: toast entrance, dialog pop, export seal stamp, recording pulse. All of it collapses under `prefers-reduced-motion`. |
| Contrast | Body text ≥ 4.5:1 in both themes. `prefers-contrast: more` strengthens greys and rules. `forced-colors` maps marks to system colours. |
| Errors and recovery | Errors say what happened and what to do. Cancelling a picker is silent. Failed loads offer "Try again". Delete asks first and says what's kept. Drafts autosave to SQLite. |
| Layout | Works from 860×600 upwards. Columns collapse below 1100 px. |

## Previewing without Wails

```bash
cd desktop/frontend && npm run dev
```

In a plain browser the app loads `src/dev/mock.ts`, a fake backend with a realistic draft and the example report, so the interface can be reviewed and screenshotted. The mock is never included in production builds; Vite drops it because it's only imported behind `import.meta.env.DEV`.
