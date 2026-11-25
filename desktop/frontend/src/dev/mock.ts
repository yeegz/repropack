// Browser-only mock of the Go backend, used for design previews. Never
// bundled into the desktop build (imported only when import.meta.env.DEV).
import fixture from "./fixture.json";
import shot from "./shot-pay.png";
import type { Draft, DraftAttachment, DraftDetail, Note, RPEvent } from "../lib/types";

const now = () => new Date().toISOString();
const drafts = new Map<string, DraftDetail>();
let seq = 0;
const id = () => Math.random().toString(16).slice(2, 10) + Math.random().toString(16).slice(2, 10);
const emit = (n: string, d: any) => window.dispatchEvent(new CustomEvent("rp:" + n, { detail: d }));

function newDraft(): DraftDetail {
  const d: Draft = { id: crypto.randomUUID(), createdAt: now(), updatedAt: now(), title: "", description: "", steps: [], expected: "", actual: "", frequency: "", targetPid: 0, targetName: "", manualRedactions: 0, exportedPath: "", exportedAt: "", eventCount: 1, attachmentCount: 0 };
  const det: DraftDetail = { draft: d, attachments: [], notes: [], events: [{ seq: ++seq, ts: now(), kind: "marker", level: "info", source: "repropack", msg: "Report started" }], watching: false, recording: false };
  drafts.set(d.id, det);
  return det;
}
// One realistic draft to look at.
{
  const det = newDraft();
  Object.assign(det.draft, { title: "Export to PDF hangs on large invoices", actual: "The progress bar stops at 80% and the app stops responding.", expected: "The PDF saves within a few seconds.", steps: ["Open an invoice with more than 200 lines", "Choose File → Export as PDF", "Wait"], frequency: "Every time", targetName: "Ledgerly", targetPid: 4121 });
  const t0 = Date.now() - 95_000;
  const add = (ms: number, e: Partial<RPEvent>) => det.events!.push({ seq: ++seq, ts: new Date(t0 + ms).toISOString(), kind: "log", ...e } as RPEvent);
  for (let i = 0; i < 90; i++) {
    add(i * 1000, { kind: "metric", source: "Ledgerly", metric: { name: "cpu", value: i > 55 ? 96 + Math.random() * 4 : 8 + Math.random() * 10, unit: "%" } });
    add(i * 1000, { kind: "metric", source: "Ledgerly", metric: { name: "memory", value: (310 + (i > 55 ? (i - 55) * 22 : Math.random() * 6)) * 1e6, unit: "By" } });
  }
  add(52_000, { kind: "log", level: "info", source: "ledgerly.log", msg: "export started invoice=INV-2041 lines=236" });
  add(56_400, { kind: "log", level: "warn", source: "ledgerly.log", msg: "layout pass exceeded budget (4200ms)" });
  add(61_000, { kind: "marker", level: "warn", source: "you", msg: "The problem happened here" });
  add(71_000, { kind: "log", level: "error", source: "ledgerly.log", msg: "renderer: worker did not respond in 10s; user=maria@example.com" });
  det.attachments.push({ id: "a1", mediaType: "image/png", role: "screenshot", title: "Window screenshot", createdAt: new Date(t0 + 64_000).toISOString(), width: 640, height: 400, durationMs: 0, annotated: false, reviewed: false, note: "", size: 5120, url: shot });
  det.notes.push({ id: "n1", at: new Date(t0 + 66_000).toISOString(), text: "Fans spin up right when it freezes." });
  det.draft.eventCount = det.events!.length;
  det.draft.attachmentCount = 1;
}

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v));
const delay = <T>(v: T, ms = 120) => new Promise<T>((r) => setTimeout(() => r(v), ms));
let watchTimer: any = null;

