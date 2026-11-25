<script lang="ts">
  import { api, on } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { bytes, clock, metricValue, shortRel } from "../lib/fmt";
  import type { DraftDetail, Process } from "../lib/types";
  import Icon from "../components/Icon.svelte";
  import Tape, { type Pin } from "../components/Tape.svelte";
  import Evidence from "../components/Evidence.svelte";

  let { detail, reload }: { detail: DraftDetail; reload: () => Promise<void> } = $props();
  const caps = $derived(app.env?.capabilities);
  let busy = $state<string>("");
  let note = $state("");
  let picking = $state(false);
  let procs = $state<Process[]>([]);
  let query = $state("");
  let lastSample = $state<{ cpu: number; rss: number } | null>(null);
  let now = $state(Date.now());
  $effect(() => { const t = setInterval(() => (now = Date.now()), 1000); return () => clearInterval(t); });

  $effect(() => on("sample", (d: any) => { if (d.draft === detail.draft.id) lastSample = { cpu: d.cpu, rss: d.rss }; }));

  async function run(name: string, fn: () => Promise<unknown>, done?: string) {
    busy = name;
    try { await fn(); if (done) app.toast(done, "ok"); await reload(); app.refreshLibrary(); }
    catch (e) { if (!/cancelled/.test((e as Error).message)) app.error(e); }
    finally { busy = ""; }
  }
  const id = $derived(detail.draft.id);
  const recordingHere = $derived(app.recordingDraft === id || detail.recording);

  async function startRec() {
    await run("rec", async () => { await api.startRecording(id); app.recordingDraft = id; app.recordingSince = Date.now(); });
  }
  async function openPicker() {
    picking = true;
    try { procs = await api.listProcesses(); } catch (e) { app.error(e); picking = false; }
    queueMicrotask(() => document.getElementById("proc-q")?.focus());
  }
  const filtered = $derived(procs.filter((p) => p.name.toLowerCase().includes(query.toLowerCase())).slice(0, 60));
  async function watch(p: Process) {
    picking = false;
    await run("watch", () => api.watchProcess(id, p.pid, p.name), `Watching ${p.name}. CPU and memory are added to the timeline every second.`);
  }
  async function addNote() {
    const t = note.trim();
    if (!t) return;
    await run("note", () => api.addNote(id, t));
    note = "";
  }

  const events = $derived(detail.events ?? []);
  const pins = $derived<Pin[]>([
    ...detail.attachments.map((a) => ({ id: a.id, at: a.createdAt, label: a.title, kind: (a.role === "recording" ? "video" : "shot") as Pin["kind"] })),
    ...detail.notes.map((n) => ({ id: n.id, at: n.at, label: n.text, kind: "note" as const })),
  ]);
  // Span from the earliest evidence (imported logs can predate the report) to now.
  const tapeStart = $derived.by(() => {
    let t = Date.parse(detail.draft.createdAt);
    for (const e of events) t = Math.min(t, Date.parse(e.ts));
    for (const p of pins) t = Math.min(t, Date.parse(p.at));
    return new Date(t).toISOString();
  });
  const tapeEnd = $derived(new Date(Math.max(now, Date.parse(tapeStart) + 30_000)).toISOString());
  const marks = $derived(events.filter((e) => e.kind === "marker" && e.source === "you").length);
</script>

