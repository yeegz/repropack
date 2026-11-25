<script lang="ts">
  // The evidence tape: one lane per kind of thing that happened, drawn on a
  // shared time axis, with screenshots and notes pinned above. Click or
  // drag to focus a time window; arrow keys move a cursor, Enter focuses
  // the window around it, Escape clears.
  import type { RPEvent } from "../lib/types";
  import { shortRel, metricValue, plural } from "../lib/fmt";

  export interface Pin { id: string; at: string; label: string; kind: "shot" | "video" | "note" | "marker" }
  interface Props {
    start: string;
    end: string;
    events: RPEvent[];
    pins?: Pin[];
    selection?: [number, number] | null; // fractions 0..1
    live?: boolean;
    compact?: boolean;
    onselect?: (sel: [number, number] | null) => void;
    onpin?: (id: string) => void;
  }
  let { start, end, events, pins = [], selection = null, live = false, compact = false, onselect, onpin }: Props = $props();

  const BUCKETS = 120;
  const descId = "tape-desc-" + Math.random().toString(36).slice(2, 8);
  const LANES = [
    { key: "crash", label: "Crashes" },
    { key: "error", label: "Errors" },
    { key: "warn", label: "Warnings" },
    { key: "marker", label: "Your marks" },
    { key: "action", label: "Actions" },
    { key: "log", label: "Logs" },
    { key: "span", label: "Requests" },
  ];

  let width = $state(800);
  const labelW = $derived(compact ? 0 : 112);
  const trackW = $derived(Math.max(100, width - labelW - 8));
  const t0 = $derived(Date.parse(start));
  const span = $derived(Math.max(1, Date.parse(end) - t0));

  function laneOf(e: RPEvent): string | null {
    if (e.kind === "metric") return null;
    if (e.kind === "crash") return "crash";
    if (e.kind === "error" || e.level === "error" || e.level === "fatal") return "error";
    if (e.kind === "marker") return e.source === "you" ? "marker" : null;
    if (e.level === "warn") return "warn";
    if (e.kind === "breadcrumb") return "action";
    if (e.kind === "span") return "span";
    return "log";
  }

  const lanes = $derived.by(() => {
    const m = new Map<string, number[]>();
    for (const e of events) {
      const k = laneOf(e);
      if (!k) continue;
      let b = m.get(k);
      if (!b) m.set(k, (b = new Array(BUCKETS).fill(0)));
      const i = Math.min(BUCKETS - 1, Math.max(0, Math.floor(((Date.parse(e.ts) - t0) / span) * BUCKETS)));
      b[i]++;
    }
    return LANES.filter((l) => m.has(l.key)).map((l) => {
      const b = m.get(l.key)!;
      return { ...l, buckets: b, max: Math.max(...b), total: b.reduce((a, c) => a + c, 0) };
    });
  });

  const metrics = $derived.by(() => {
    const m = new Map<string, { name: string; unit?: string; pts: [number, number][]; min: number; max: number; last: number }>();
    for (const e of events) {
      if (e.kind !== "metric" || !e.metric) continue;
      const key = e.metric.name;
      let s = m.get(key);
      if (!s) m.set(key, (s = { name: key, unit: e.metric.unit, pts: [], min: Infinity, max: -Infinity, last: 0 }));
      if (s.pts.length < 4000) s.pts.push([Date.parse(e.ts), e.metric.value]);
      s.min = Math.min(s.min, e.metric.value);
      s.max = Math.max(s.max, e.metric.value);
      s.last = e.metric.value;
    }
    return [...m.values()].slice(0, 4);
  });

  const LANE_H = $derived(compact ? 18 : 26);
  const METRIC_H = $derived(compact ? 28 : 40);
  const PIN_H = 26;
  const height = $derived((pins.length ? PIN_H : 6) + lanes.length * LANE_H + metrics.length * METRIC_H + 24);

  // Short recordings get decimal seconds so tick labels never repeat.
  function axisLabel(ms: number): string {
    if (span < 20_000) return (ms / 1000).toFixed(span < 5000 ? 2 : 1) + " s";
    const m = Math.floor(ms / 60000), sec = Math.floor((ms % 60000) / 1000);
    return `${m}:${String(sec).padStart(2, "0")}`;
  }
  const x = (frac: number) => labelW + frac * trackW;
  const fracOf = (iso: string) => Math.min(1, Math.max(0, (Date.parse(iso) - t0) / span));

  function metricPath(s: (typeof metrics)[number], y: number): string {
    const h = METRIC_H - 12;
    const rng = s.max - s.min || 1;
    let d = "";
    for (let i = 0; i < s.pts.length; i++) {
      const [t, v] = s.pts[i];
      const px = x(Math.min(1, Math.max(0, (t - t0) / span)));
      const py = y + 4 + h - ((v - s.min) / rng) * h;
      d += (i ? "L" : "M") + px.toFixed(1) + " " + py.toFixed(1);
    }
    return d;
  }

  // Interaction
  let svgEl: SVGSVGElement | undefined = $state();
  let drag = $state<{ from: number; to: number } | null>(null);
  let cursor = $state(-1); // fraction, keyboard cursor

  function fracAt(ev: PointerEvent): number {
    const r = svgEl!.getBoundingClientRect();
    return Math.min(1, Math.max(0, (ev.clientX - r.left - labelW) / trackW));
  }
  function down(ev: PointerEvent) {
    if (!onselect || ev.button !== 0) return;
    const f = fracAt(ev);
    drag = { from: f, to: f };
    (ev.currentTarget as Element).setPointerCapture(ev.pointerId);
  }
  function move(ev: PointerEvent) {
    if (drag) drag = { ...drag, to: fracAt(ev) };
  }
  function up() {
    if (!drag) return;
    let [a, b] = [Math.min(drag.from, drag.to), Math.max(drag.from, drag.to)];
    if (b - a < 0.01) [a, b] = [Math.max(0, a - 0.04), Math.min(1, a + 0.04)];
    drag = null;
    onselect?.([a, b]);
  }
  function key(ev: KeyboardEvent) {
    if (!onselect) return;
    const step = ev.shiftKey ? 0.1 : 1 / BUCKETS;
    if (ev.key === "ArrowRight" || ev.key === "ArrowLeft") {
      ev.preventDefault();
      cursor = Math.min(1, Math.max(0, (cursor < 0 ? 0 : cursor) + (ev.key === "ArrowRight" ? step : -step)));
    } else if (ev.key === "Home") { ev.preventDefault(); cursor = 0; }
    else if (ev.key === "End") { ev.preventDefault(); cursor = 1; }
    else if (ev.key === "Enter" && cursor >= 0) { ev.preventDefault(); onselect([Math.max(0, cursor - 0.04), Math.min(1, cursor + 0.04)]); }
    else if (ev.key === "Escape") { cursor = -1; onselect(null); }
  }
  const cursorText = $derived.by(() => {
    if (cursor < 0) return "Use arrow keys to move through time";
    const b = Math.min(BUCKETS - 1, Math.floor(cursor * BUCKETS));
    const parts = lanes.filter((l) => l.buckets[b] > 0).map((l) => plural(l.buckets[b], l.label.toLowerCase().replace(/s$/, ""), l.label.toLowerCase()));
    const at = new Date(t0 + cursor * span).toISOString();
    return `${shortRel(start, at)}${parts.length ? ", " + parts.join(", ") : ", nothing recorded"}. Press Enter to focus here.`;
  });
  const description = $derived(
    `Timeline from ${shortRel(start, start)} to ${shortRel(start, end)}. ` +
      lanes.map((l) => `${l.label}: ${l.total}`).join("; ") +
      (metrics.length ? `. Measurements: ${metrics.map((m) => `${m.name} from ${metricValue(m.min, m.unit)} to ${metricValue(m.max, m.unit)}`).join("; ")}` : "") +
      (pins.length ? `. ${plural(pins.length, "pinned item", "pinned items")}.` : ""),
  );
  const ticks = $derived.by(() => {
    const n = Math.max(2, Math.min(8, Math.floor(trackW / 110)));
    return Array.from({ length: n + 1 }, (_, i) => i / n);
  });
  const sel = $derived(drag ? [Math.min(drag.from, drag.to), Math.max(drag.from, drag.to)] : selection);