export const mock = {
  Environment: () => delay({ capabilities: { os: "darwin", screenshots: true, recording: true, processList: true, note: "macOS asks for Screen Recording permission the first time. You choose the window or area each time." }, version: "0.1.0", osName: "macOS", osVersion: "27.0", arch: "arm64", model: "Mac15,6", cpu: "Apple M3 Pro", memoryBytes: 36 * 2 ** 30, dataDir: "~/Library/Application Support/ReproPack", retentionDays: 30, issueRepo: "", issueHost: "github" }),
  SetRetentionDays: () => delay(undefined),
  ListDrafts: () => delay([...drafts.values()].map((d) => ({ ...clone(d.draft), eventCount: d.events?.length ?? 0, attachmentCount: d.attachments.length }))),
  NewDraft: () => delay(clone(newDraft().draft)),
  GetDraft: (i: string) => delay(clone(drafts.get(i)!)),
  SaveDraftText: (i: string, t: any) => { Object.assign(drafts.get(i)!.draft, t, { updatedAt: now() }); return delay(undefined, 40); },
  DeleteDraft: (i: string) => { drafts.delete(i); return delay(undefined); },
  AddNote: (i: string, text: string) => { const n: Note = { id: id(), at: now(), text }; drafts.get(i)!.notes.push(n); return delay(n); },
  DeleteNote: (i: string, n: string) => { const d = drafts.get(i)!; d.notes = d.notes.filter((x) => x.id !== n); return delay(undefined); },
  MarkMoment: (i: string) => { drafts.get(i)!.events!.push({ seq: ++seq, ts: now(), kind: "marker", level: "warn", source: "you", msg: "The problem happened here" }); return delay(undefined); },
  TakeScreenshot: (i: string, mode: string) => { const a: DraftAttachment = { id: id(), mediaType: "image/png", role: "screenshot", title: mode === "window" ? "Window screenshot" : "Area screenshot", createdAt: now(), width: 640, height: 400, durationMs: 0, annotated: false, reviewed: false, note: "", size: 5120, url: shot }; drafts.get(i)!.attachments.push(a); return delay(a, 600); },
  StartRecording: (i: string) => { drafts.get(i)!.recording = true; emit("recording", { draft: i, state: "started", at: now() }); return delay(undefined); },
  StopRecording: () => { for (const d of drafts.values()) if (d.recording) { d.recording = false; emit("recording", { draft: d.draft.id, state: "stopped" }); } return delay(null as any); },
  AddFiles: () => delay([]),
  RemoveAttachment: (i: string, a: string) => { const d = drafts.get(i)!; d.attachments = d.attachments.filter((x) => x.id !== a); return delay(undefined); },
  SetAttachmentNote: (i: string, a: string, n: string) => { drafts.get(i)!.attachments.find((x) => x.id === a)!.note = n; return delay(undefined); },
  SetReviewed: (i: string, a: string, v: boolean) => { drafts.get(i)!.attachments.find((x) => x.id === a)!.reviewed = v; return delay(undefined); },
  SaveMarkup: (i: string, a: string, png: string) => { const at = drafts.get(i)!.attachments.find((x) => x.id === a)!; Object.assign(at, { url: png, annotated: true, reviewed: true }); return delay(clone(at)); },
  ListProcesses: () => delay([{ pid: 4121, name: "Ledgerly", app: true }, { pid: 512, name: "Mail", app: true }, { pid: 733, name: "Notes", app: true }, { pid: 901, name: "node", app: false }]),
  WatchProcess: (i: string, pid: number, name: string) => { const d = drafts.get(i)!; Object.assign(d.draft, { targetPid: pid, targetName: name }); d.watching = true; clearInterval(watchTimer); watchTimer = setInterval(() => { const cpu = 10 + Math.random() * 30, rss = 3.2e8 + Math.random() * 2e7; d.events!.push({ seq: ++seq, ts: now(), kind: "metric", source: name, metric: { name: "cpu", value: cpu, unit: "%" } }); emit("sample", { draft: i, cpu, rss, at: now() }); }, 1000); return delay(undefined); },
  StopWatching: () => { clearInterval(watchTimer); for (const d of drafts.values()) d.watching = false; return delay(undefined); },
  Review: (i: string) => { const d = drafts.get(i)!; return delay({ policy: "default", findings: [{ key: "events[364].msg#0", field: "events[364].msg", label: "Event message", ruleId: "email", rule: "email addresses", preview: "ma•••••••••••om", context: "…did not respond in 10s; user=⟦ma•••••••••••om⟧" }, { key: "steps[0]#0", field: "steps[0]", label: "Step 1", ruleId: "home-path", rule: "account names in home directory paths", preview: "ma•a", context: "Open /Users/⟦ma•a⟧/Invoices/INV-2041.ledger" }], media: clone(d.attachments.filter((a) => /^(image|video)\//.test(a.mediaType))), unreviewed: d.attachments.filter((a) => !a.reviewed && /^(image|video)\//.test(a.mediaType)).length }); },
  Export: (i: string, keep: string[]) => { const d = drafts.get(i)!; d.draft.exportedPath = "/Users/you/Desktop/" + (d.draft.title || "bug-report") + ".repropack"; return delay({ path: d.draft.exportedPath, htmlPath: d.draft.exportedPath.replace(/\.repropack$/, ".html"), size: 48213, digest: "sha256:9c2f4e1b7a0d5c3e8f6b2a1d4c7e9f0a3b5d8c1e2f4a6b8d0c2e4f6a8b0d2c4e", removed: 2 - keep.length, kept: keep.length, verified: keep.length === 0, events: d.events!.length, attachments: d.attachments.length }, 900); },
  RevealFile: () => delay(undefined),
  IssueLink: (_p: string, host: string, repo: string) => delay(`https://${host === "gitlab" ? "gitlab.com" : "github.com"}/${repo}/issues/new?title=...`),
  OpenURL: (u: string) => { window.open(u, "_blank", "noopener"); return delay(undefined); },
  OpenReportDialog: () => mock.OpenReport("~/Downloads/checkout-crash.repropack"),
  OpenReport: (p: string) => delay({ token: "t1", path: p, fileSize: 8067, summary: clone(fixture), mediaBase: shot + "?" }),
  CloseReport: () => delay(undefined),
  ReadTextAttachment: () => delay("09:41:03.460 INFO  POST /v1/payments start\n09:41:05.210 WARN  provider slow, retry 1\n09:41:06.980 ERROR 502 card declined by gateway\n"),
  ExportReportHTML: () => delay("/Users/you/Desktop/checkout-crash.html"),
  Recents: () => delay([{ path: "/Users/you/Downloads/checkout-crash.repropack", title: "Checkout crashes after a declined card", openedAt: now(), exists: true }]),
  ForgetRecent: () => delay(undefined),
};
export const devShot = shot;
