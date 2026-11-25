// Plain JSON shapes returned by the Go backend.
export type Level = "trace" | "debug" | "info" | "warn" | "error" | "fatal" | "";

export interface ErrorInfo { type?: string; message: string; stack?: Frame[]; cause?: ErrorInfo; raw_stack?: string; handled?: boolean }
export interface Frame { function?: string; file?: string; line?: number; module?: string }
export interface Metric { name: string; value: number; unit?: string }
export interface Span { name: string; trace_id?: string; span_id?: string; parent_span_id?: string; start: string; end: string; status?: string }
export interface RPEvent {
  seq: number; ts: string; kind: string; level?: Level; source?: string; msg?: string;
  attrs?: Record<string, unknown>; trace_id?: string; span_id?: string;
  error?: ErrorInfo; metric?: Metric; span?: Span;
}
export interface Attachment {
  id: string; path: string; media_type: string; sha256: string; size: number; role: string;
  title?: string; created_at: string; width?: number; height?: number; duration_ms?: number; annotated?: boolean;
}
export interface Annotation { id: string; text: string; at?: string; attachment_id?: string; author?: string }
export interface Manifest {
  format: string; spec_version: string; report_id: string; created_at: string; title: string; description?: string;
  producer: { name: string; version: string; sdk?: string };
  app?: { name: string; version?: string; build?: string; environment?: string; runtime?: string; pid?: number };
  os?: { name: string; version?: string; build?: string; arch?: string; kernel?: string; locale?: string; timezone?: string };
  device?: { model?: string; cpu?: string; cpu_cores?: number; memory_bytes?: number };
  reproduction?: { steps?: string[]; expected?: string; actual?: string; frequency?: string };
  context?: Record<string, unknown>;
  annotations?: Annotation[];
  events: { path: string; sha256: string; size: number; count: number; first?: string; last?: string; dropped?: number };
  attachments?: Attachment[];
  redaction?: { policy: string; rules?: { id: string; description?: string; matches: number }[]; manual?: number; verified: boolean; verified_at?: string; reviewed_attachments?: string[] };
  content_digest: string;
}
export interface Lane { name: string; label: string; buckets: number[]; max: number; total: number }
export interface MetricSeries { name: string; unit: string; points: { t: string; v: number }[]; min: number; max: number; last: number }
export interface Summary {
  manifest: Manifest; integrity: string; verified: boolean; start: string; end: string; duration: number;
  by_kind: Record<string, number>; by_level: Record<string, number>; headline: string; first_error?: RPEvent;
  errors: number; lanes: Lane[] | null; metrics: MetricSeries[] | null; unreviewed_media: string[] | null;
  events: RPEvent[] | null; truncated: boolean; total_events: number;
}
export interface ReportView { token: string; path: string; fileSize: number; summary: Summary; mediaBase: string }

export interface Draft {
  id: string; createdAt: string; updatedAt: string; title: string; description: string; steps: string[];
  expected: string; actual: string; frequency: string; targetPid: number; targetName: string;
  manualRedactions: number; exportedPath: string; exportedAt: string; eventCount: number; attachmentCount: number;
}
export interface DraftAttachment {
  id: string; mediaType: string; role: string; title: string; createdAt: string; width: number; height: number;
  durationMs: number; annotated: boolean; reviewed: boolean; note: string; size: number; url: string;
}
export interface Note { id: string; at: string; text: string }
export interface DraftDetail { draft: Draft; attachments: DraftAttachment[]; notes: Note[]; events: RPEvent[] | null; watching: boolean; recording: boolean }
export interface DraftText { title: string; description: string; steps: string[]; expected: string; actual: string; frequency: string }
export interface Capabilities { os: string; screenshots: boolean; recording: boolean; processList: boolean; note: string }
export interface Environment {
  capabilities: Capabilities; version: string; osName: string; osVersion: string; arch: string; model: string; cpu: string;
  memoryBytes: number; dataDir: string; retentionDays: number; issueRepo: string; issueHost: string;
}
export interface Process { pid: number; name: string; app: boolean }
export interface ReviewFinding { key: string; field: string; label: string; ruleId: string; rule: string; preview: string; context: string }
export interface Review { findings: ReviewFinding[]; media: DraftAttachment[]; unreviewed: number; policy: string }
export interface ExportResult {
  path: string; htmlPath?: string; size: number; digest: string; removed: number; kept: number; verified: boolean;
  events: number; attachments: number;
}
export interface Recent { path: string; title: string; openedAt: string; exists: boolean }
