import type { Environment } from "./types";

export type Route =
  | { name: "home" }
  | { name: "draft"; id: string; step?: number }
  | { name: "report"; token: string }
  | { name: "settings" };

export interface Toast { id: number; text: string; tone: "ok" | "error" | "info"; action?: { label: string; run: () => void } }

class AppState {
  route = $state<Route>({ name: "home" });
  env = $state<Environment | null>(null);
  toasts = $state<Toast[]>([]);
  libraryVersion = $state(0); // bump to refresh the sidebar
  recordingDraft = $state<string | null>(null);
  recordingSince = $state<number>(0);
  private n = 0;

  go(r: Route) { this.route = r; }
  refreshLibrary() { this.libraryVersion++; }
  toast(text: string, tone: Toast["tone"] = "info", action?: Toast["action"]) {
    const t = { id: ++this.n, text, tone, action };
    this.toasts = [...this.toasts, t];
    setTimeout(() => this.dismiss(t.id), tone === "error" ? 9000 : 5000);
  }
  dismiss(id: number) { this.toasts = this.toasts.filter((t) => t.id !== id); }
  error(e: unknown) { this.toast((e as Error)?.message ?? String(e), "error"); }
}

export const app = new AppState();
