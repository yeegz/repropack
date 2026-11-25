<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { reports } from "../lib/reports";
  import { bytes, dateTime, duration, osName, rel, shortRel, valueText } from "../lib/fmt";
  import type { Attachment, RPEvent, ReportView } from "../lib/types";
  import Icon from "../components/Icon.svelte";
  import Tape, { type Pin } from "../components/Tape.svelte";
  import EventList from "../components/EventList.svelte";
  import EventDetail from "../components/EventDetail.svelte";

  let { token }: { token: string } = $props();
  const view = $derived<ReportView | null>(reports.get(token) ?? null);
  const s = $derived(view?.summary);
  const m = $derived(s?.manifest);
  type Tab = "overview" | "events" | "evidence" | "details";
  let tab = $state<Tab>("overview");
  let selection = $state<[number, number] | null>(null);
  let selected = $state<number | null>(null);
  let q = $state("");
  let levels = $state<Record<string, boolean>>({ trace: true, debug: true, info: true, warn: true, error: true, fatal: true });
  let problemsOnly = $state(false);
  let hideMetrics = $state(true);
  let texts = $state<Record<string, string>>({});
  let lightbox = $state<Attachment | null>(null);

  onMount(() => {
    if (!view) app.go({ name: "home" });
  });

  const all = $derived<RPEvent[]>(s?.events ?? []);
  const t0 = $derived(s ? Date.parse(s.start) : 0);
  const span = $derived(s ? Math.max(1, Date.parse(s.end) - t0) : 1);
  const filtered = $derived.by(() => {
    const text = q.trim().toLowerCase();
    return all.filter((e) => {
      if (hideMetrics && e.kind === "metric") return false;
      if (e.level && levels[e.level] === false) return false;
      if (problemsOnly && !(e.kind === "error" || e.kind === "crash" || e.level === "warn" || e.level === "error" || e.level === "fatal" || (e.kind === "marker" && e.source === "you"))) return false;
      if (selection) {
        const f = (Date.parse(e.ts) - t0) / span;
        if (f < selection[0] || f > selection[1]) return false;
      }
      if (text && !((e.msg ?? "") + " " + (e.source ?? "") + " " + (e.error?.message ?? "")).toLowerCase().includes(text)) return false;
      return true;
    });
  });
  const selEvent = $derived(all.find((e) => e.seq === selected) ?? null);

  const atts = $derived(m?.attachments ?? []);
  const notesFor = (id: string) => (m?.annotations ?? []).filter((a) => a.attachment_id === id);
  const freeNotes = $derived((m?.annotations ?? []).filter((a) => !a.attachment_id));
  const pins = $derived<Pin[]>([
    ...atts.filter((a) => a.role === "screenshot" || a.role === "recording").map((a) => ({ id: a.id, at: a.created_at, label: a.title || a.role, kind: (a.role === "recording" ? "video" : "shot") as Pin["kind"] })),
    ...freeNotes.filter((n) => n.at).map((n) => ({ id: "note:" + n.id, at: n.at!, label: n.text, kind: "note" as const })),
  ]);
  const media = (a: Attachment) => /^(image\/(png|jpeg|gif|webp)|video\/(mp4|quicktime|webm))$/.test(a.media_type);
  const isText = (a: Attachment) => /^(text\/plain|application\/json|application\/x-ndjson)/.test(a.media_type);
  const url = (a: Attachment) => view!.mediaBase + a.id;

  $effect(() => {
    if (tab === "evidence" && s?.verified) for (const a of atts) if (isText(a) && texts[a.id] === undefined) {
      texts[a.id] = "";
      api.readTextAttachment(token, a.id).then((t) => (texts[a.id] = t)).catch((e) => (texts[a.id] = "Could not read: " + (e as Error).message));
    }
  });

  function pinned(id: string) {
    if (id.startsWith("note:")) { tab = "overview"; return; }
    const a = atts.find((x) => x.id === id);
    if (a) { tab = "evidence"; queueMicrotask(() => document.getElementById("att-" + id)?.scrollIntoView({ behavior: "smooth", block: "center" })); }
  }
  function focusRange(sel: [number, number] | null) { selection = sel; if (sel) tab = "events"; }
  function jumpToFirstError() {
    if (!s?.first_error) return;
    selected = s.first_error.seq; problemsOnly = false; selection = null; q = ""; tab = "events";
  }
  async function saveHTML() {
    try { const p = await api.exportReportHTML(token); if (p) app.toast("Web page saved", "ok", { label: "Show", run: () => api.revealFile(p) }); }
    catch (e) { app.error(e); }
  }
  function close() { api.closeReport(token); reports.delete(token); app.go({ name: "home" }); }

  const tabs: { id: Tab; label: string }[] = [
    { id: "overview", label: "Overview" }, { id: "events", label: "Events" }, { id: "evidence", label: "Evidence" }, { id: "details", label: "System and privacy" },
  ];
  function tabKey(e: KeyboardEvent) {
    const i = tabs.findIndex((t) => t.id === tab);
    let j = i;
    if (e.key === "ArrowRight") j = (i + 1) % tabs.length;
    else if (e.key === "ArrowLeft") j = (i - 1 + tabs.length) % tabs.length;
    else return;
    e.preventDefault(); tab = tabs[j].id;
    queueMicrotask(() => document.getElementById("tab-" + tab)?.focus());
  }
  const headTone = $derived(s?.first_error?.kind === "crash" ? "crash" : s?.first_error ? "error" : "calm");
  const totalRemoved = $derived((m?.redaction?.rules ?? []).reduce((a, r) => a + r.matches, 0));
