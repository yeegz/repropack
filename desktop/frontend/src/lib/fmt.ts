export function rel(startISO: string, tISO: string): string {
  let d = Date.parse(tISO) - Date.parse(startISO);
  const neg = d < 0;
  if (neg) d = -d;
  const m = Math.floor(d / 60000);
  const s = (d % 60000) / 1000;
  const body = m >= 60 ? `${Math.floor(m / 60)}:${String(m % 60).padStart(2, "0")}:${s.toFixed(3).padStart(6, "0")}` : `${m}:${s.toFixed(3).padStart(6, "0")}`;
  return (neg ? "−" : "") + body;
}

export function shortRel(startISO: string, tISO: string): string {
  const d = Math.max(0, Date.parse(tISO) - Date.parse(startISO));
  const m = Math.floor(d / 60000), s = Math.floor((d % 60000) / 1000);
  return `${m}:${String(s).padStart(2, "0")}`;
}

/** Go durations arrive as nanoseconds. */
export function duration(ns: number): string {
  const ms = ns / 1e6;
  if (ms < 1000) return `${Math.round(ms)} ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${Math.round(s % 60)} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

export function metricValue(v: number, unit?: string): string {
  if (unit === "By") return bytes(v);
  if (unit === "%") return `${v.toFixed(v < 10 ? 1 : 0)}%`;
  if (unit === "ms") return `${Math.round(v)} ms`;
  const n = Number.isInteger(v) ? String(v) : v.toFixed(2);
  return unit && !unit.startsWith("{") ? `${n} ${unit}` : n;
}

const dtf = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
const tf = new Intl.DateTimeFormat(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
export const dateTime = (iso: string) => (iso ? dtf.format(new Date(iso)) : "");
export const clock = (iso: string) => tf.format(new Date(iso));

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
export function ago(iso: string): string {
  const d = (Date.parse(iso) - Date.now()) / 1000;
  const a = Math.abs(d);
  if (a < 60) return "just now";
  if (a < 3600) return rtf.format(Math.round(d / 60), "minute");
  if (a < 86400) return rtf.format(Math.round(d / 3600), "hour");
  if (a < 86400 * 7) return rtf.format(Math.round(d / 86400), "day");
  return dtf.format(new Date(iso));
}

export const osName = (n?: string) => ({ darwin: "macOS", windows: "Windows", linux: "Linux" } as Record<string, string>)[n ?? ""] ?? n ?? "";

export function kindLabel(k: string): string {
  return ({ log: "Log", error: "Error", crash: "Crash", metric: "Metric", span: "Span", breadcrumb: "Action", marker: "Marker" } as Record<string, string>)[k] ?? "Custom";
}

export function plural(n: number, one: string, many: string) {
  return `${n} ${n === 1 ? one : many}`;
}

export function valueText(v: unknown): string {
  if (typeof v === "string") return v;
  if (v === null || v === undefined) return "null";
  if (typeof v === "number") return Number.isInteger(v) ? String(v) : String(+v.toFixed(4));
  return JSON.stringify(v);
}
