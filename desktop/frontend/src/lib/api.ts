// Thin, typed wrapper over the Go bindings. In a plain browser (npm run dev
// without Wails) a mock backend is loaded so the interface can be previewed.
import type {
  Draft, DraftAttachment, DraftDetail, DraftText, Environment, ExportResult, Note, Process, Recent, Review, ReportView,
} from "./types";

type Backend = Record<string, (...a: any[]) => Promise<any>>;
let backend: Backend | null = null;

export const isDesktop = typeof (window as any).go?.main?.App === "object";

async function be(): Promise<Backend> {
  if (backend) return backend;
  if (isDesktop) backend = (window as any).go.main.App as Backend;
  else if (import.meta.env.DEV) backend = (await import("../dev/mock")).mock as Backend;
  else throw new Error("ReproPack backend is not available");
  return backend!;
}

/** Normalises errors from Go (plain strings) into Error objects. */
async function call<T>(name: string, ...args: unknown[]): Promise<T> {
  const b = await be();
  try {
    return (await b[name](...args)) as T;
  } catch (e) {
    throw new Error(typeof e === "string" ? e : (e as Error)?.message ?? String(e));
  }
}

export const api = {
  environment: () => call<Environment>("Environment"),
  setRetentionDays: (d: number) => call<void>("SetRetentionDays", d),
  listDrafts: () => call<Draft[]>("ListDrafts"),
  newDraft: () => call<Draft>("NewDraft"),
  getDraft: (id: string) => call<DraftDetail>("GetDraft", id),
  saveDraftText: (id: string, t: DraftText) => call<void>("SaveDraftText", id, t),
  deleteDraft: (id: string) => call<void>("DeleteDraft", id),
  addNote: (id: string, text: string) => call<Note>("AddNote", id, text),
  deleteNote: (id: string, noteId: string) => call<void>("DeleteNote", id, noteId),
  markMoment: (id: string, label = "") => call<void>("MarkMoment", id, label),
  takeScreenshot: (id: string, mode: "window" | "area") => call<DraftAttachment>("TakeScreenshot", id, mode),
  startRecording: (id: string) => call<void>("StartRecording", id),
  stopRecording: () => call<DraftAttachment>("StopRecording"),
  addFiles: (id: string) => call<DraftAttachment[] | null>("AddFiles", id),
  removeAttachment: (id: string, attId: string) => call<void>("RemoveAttachment", id, attId),
  setAttachmentNote: (id: string, attId: string, note: string) => call<void>("SetAttachmentNote", id, attId, note),
  setReviewed: (id: string, attId: string, v: boolean) => call<void>("SetReviewed", id, attId, v),
  saveMarkup: (id: string, attId: string, png: string, redactions: number) => call<DraftAttachment>("SaveMarkup", id, attId, png, redactions),
  listProcesses: () => call<Process[]>("ListProcesses"),
  watchProcess: (id: string, pid: number, name: string) => call<void>("WatchProcess", id, pid, name),
  stopWatching: () => call<void>("StopWatching"),
  review: (id: string) => call<Review>("Review", id),
  exportDraft: (id: string, keep: string[], html: boolean) => call<ExportResult | null>("Export", id, keep, html),
  revealFile: (p: string) => call<void>("RevealFile", p),
  issueLink: (path: string, host: string, repo: string) => call<string>("IssueLink", path, host, repo),
  openURL: (u: string) => call<void>("OpenURL", u),
  openReportDialog: () => call<ReportView | null>("OpenReportDialog"),
  openReport: (p: string) => call<ReportView>("OpenReport", p),
  closeReport: (t: string) => call<void>("CloseReport", t),
  readTextAttachment: (t: string, id: string) => call<string>("ReadTextAttachment", t, id),
  exportReportHTML: (t: string) => call<string>("ExportReportHTML", t),
  recents: () => call<Recent[]>("Recents"),
  forgetRecent: (p: string) => call<void>("ForgetRecent", p),
};

/** Subscribe to a backend event. Returns an unsubscribe function. */
export function on(name: string, fn: (data: any) => void): () => void {
  const rt = (window as any).runtime;
  if (rt?.EventsOn) return rt.EventsOn(name, fn);
  const h = (e: Event) => fn((e as CustomEvent).detail);
  window.addEventListener("rp:" + name, h);
  return () => window.removeEventListener("rp:" + name, h);
}