<div class="capture">
  <section class="tapebox" aria-labelledby="tape-h">
    <div class="tapehead">
      <h2 id="tape-h">What's been collected</h2>
      <span class="muted">{shortRel(tapeStart, tapeEnd)} of activity</span>
    </div>
    {#if events.length > 1 || pins.length}
      <Tape start={tapeStart} end={tapeEnd} {events} {pins} live />
    {:else}
      <p class="empty">Screenshots, recordings, notes and the measurements of any app you watch will line up here in time order.</p>
    {/if}
  </section>

  <div class="grid">
    <section aria-labelledby="act-h">
      <h2 id="act-h" class="vh">Capture actions</h2>
      <ul class="actions">
        <li>
          <span class="ic"><Icon name="window" /></span>
          <div><h3>Screenshot a window</h3><p>Click the window that shows the problem. Only that window is captured.</p></div>
          <button class="btn" disabled={!caps?.screenshots || !!busy} onclick={() => run("win", () => api.takeScreenshot(id, "window"), "Screenshot added")}>{busy === "win" ? "Waiting for your click…" : "Choose window"}</button>
        </li>
        <li>
          <span class="ic"><Icon name="area" /></span>
          <div><h3>Screenshot an area</h3><p>Drag around just the part you want to show.</p></div>
          <button class="btn" disabled={!caps?.screenshots || !!busy} onclick={() => run("area", () => api.takeScreenshot(id, "area"), "Screenshot added")}>{busy === "area" ? "Waiting for your selection…" : "Select area"}</button>
        </li>
        <li>
          <span class="ic rec"><Icon name="record" /></span>
          <div><h3>Record your screen</h3><p>Pick a window, an area or a display in the macOS toolbar. A red bar stays visible while recording.</p></div>
          {#if recordingHere}
            <button class="btn stop" onclick={() => run("stop", () => api.stopRecording(), "Recording saved")}><Icon name="stop" size={15} />Stop</button>
          {:else}
            <button class="btn" disabled={!caps?.recording || !!busy || !!app.recordingDraft} onclick={startRec}>Start recording</button>
          {/if}
        </li>
        <li>
          <span class="ic"><Icon name="file" /></span>
          <div><h3>Add files</h3><p>Screenshots, videos or log files you already have. Logs are added to the timeline.</p></div>
          <button class="btn" disabled={!!busy} onclick={() => run("files", () => api.addFiles(id))}>Choose files…</button>
        </li>
        <li class="watch">
          <span class="ic"><Icon name="pulse" /></span>
          <div>
            <h3>Watch an app</h3>
            {#if detail.watching}
              <p class="live">Watching <strong>{detail.draft.targetName}</strong>{#if lastSample}: CPU {metricValue(lastSample.cpu, "%")}, memory {bytes(lastSample.rss)}{/if}</p>
            {:else}
              <p>Records its CPU and memory once a second while you reproduce the problem. Nothing else is read.</p>
            {/if}
          </div>
          {#if detail.watching}
            <button class="btn" onclick={() => run("unwatch", () => api.stopWatching())}>Stop watching</button>
          {:else}
            <button class="btn" disabled={!caps?.processList || !!busy} onclick={openPicker} aria-expanded={picking}>Choose app</button>
          {/if}
        </li>
      </ul>
      {#if picking}
        <div class="picker" role="dialog" aria-label="Choose an app to watch">
          <div class="pq"><Icon name="search" size={16} /><input id="proc-q" class="input" placeholder="Search running apps" bind:value={query} onkeydown={(e) => e.key === "Escape" && (picking = false)} /></div>
          <ul class="plist scroll" role="listbox" aria-label="Running apps">
            {#each filtered as p (p.pid)}
              <li role="option" aria-selected="false"><button onclick={() => watch(p)}><span>{p.name}</span>{#if !p.app}<span class="muted">background process</span>{/if}</button></li>
            {:else}
              <li class="muted none">No running app matches “{query}”.</li>
            {/each}
          </ul>
          <button class="btn quiet" onclick={() => (picking = false)}>Cancel</button>
        </div>
      {/if}
      {#if caps && !caps.screenshots}<p class="capnote"><Icon name="warn" size={15} />{caps.note}</p>{/if}
    </section>

    <section class="moment" aria-labelledby="mom-h">
      <h2 id="mom-h">When it happens</h2>
      <button class="btn big markbtn" onclick={() => run("mark", () => api.markMoment(id), "Marked on the timeline")}><Icon name="flag" />It happened just now</button>
      <p class="muted small">{marks ? `${marks} ${marks === 1 ? "moment" : "moments"} marked` : "Press this the moment you see the problem, so developers know where to look."}</p>
      <form class="noteform" onsubmit={(e) => { e.preventDefault(); addNote(); }}>
        <label class="field"><span>Add a note</span>
          <textarea class="textarea" rows="3" bind:value={note} placeholder="The fans spun up right when it froze." onkeydown={(e) => { if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) addNote(); }}></textarea>
        </label>
        <button class="btn" type="submit" disabled={!note.trim()}>Add note</button>
      </form>
      {#if detail.notes.length}
        <ul class="notes">
          {#each detail.notes as n (n.id)}
            <li><span class="mono t">{clock(n.at)}</span><span class="selectable">{n.text}</span>
              <button class="btn quiet icon sm" aria-label="Delete note" onclick={() => run("dn", () => api.deleteNote(id, n.id))}><Icon name="x" size={13} /></button></li>
          {/each}
        </ul>
      {/if}
    </section>
  </div>

  <Evidence draftId={id} attachments={detail.attachments} {reload} />
</div>

<style>
  .capture { display: grid; gap: 22px; padding-top: 8px; }
  .tapebox { background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); padding: 14px 18px 10px; }
  .tapehead { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 10px; }
  .tapehead h2 { font-size: 15px; }
  .tapehead .muted { font-size: 12.5px; font-variant-numeric: tabular-nums; }
  .empty { color: var(--ink-3); font-size: 13.5px; padding: 18px 0 14px; }
  .grid { display: grid; grid-template-columns: minmax(0, 1fr) 320px; gap: 28px; align-items: start; }
  .actions { list-style: none; margin: 0; padding: 0; border: 1px solid var(--rule); border-radius: var(--r-lg); background: var(--surface); }
  .actions li { display: grid; grid-template-columns: 36px 1fr auto; gap: 14px; align-items: center; padding: 14px 16px; }
  .actions li + li { border-top: 1px solid var(--rule); }
  .ic { width: 36px; height: 36px; border-radius: 10px; background: var(--sunk); display: grid; place-items: center; color: var(--ink-2); }
  .ic.rec { color: var(--rec); }
  h3 { font-size: 14px; }
  .actions p { font-size: 13px; color: var(--ink-3); margin-top: 1px; line-height: 1.4; }
  .actions .live { color: var(--ink); }
  .stop { color: var(--rec); border-color: var(--rec); }
  .picker { margin-top: 8px; border: 1px solid var(--rule); border-radius: var(--r-md); background: var(--raised); box-shadow: var(--shadow-pop); padding: 10px; display: grid; gap: 8px; }
  .pq { display: flex; align-items: center; gap: 8px; color: var(--ink-3); }
  .plist { list-style: none; margin: 0; padding: 0; max-height: 240px; }
  .plist button { width: 100%; text-align: left; display: flex; justify-content: space-between; border: 0; background: none; padding: 7px 10px; border-radius: var(--r-xs); font-weight: 600; }
  .plist button:hover, .plist button:focus-visible { background: var(--sunk); }
  .none { padding: 8px 10px; }
  .capnote { margin-top: 10px; display: flex; gap: 8px; align-items: center; color: var(--warn); font-size: 13px; }
  .moment { display: grid; gap: 10px; }
  .moment h2 { font-size: 15px; }
  .markbtn { width: 100%; background: var(--warn-soft); border-color: color-mix(in oklab, var(--warn) 40%, transparent); color: var(--warn); }
  .markbtn:hover { background: color-mix(in oklab, var(--warn) 18%, var(--surface)); }
  .small { font-size: 12.5px; line-height: 1.4; }
  .noteform { display: grid; gap: 8px; margin-top: 8px; }
  .noteform .btn { justify-self: start; }
  .notes { list-style: none; margin: 4px 0 0; padding: 0; display: grid; gap: 6px; }
  .notes li { display: grid; grid-template-columns: auto 1fr auto; gap: 8px; align-items: start; font-size: 13.5px; padding: 8px 6px 8px 10px; background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-sm); }
  .notes .t { color: var(--ink-3); font-size: 12px; padding-top: 2px; }
  .sm { height: 24px; width: 24px; }
  @media (max-width: 1100px) { .grid { grid-template-columns: 1fr; } }
</style>
