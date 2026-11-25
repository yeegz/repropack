<script lang="ts">
  import type { ErrorInfo, RPEvent } from "../lib/types";
  import { rel, kindLabel, metricValue, valueText, duration } from "../lib/fmt";

  let { event, start }: { event: RPEvent | null; start: string } = $props();
  function chain(e?: ErrorInfo): ErrorInfo[] { const out: ErrorInfo[] = []; let c = e; while (c && out.length < 32) { out.push(c); c = c.cause; } return out; }
  const attrs = $derived(Object.entries(event?.attrs ?? {}).sort(([a], [b]) => a.localeCompare(b)));
</script>

<aside class="detail scroll" aria-label="Event details" aria-live="polite">
  {#if !event}
    <p class="none">Select an event to see everything recorded with it.</p>
  {:else}
    <div class="head">
      <span class="k">{kindLabel(event.kind)}{event.level ? `, ${event.level}` : ""}</span>
      <span class="mono t">{rel(start, event.ts)}</span>
    </div>
    {#if event.msg}<p class="msg selectable">{event.msg}</p>{/if}
    {#if event.metric}<p class="big">{event.metric.name} <strong>{metricValue(event.metric.value, event.metric.unit)}</strong></p>{/if}
    {#if event.span}
      <p class="big">{event.span.name} <strong>{duration((Date.parse(event.span.end) - Date.parse(event.span.start)) * 1e6)}</strong>{event.span.status ? `, status ${event.span.status}` : ""}</p>
    {/if}
    {#each chain(event.error) as er, i}
      <section class="err" class:cause={i > 0}>
        <h3>{i ? "Caused by" : event.error?.handled === false ? "Unhandled error" : "Error"}</h3>
        <p class="selectable"><strong>{er.type ? er.type + ": " : ""}</strong>{er.message}</p>
        {#if er.stack?.length}
          <ol class="stack">
            {#each er.stack as f}<li><span class="fn mono">{f.function}</span><span class="loc mono selectable">{f.file}{f.line ? `:${f.line}` : ""}</span></li>{/each}
          </ol>
        {:else if er.raw_stack}
          <pre class="raw selectable">{er.raw_stack}</pre>
        {/if}
      </section>
    {/each}
    {#if attrs.length}
      <h3>Details</h3>
      <dl class="attrs">
        {#each attrs as [k, v]}<div><dt class="mono">{k}</dt><dd class="mono selectable">{valueText(v)}</dd></div>{/each}
      </dl>
    {/if}
    <dl class="meta">
      {#if event.source}<div><dt>Source</dt><dd>{event.source}</dd></div>{/if}
      <div><dt>Time</dt><dd class="mono">{event.ts}</dd></div>
      <div><dt>Sequence</dt><dd class="mono">{event.seq}</dd></div>
      {#if event.trace_id}<div><dt>Trace</dt><dd class="mono selectable">{event.trace_id}</dd></div>{/if}
      {#if event.span_id}<div><dt>Span</dt><dd class="mono selectable">{event.span_id}</dd></div>{/if}
    </dl>
  {/if}
</aside>

<style>
  .detail { height: 100%; padding: 16px 18px; border-left: 1px solid var(--rule); background: var(--surface); display: flex; flex-direction: column; gap: 12px; }
  .none { color: var(--ink-3); font-size: 13.5px; margin-top: 12px; }
  .head { display: flex; justify-content: space-between; font-size: 12.5px; color: var(--ink-3); font-weight: 600; }
  .msg { font-size: 14.5px; line-height: 1.45; white-space: pre-wrap; overflow-wrap: anywhere; }
  .big { font-size: 14px; }
  h3 { font-size: 12.5px; color: var(--ink-3); margin-bottom: 4px; }
  .err { background: var(--err-soft); border-radius: var(--r-sm); padding: 10px 12px; }
  .err.cause { background: var(--sunk); }
  .err p { font-size: 13.5px; overflow-wrap: anywhere; }
  .stack { margin: 8px 0 0; padding-left: 18px; display: grid; gap: 4px; font-size: 12px; }
  .stack li { display: grid; }
  .fn { font-weight: 500; color: var(--ink); }
  .loc { color: var(--ink-3); overflow-wrap: anywhere; }
  .raw { font: 11.5px/1.45 var(--mono); white-space: pre-wrap; max-height: 280px; overflow: auto; margin: 6px 0 0; }
  dl { margin: 0; display: grid; gap: 4px; }
  dl div { display: grid; grid-template-columns: minmax(80px, max-content) 1fr; gap: 10px; font-size: 12.5px; }
  dt { color: var(--ink-3); }
  dd { margin: 0; overflow-wrap: anywhere; }
  .meta { margin-top: auto; padding-top: 10px; border-top: 1px solid var(--rule); }
</style>