</script>

<div class="tape" class:compact bind:clientWidth={width}>
  {#if onselect}<p id={descId} class="vh">{description}</p>{/if}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <svg bind:this={svgEl} {width} {height} viewBox="0 0 {width} {height}"
    role={onselect ? "slider" : "img"}
    tabindex={onselect ? 0 : undefined}
    aria-label={onselect ? "Timeline" : description}
    aria-describedby={onselect ? descId : undefined}
    aria-valuemin={onselect ? 0 : undefined} aria-valuemax={onselect ? 100 : undefined}
    aria-valuenow={onselect ? Math.round(Math.max(0, cursor) * 100) : undefined}
    aria-valuetext={onselect ? cursorText : undefined}
    onpointerdown={down} onpointermove={move} onpointerup={up} onkeydown={key}>
    {#if sel}
      <rect class="sel" x={x(sel[0])} y="0" width={Math.max(2, (sel[1] - sel[0]) * trackW)} height={height - 18} rx="4" />
    {/if}
    {#each ticks as t}
      <line class="tick" x1={x(t)} x2={x(t)} y1={pins.length ? PIN_H - 4 : 2} y2={height - 20} />
      <text class="axis" x={x(t)} y={height - 5} text-anchor={t === 0 ? "start" : t === 1 ? "end" : "middle"}>{axisLabel(t * span)}</text>
    {/each}
    {#if pins.length && !compact}<text class="lab" x="0" y="17">Evidence</text>{/if}
    {#each lanes as l, i}
      {@const y = (pins.length ? PIN_H : 6) + i * LANE_H}
      <g class="lane lane-{l.key}">
        <line class="track" x1={labelW} x2={labelW + trackW} y1={y + LANE_H / 2} y2={y + LANE_H / 2} />
        {#if !compact}<text class="lab" x="0" y={y + LANE_H / 2 + 4}>{l.label}<tspan class="cnt" dx="6">{l.total}</tspan></text>{/if}
        {#each l.buckets as c, b}
          {#if c > 0}
            {@const f = c / Math.max(1, l.max)}
            {@const h = (compact ? 6 : 8) + f * (LANE_H - (compact ? 8 : 10))}
            <rect x={x(b / BUCKETS) + 0.5} y={y + (LANE_H - h) / 2} width={Math.max(2, trackW / BUCKETS - 1.5)} height={h} rx="1.5" opacity={0.4 + 0.6 * f} />
          {/if}
        {/each}
      </g>
    {/each}
    {#each metrics as m, i}
      {@const y = (pins.length ? PIN_H : 6) + lanes.length * LANE_H + i * METRIC_H}
      <g class="metric">
        <line class="track" x1={labelW} x2={labelW + trackW} y1={y + METRIC_H - 6} y2={y + METRIC_H - 6} />
        {#if !compact}
          <text class="lab" x="0" y={y + 16}>{m.name}</text>
          <text class="val" x="0" y={y + 30}>{metricValue(m.last, m.unit)}</text>
        {/if}
        <path d={metricPath(m, y)} />
      </g>
    {/each}
    {#if cursor >= 0}
      <line class="cursor" x1={x(cursor)} x2={x(cursor)} y1="0" y2={height - 18} />
    {/if}
    {#if live}
      <line class="now" x1={labelW + trackW} x2={labelW + trackW} y1="0" y2={height - 18} />
    {/if}
  </svg>
  {#each pins as p (p.id)}
    {#if !onpin}
      <span class="pin pin-{p.kind}" style="left:{x(fracOf(p.at)) - 8}px" aria-hidden="true"><span></span></span>
    {:else}
    <button class="pin pin-{p.kind}" style="left:{x(fracOf(p.at)) - 8}px" title={p.label} aria-label="{p.label} at {shortRel(start, p.at)}"
      onclick={() => onpin?.(p.id)}>
      <span aria-hidden="true"></span>
    </button>
    {/if}
  {/each}
</div>

<style>
  .tape { position: relative; width: 100%; }
  svg { display: block; overflow: visible; touch-action: none; }
  svg[role="slider"] { cursor: crosshair; border-radius: var(--r-sm); }
  .lab { font-size: 12px; font-weight: 600; fill: var(--ink-2); }
  .cnt { fill: var(--ink-3); font-weight: 500; font-variant-numeric: tabular-nums; }
  .val { font-size: 11.5px; fill: var(--ink-3); font-family: var(--mono); }
  .axis { font-size: 10.5px; fill: var(--ink-3); font-family: var(--mono); }
  .tick { stroke: var(--rule); stroke-dasharray: 2 4; }
  .track { stroke: var(--rule); }
  .lane rect { fill: var(--info); }
  .lane-error rect { fill: var(--err); }
  .lane-crash rect { fill: var(--crash); }
  .lane-warn rect { fill: var(--warn); }
  .lane-marker rect { fill: var(--seal); }
  .lane-action rect { fill: var(--seal); opacity: 0.75; }
  .lane-span rect { fill: var(--span); }
  .metric path { fill: none; stroke: var(--seal); stroke-width: 1.6; stroke-linejoin: round; }
  .sel { fill: var(--seal-soft); stroke: var(--seal); stroke-width: 1; }
  .cursor { stroke: var(--ink); stroke-width: 1.5; }
  .now { stroke: var(--rec); stroke-width: 2; stroke-dasharray: 3 3; }
  .pin {
    position: absolute; top: 2px; width: 16px; height: 20px; padding: 0; border: 0; background: none;
    display: grid; place-items: center;
  }
  .pin span { width: 10px; height: 10px; transform: rotate(45deg); border-radius: 2px; background: var(--seal); box-shadow: 0 0 0 2px var(--surface); transition: transform var(--t-fast) var(--ease); }
  .pin-note span { background: var(--ink-2); border-radius: 50% 50% 50% 2px; transform: rotate(-45deg); }
  .pin-video span { background: var(--span); }
  .pin:hover span, .pin:focus-visible span { transform: rotate(45deg) scale(1.35); }
  .pin-note:hover span { transform: rotate(-45deg) scale(1.35); }
  @media (forced-colors: active) {
    .lane rect, .pin span { fill: CanvasText; background: CanvasText; }
    .metric path { stroke: CanvasText; }
  }
</style>
