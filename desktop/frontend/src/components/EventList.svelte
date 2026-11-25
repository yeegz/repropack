<script lang="ts">
  // Windowed, keyboard-navigable event list (listbox pattern). Handles tens
  // of thousands of rows by rendering only what is visible.
  import type { RPEvent } from "../lib/types";
  import { rel, kindLabel, metricValue } from "../lib/fmt";

  let { events, start, selected = $bindable<number | null>(null) }: { events: RPEvent[]; start: string; selected?: number | null } = $props();
  const ROW = 34;
  let box: HTMLDivElement | undefined = $state();
  let scrollTop = $state(0);
  let viewH = $state(400);
  const first = $derived(Math.max(0, Math.floor(scrollTop / ROW) - 8));
  const last = $derived(Math.min(events.length, Math.ceil((scrollTop + viewH) / ROW) + 8));
  const idx = $derived(selected === null ? -1 : events.findIndex((e) => e.seq === selected));

  function text(e: RPEvent): string {
    if (e.msg) return e.msg.split("\n")[0];
    if (e.error) return e.error.message;
    if (e.metric) return `${e.metric.name} = ${metricValue(e.metric.value, e.metric.unit)}`;
    if (e.span) return e.span.name;
    return "";
  }
  function sel(i: number) {
    if (!events.length) return;
    i = Math.max(0, Math.min(events.length - 1, i));
    selected = events[i].seq;
    const top = i * ROW;
    if (box) {
      if (top < box.scrollTop) box.scrollTop = top;
      else if (top + ROW > box.scrollTop + viewH) box.scrollTop = top + ROW - viewH;
    }
  }
  function key(e: KeyboardEvent) {
    const page = Math.max(1, Math.floor(viewH / ROW) - 1);
    const m: Record<string, number> = { ArrowDown: 1, ArrowUp: -1, PageDown: page, PageUp: -page };
    if (e.key in m) { e.preventDefault(); sel((idx < 0 ? -1 : idx) + m[e.key]); }
    else if (e.key === "Home") { e.preventDefault(); sel(0); }
    else if (e.key === "End") { e.preventDefault(); sel(events.length - 1); }
  }
  // Keep the selection visible when the list changes (e.g. filtering).
  $effect(() => { events; if (idx >= 0) queueMicrotask(() => sel(idx)); });
  const level = (e: RPEvent) => e.kind === "crash" ? "crash" : (e.level || (e.kind === "error" ? "error" : ""));
</script>

<div class="list scroll" bind:this={box} bind:clientHeight={viewH} onscroll={() => (scrollTop = box!.scrollTop)}
  role="listbox" tabindex="0" aria-label="Events, {events.length}" aria-activedescendant={idx >= 0 ? `ev-${events[idx].seq}` : undefined} onkeydown={key}>
  <div style="height:{events.length * ROW}px; position:relative">
    {#each events.slice(first, last) as e, i (e.seq)}
      {@const lv = level(e)}
      <!-- svelte-ignore a11y_click_events_have_key_events -->
      <div id="ev-{e.seq}" role="option" tabindex="-1" aria-selected={e.seq === selected} class="row lv-{lv} k-{e.kind}" class:sel={e.seq === selected}
        style="top:{(first + i) * ROW}px" onclick={() => sel(first + i)}>
        <span class="t mono">{rel(start, e.ts)}</span>
        <span class="lvl">{lv || kindLabel(e.kind)}</span>
        <span class="msg">{text(e)}</span>
        {#if e.source}<span class="src">{e.source}</span>{/if}
      </div>
    {/each}
  </div>
  {#if !events.length}<p class="none">No events match. Clear the filters or the time selection.</p>{/if}
</div>

<style>
  .list { position: relative; height: 100%; outline-offset: -2px; }
  .row { position: absolute; left: 0; right: 0; height: 34px; display: grid; grid-template-columns: 96px 64px 1fr auto; gap: 10px; align-items: center; padding: 0 14px; border-bottom: 1px solid var(--rule); font-size: 13px; }
  .row:hover { background: color-mix(in oklab, var(--ink) 3%, transparent); }
  .row.sel { background: var(--seal-soft); box-shadow: inset 3px 0 0 var(--seal); }
  .t { color: var(--ink-3); font-size: 12px; }
  .lvl { font-size: 11.5px; font-weight: 650; color: var(--ink-3); text-transform: lowercase; }
  .lv-warn .lvl { color: var(--warn); }
  .lv-error .lvl, .lv-fatal .lvl { color: var(--err); }
  .lv-crash .lvl { color: var(--crash); }
  .lv-error, .lv-fatal { background: color-mix(in oklab, var(--err-soft) 50%, transparent); }
  .lv-crash { background: color-mix(in oklab, var(--crash-soft) 70%, transparent); }
  .k-metric .msg { color: var(--ink-3); }
  .msg { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .src { font-size: 12px; color: var(--ink-3); max-width: 140px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .none { position: absolute; top: 24px; left: 0; right: 0; text-align: center; color: var(--ink-3); }
</style>