</script>

{#if view && s && m}
<div class="viewer">
  <header class="top">
    <div class="titles">
      <p class="from">{m.app ? `${m.app.name}${m.app.version ? " " + m.app.version : ""}` : m.producer.name}{m.os ? `, ${osName(m.os.name)} ${m.os.version ?? ""}` : ""}</p>
      <h1 class="selectable">{m.title}</h1>
    </div>
    <div class="acts">
      {#if s.verified}
        <span class="sealok" title="Every file matches the fingerprint recorded when the report was saved"><Icon name="shield" size={16} />Untouched since saved</span>
      {/if}
      <button class="btn" onclick={saveHTML}><Icon name="html" size={16} />Save as web page</button>
      <button class="btn quiet icon" aria-label="Close report" onclick={close}><Icon name="x" size={16} /></button>
    </div>
  </header>

  {#if !s.verified}
    <div class="tamper" role="alert"><Icon name="warn" /><div><strong>This report may have been changed or damaged after it was saved.</strong><p class="selectable">{s.integrity}</p><p>Treat its contents as unverified. Screenshots and recordings are not shown.</p></div></div>
  {/if}

  <div class="tabs" role="tablist" aria-label="Report sections" tabindex="-1" onkeydown={tabKey}>
    {#each tabs as t}
      <button id="tab-{t.id}" role="tab" aria-selected={tab === t.id} aria-controls="panel" tabindex={tab === t.id ? 0 : -1} class:on={tab === t.id} onclick={() => (tab = t.id)}>
        {t.label}{#if t.id === "events"}<span class="c">{s.total_events}</span>{:else if t.id === "evidence"}<span class="c">{atts.length}</span>{/if}
      </button>
    {/each}
  </div>

  <div id="panel" role="tabpanel" aria-labelledby="tab-{tab}" class="panel" class:fill={tab === "events"}>
    {#if tab === "overview"}
      <div class="scroll pad">
        <button class="headline {headTone}" onclick={jumpToFirstError} disabled={!s.first_error}>
          <span>{s.headline}</span>{#if s.first_error}<span class="go">Show in events<Icon name="chevron" size={15} /></span>{/if}
        </button>
        <div class="facts">
          <div><span class="n" class:hot={s.errors > 0}>{s.errors}</span><span>{s.errors === 1 ? "error" : "errors"}</span></div>
          <div><span class="n">{s.by_level.warn ?? 0}</span><span>warnings</span></div>
          <div><span class="n">{duration(s.duration)}</span><span>recorded, from {dateTime(s.start)}</span></div>
          <div><span class="n">{atts.length}</span><span>{atts.length === 1 ? "attachment" : "attachments"}</span></div>
        </div>

        <section class="card tapecard" aria-labelledby="tl-h">
          <div class="ch"><h2 id="tl-h">Timeline</h2><span class="muted">Drag across a moment to see its events</span></div>
          <Tape start={s.start} end={s.end} events={all} {pins} {selection} onselect={focusRange} onpin={pinned} />
        </section>

        <div class="story">
          {#if m.reproduction || m.description}
            <section class="card" aria-labelledby="st-h">
              <h2 id="st-h">What the reporter saw</h2>
              {#if m.description}<p class="prose selectable">{m.description}</p>{/if}
              {#if m.reproduction?.steps?.length}
                <h3>Steps to reproduce</h3>
                <ol class="steps selectable">{#each m.reproduction.steps as st}<li>{st}</li>{/each}</ol>
              {/if}
              {#if m.reproduction?.expected || m.reproduction?.actual}
                <div class="ea">
                  {#if m.reproduction?.expected}<div><h3>Expected</h3><p class="selectable">{m.reproduction.expected}</p></div>{/if}
                  {#if m.reproduction?.actual}<div class="act"><h3>Actually happened</h3><p class="selectable">{m.reproduction.actual}</p></div>{/if}
                </div>
              {/if}
              {#if m.reproduction?.frequency}<p class="muted freq">Happens: {m.reproduction.frequency}</p>{/if}
            </section>
          {/if}
          <section class="card" aria-labelledby="nt-h">
            <h2 id="nt-h">Notes</h2>
            {#if freeNotes.length}
              <ul class="notes">{#each freeNotes as n}<li>{#if n.at}<span class="mono t">{shortRel(s.start, n.at)}</span>{/if}<span class="selectable">{n.text}</span></li>{/each}</ul>
            {:else}<p class="muted">No notes were added.</p>{/if}
            {#if atts.some(media)}
              <h3 class="evh">Evidence</h3>
              <div class="mini">
                {#each atts.filter(media).slice(0, 4) as a}
                  <button class="minithumb" onclick={() => pinned(a.id)} aria-label="Open {a.title || a.role}">
                    {#if s.verified && a.media_type.startsWith("image/")}<img src={url(a)} alt="" />{:else}<Icon name={a.role === "recording" ? "record" : "window"} />{/if}
                  </button>
                {/each}
              </div>
            {/if}
          </section>
        </div>
      </div>
    {:else if tab === "events"}
      <div class="evwrap">
        <div class="evtape"><Tape start={s.start} end={s.end} events={all} {pins} {selection} onselect={(x) => (selection = x)} onpin={pinned} compact /></div>
        <div class="filters" role="search">
          <label class="search"><Icon name="search" size={15} /><span class="vh">Search events</span><input class="input" bind:value={q} placeholder="Search messages and errors" /></label>
          <div class="lvls" role="group" aria-label="Levels">
            {#each ["debug", "info", "warn", "error"] as l}
              <label class="lv lv-{l}" class:off={!levels[l]}><input type="checkbox" bind:checked={levels[l]} />{l}</label>
            {/each}
          </div>
          <label class="tog"><input type="checkbox" bind:checked={problemsOnly} />Problems only</label>
          <label class="tog"><input type="checkbox" checked={!hideMetrics} onchange={(e) => (hideMetrics = !(e.currentTarget as HTMLInputElement).checked)} />Measurements</label>
          {#if selection}<button class="btn sm" onclick={() => (selection = null)}><Icon name="x" size={13} />{shortRel(s.start, new Date(t0 + selection[0] * span).toISOString())}–{shortRel(s.start, new Date(t0 + selection[1] * span).toISOString())}</button>{/if}
          <span class="count muted" aria-live="polite">{filtered.length} of {all.length}</span>
        </div>
        <div class="split">
          <EventList events={filtered} start={s.start} bind:selected />
          <EventDetail event={selEvent} start={s.start} />
        </div>
        {#if s.truncated}<p class="trunc">Showing the first {all.length} of {s.total_events} events. Use <span class="mono">repropack inspect --events</span> for all of them.</p>{/if}
      </div>
    {:else if tab === "evidence"}
      <div class="scroll pad">
        {#if !atts.length}<p class="muted">This report has no attachments.</p>{/if}
        <div class="gal">
          {#each atts as a (a.id)}
            <figure id="att-{a.id}" class="att">
              <div class="frame">
                {#if !s.verified}
                  <p class="muted ph">Hidden because the report failed its integrity check.</p>
                {:else if a.media_type.startsWith("image/") && media(a)}
                  <button class="imgbtn" onclick={() => (lightbox = a)} aria-label="Enlarge {a.title}"><img src={url(a)} alt="{a.title || a.role} at {shortRel(s.start, a.created_at)}" /></button>
                {:else if a.media_type.startsWith("video/") && media(a)}
                  <!-- svelte-ignore a11y_media_has_caption -->
                  <video src={url(a)} controls preload="metadata" aria-label={a.title}></video>
                {:else if isText(a)}
                  <!-- svelte-ignore a11y_no_noninteractive_tabindex (Allow keyboard scrolling in the attachment preview.) -->
                  <pre class="txt selectable" tabindex="0" role="group" aria-label="{a.title} contents">{texts[a.id] || "Loading…"}</pre>
                {:else}
                  <p class="muted ph">{a.media_type} files are not displayed.</p>
                {/if}
              </div>
              <figcaption>
                <strong>{a.title || "Untitled " + a.role}</strong>
                <span class="muted">{a.role} at {shortRel(s.start, a.created_at)}, {bytes(a.size)}{a.annotated ? ", marked up" : ""}</span>
                {#each notesFor(a.id) as n}<span class="cap selectable">{n.text}</span>{/each}
              </figcaption>
            </figure>
          {/each}
        </div>
      </div>
    {:else}
      <div class="scroll pad details">
        <section class="card">
          <h2>System</h2>
          <dl class="kv">
            {#if m.app}<div><dt>App</dt><dd>{[m.app.name, m.app.version, m.app.build && `(${m.app.build})`].filter(Boolean).join(" ")}</dd></div>{/if}
            {#if m.app?.environment}<div><dt>Environment</dt><dd>{m.app.environment}</dd></div>{/if}
            {#if m.app?.runtime}<div><dt>Runtime</dt><dd>{m.app.runtime}</dd></div>{/if}
            {#if m.os}<div><dt>System</dt><dd>{[osName(m.os.name), m.os.version, m.os.build && `(${m.os.build})`].filter(Boolean).join(" ")}{m.os.arch ? `, ${m.os.arch}` : ""}</dd></div>{/if}
            {#if m.os?.locale}<div><dt>Language</dt><dd>{m.os.locale}</dd></div>{/if}
            {#if m.os?.timezone}<div><dt>Time zone</dt><dd>{m.os.timezone}</dd></div>{/if}
            {#if m.device?.model}<div><dt>Computer</dt><dd>{m.device.model}</dd></div>{/if}
            {#if m.device?.cpu}<div><dt>Processor</dt><dd>{m.device.cpu}{m.device.cpu_cores ? `, ${m.device.cpu_cores} cores` : ""}</dd></div>{/if}
            {#if m.device?.memory_bytes}<div><dt>Memory</dt><dd>{bytes(m.device.memory_bytes)}</dd></div>{/if}
            <div><dt>Captured with</dt><dd>{m.producer.name} {m.producer.version}{m.producer.sdk ? `, ${m.producer.sdk}` : ""}</dd></div>
          </dl>
          {#if m.context && Object.keys(m.context).length}
            <h3>Custom fields</h3>
            <dl class="kv mono-v">{#each Object.entries(m.context).sort() as [k, v]}<div><dt class="mono">{k}</dt><dd class="mono selectable">{valueText(v)}</dd></div>{/each}</dl>
          {/if}
        </section>
        <section class="card">
          <h2>Privacy</h2>
          {#if m.redaction}
            <p>{m.redaction.verified ? `Checked after saving: nothing matching the ${m.redaction.policy} rules remains.` : `Hidden with the ${m.redaction.policy} rules. Some values were kept on purpose or it was not re-checked.`}</p>
            {#if m.redaction.rules?.length}
              <table class="tbl"><thead><tr><th scope="col">What was hidden</th><th scope="col">Count</th></tr></thead>
                <tbody>{#each m.redaction.rules as r}<tr><td>{r.description || r.id}</td><td class="num">{r.matches}</td></tr>{/each}</tbody></table>
            {/if}
            <p class="muted">{totalRemoved} {totalRemoved === 1 ? "value" : "values"} hidden{m.redaction.manual ? `, ${m.redaction.manual} ${m.redaction.manual === 1 ? "area" : "areas"} blacked out on screenshots` : ""}.</p>
          {:else}
            <p>No redaction was recorded. Treat the contents as possibly sensitive.</p>
          {/if}
          {#if s.unreviewed_media?.length}<p class="warnline">{s.unreviewed_media.length} images or recordings were not marked as checked by the reporter.</p>{/if}
        </section>
        <section class="card wide">
          <h2>Integrity</h2>
          <p>{s.verified ? "Every file matches the fingerprint recorded in the report." : s.integrity}</p>
          <table class="tbl"><thead><tr><th scope="col">File</th><th scope="col">Size</th><th scope="col">SHA-256</th></tr></thead>
            <tbody>
              <tr><td class="mono">{m.events.path}</td><td class="num">{bytes(m.events.size)}</td><td class="mono selectable">{m.events.sha256.slice(0, 16)}…</td></tr>
              {#each atts as a}<tr><td class="mono">{a.path}</td><td class="num">{bytes(a.size)}</td><td class="mono selectable">{a.sha256.slice(0, 16)}…</td></tr>{/each}
            </tbody></table>
          <p class="muted mono selectable dig">{m.content_digest}</p>
          <p class="muted">Report {m.report_id}, format {m.spec_version}, file {bytes(view.fileSize)} at <span class="selectable">{view.path}</span></p>
        </section>
      </div>
    {/if}
  </div>
</div>

{#if lightbox}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="lb" onclick={() => (lightbox = null)} onkeydown={(e) => e.key === "Escape" && (lightbox = null)} role="dialog" aria-modal="true" aria-label={lightbox.title} tabindex="-1">
    <img src={url(lightbox)} alt={lightbox.title} />
    <button class="btn" onclick={() => (lightbox = null)}>Close</button>
  </div>
{/if}
{/if}

<style>
  .viewer { position: relative; z-index: 1; display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .top { display: flex; align-items: flex-end; gap: 16px; padding: 44px 28px 12px; }
  .titles { flex: 1; min-width: 0; }
  .from { color: var(--ink-3); font-size: 13px; font-weight: 600; }
  h1 { font-size: 26px; line-height: 1.15; letter-spacing: -0.022em; margin-top: 2px; }
  .acts { display: flex; gap: 8px; align-items: center; }
  .sealok { display: inline-flex; align-items: center; gap: 6px; color: var(--seal); font-weight: 650; font-size: 13px; padding: 0 10px; }
  .tamper { margin: 0 28px 10px; display: flex; gap: 12px; padding: 12px 14px; border-radius: var(--r-md); background: var(--err-soft); color: var(--ink); border: 1px solid color-mix(in oklab, var(--err) 40%, transparent); }
  .tamper :global(svg) { color: var(--err); flex: none; }
  .tamper p { font-size: 13px; margin-top: 2px; }
  .tabs { display: flex; gap: 2px; padding: 0 24px; border-bottom: 1px solid var(--rule); }
  .tabs button { position: relative; border: 0; background: none; padding: 10px 10px 11px; font-weight: 600; font-size: 13.5px; color: var(--ink-3); display: flex; gap: 6px; align-items: center; }
  .tabs button.on { color: var(--ink); }
  .tabs button.on::after { content: ""; position: absolute; left: 8px; right: 8px; bottom: -1px; height: 2px; border-radius: 2px; background: var(--seal); }
  .c { font-size: 11.5px; font-weight: 600; color: var(--ink-3); background: var(--sunk); padding: 0 6px; border-radius: 6px; }
  .panel { flex: 1; min-height: 0; display: flex; flex-direction: column; }
  .pad { padding: 20px 28px 40px; height: 100%; }
  .headline { all: unset; box-sizing: border-box; display: flex; align-items: center; gap: 12px; width: 100%; padding: 14px 16px; border-radius: var(--r-md); font-size: 16px; font-weight: 600; border-left: 4px solid; cursor: default; }
  .headline:focus-visible { outline: 2px solid var(--focus); outline-offset: 2px; }
  .headline.error { background: var(--err-soft); border-color: var(--err); }
  .headline.crash { background: var(--crash-soft); border-color: var(--crash); }
  .headline.calm { background: var(--seal-soft); border-color: var(--seal); }
  .headline span:first-child { flex: 1; }
  .go { display: inline-flex; align-items: center; gap: 2px; font-size: 13px; color: var(--ink-2); }
  .facts { display: flex; flex-wrap: wrap; gap: 10px 40px; margin: 18px 2px 20px; }
  .facts div { display: grid; }
  .facts .n { font-size: 22px; font-weight: 650; letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
  .facts .n.hot { color: var(--err); }
  .facts span:last-child { font-size: 12.5px; color: var(--ink-3); }
  .card { background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); padding: 16px 18px; display: grid; gap: 10px; align-content: start; }
  .card h2 { font-size: 15px; }
  .card h3 { font-size: 12.5px; color: var(--ink-3); margin-top: 4px; }
  .ch { display: flex; justify-content: space-between; align-items: baseline; }
  .ch .muted { font-size: 12.5px; }
  .story { display: grid; grid-template-columns: 1.4fr 1fr; gap: 14px; margin-top: 14px; }
  .prose { white-space: pre-wrap; line-height: 1.55; max-width: 70ch; }
  .steps { margin: 0; padding-left: 20px; display: grid; gap: 4px; }
  .ea { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
  .ea > div { background: var(--sunk); border-radius: var(--r-sm); padding: 10px 12px; }
  .ea .act { background: var(--err-soft); }
  .freq { font-size: 13px; }
  .notes { list-style: none; margin: 0; padding: 0; display: grid; gap: 8px; }
  .notes li { display: grid; grid-template-columns: auto 1fr; gap: 10px; font-size: 13.5px; }
  .notes .t { color: var(--ink-3); font-size: 12px; padding-top: 2px; }
  .mini { display: flex; gap: 8px; }
  .minithumb { width: 76px; height: 52px; border-radius: var(--r-xs); border: 1px solid var(--rule); background: var(--sunk); overflow: hidden; padding: 0; display: grid; place-items: center; color: var(--ink-3); }
  .minithumb img { width: 100%; height: 100%; object-fit: cover; }
  .evwrap { display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .evtape { padding: 10px 28px 0; }
  .filters { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; padding: 10px 28px; border-bottom: 1px solid var(--rule); }
  .search { display: flex; align-items: center; gap: 6px; color: var(--ink-3); }
  .search .input { width: 240px; height: 32px; padding: 4px 10px; }
  .lvls { display: flex; gap: 4px; }
  .lv, .tog { display: inline-flex; align-items: center; gap: 6px; height: 28px; padding: 0 10px; border: 1px solid var(--rule-2); border-radius: 999px; font-size: 12.5px; font-weight: 600; background: var(--surface); }
  .lv input, .tog input { margin: 0; accent-color: var(--seal); }
  .lv.off { color: var(--ink-3); background: transparent; }
  .lv-warn { color: var(--warn); } .lv-error { color: var(--err); }
  .sm { height: 28px; font-size: 12.5px; padding: 0 10px; }
  .count { margin-left: auto; font-size: 12.5px; font-variant-numeric: tabular-nums; }
  .split { flex: 1; min-height: 0; display: grid; grid-template-columns: minmax(0, 1fr) 380px; }
  .trunc { padding: 6px 28px; font-size: 12.5px; color: var(--ink-3); border-top: 1px solid var(--rule); }
  .gal { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 16px; }
  .att { margin: 0; background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); overflow: hidden; }
  .frame { background: var(--sunk); min-height: 160px; display: grid; place-items: center; }
  .frame img, .frame video { max-width: 100%; max-height: 440px; display: block; }
  .imgbtn { all: unset; cursor: zoom-in; display: block; }
  .imgbtn:focus-visible { outline: 2px solid var(--focus); }
  .txt { margin: 0; width: 100%; max-height: 300px; overflow: auto; padding: 12px 14px; font: 12px/1.5 var(--mono); white-space: pre-wrap; align-self: stretch; }
  .ph { padding: 24px; text-align: center; font-size: 13px; }
  figcaption { padding: 10px 14px 12px; display: grid; gap: 2px; font-size: 13.5px; }
  figcaption .muted { font-size: 12.5px; }
  .cap { margin-top: 6px; padding-left: 10px; border-left: 2px solid var(--seal); }
  .details { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; align-content: start; }
  .details .wide { grid-column: 1 / -1; }
  .kv { margin: 0; display: grid; gap: 5px; }
  .kv div { display: grid; grid-template-columns: 130px 1fr; gap: 12px; font-size: 13.5px; }
  .kv dt { color: var(--ink-3); }
  .kv dd { margin: 0; overflow-wrap: anywhere; }
  .mono-v div { font-size: 12.5px; }
  .tbl { width: 100%; border-collapse: collapse; font-size: 13px; }
  .tbl th { text-align: left; font-size: 12px; color: var(--ink-3); font-weight: 600; padding: 4px 8px 4px 0; border-bottom: 1px solid var(--rule); }
  .tbl td { padding: 6px 8px 6px 0; border-bottom: 1px solid var(--rule); }
  .tbl .num { text-align: right; font-variant-numeric: tabular-nums; }
  .warnline { background: var(--warn-soft); padding: 8px 10px; border-radius: var(--r-sm); font-size: 13px; }
  .dig { font-size: 12px; overflow-wrap: anywhere; }
  .lb { position: fixed; inset: 0; z-index: 40; background: rgba(0, 0, 0, 0.82); display: grid; place-items: center; gap: 12px; padding: 32px; grid-template-rows: 1fr auto; }
  .lb img { max-width: 100%; max-height: calc(100vh - 120px); box-shadow: var(--shadow-pop); }
  @media (max-width: 1100px) { .story, .details { grid-template-columns: 1fr; } .split { grid-template-columns: 1fr; grid-template-rows: 1fr 260px; } }
</style>
