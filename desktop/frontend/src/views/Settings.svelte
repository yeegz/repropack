<script lang="ts">
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { bytes } from "../lib/fmt";

  let days = $state(app.env?.retentionDays ?? 30);
  let saved = $state(false);
  const options = [{ v: 7, l: "1 week" }, { v: 30, l: "30 days" }, { v: 90, l: "90 days" }, { v: 0, l: "Keep until I delete them" }];
  async function setDays(v: number) {
    days = v;
    try { await api.setRetentionDays(v); if (app.env) app.env.retentionDays = v; saved = true; setTimeout(() => (saved = false), 1600); }
    catch (e) { app.error(e); }
  }
</script>

<div class="settings scroll">
  <div class="inner">
    <h1>Settings</h1>

    <section aria-labelledby="ret-h">
      <h2 id="ret-h">Unfinished reports</h2>
      <p class="muted">Reports you haven't exported stay on this Mac with their screenshots and recordings. Remove ones you haven't touched after:</p>
      <div class="opts" role="radiogroup" aria-labelledby="ret-h">
        {#each options as o}
          <label class="opt" class:on={days === o.v}><input type="radio" name="ret" checked={days === o.v} onchange={() => setDays(o.v)} />{o.l}</label>
        {/each}
      </div>
      <p class="muted small" aria-live="polite">{saved ? "Saved. Old reports are removed the next time ReproPack opens." : "Exported .repropack files are never touched."}</p>
    </section>

    <section aria-labelledby="priv-h">
      <h2 id="priv-h">What ReproPack does and doesn't do</h2>
      <ul class="facts">
        <li>Captures the screen only when you choose a window, area or display in the macOS picker, and shows a red bar while recording.</li>
        <li>Never records keystrokes, clipboard, browsing or other apps' contents.</li>
        <li>Watching an app reads only its CPU and memory use, once a second, until you stop.</li>
        <li>Never uploads anything. Issue links open in your browser and you attach the file yourself.</li>
        <li>Treats reports you open as untrusted: nothing inside them is run, and pictures are shown only after their fingerprints check out.</li>
      </ul>
    </section>

    {#if app.env}
      <section aria-labelledby="ab-h">
        <h2 id="ab-h">About this Mac and app</h2>
        <dl>
          <div><dt>ReproPack</dt><dd>{app.env.version}, report format 1.0</dd></div>
          <div><dt>System</dt><dd>{app.env.osName} {app.env.osVersion}, {app.env.arch}</dd></div>
          {#if app.env.cpu}<div><dt>Processor</dt><dd>{app.env.cpu}</dd></div>{/if}
          {#if app.env.memoryBytes}<div><dt>Memory</dt><dd>{bytes(app.env.memoryBytes)}</dd></div>{/if}
          <div><dt>Data folder</dt><dd class="mono selectable">{app.env.dataDir}</dd></div>
        </dl>
      </section>
    {/if}
  </div>
</div>

<style>
  .settings { position: relative; z-index: 1; flex: 1; }
  .inner { max-width: 720px; padding: 56px 40px; display: grid; gap: 34px; }
  h1 { font-size: 28px; letter-spacing: -0.02em; }
  h2 { font-size: 16px; margin-bottom: 6px; }
  section p { font-size: 13.5px; line-height: 1.5; }
  .small { font-size: 12.5px; margin-top: 8px; }
  .opts { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 12px; }
  .opt { display: inline-flex; align-items: center; height: 32px; padding: 0 13px; border: 1px solid var(--rule-2); border-radius: 999px; font-weight: 600; font-size: 13px; background: var(--surface); }
  .opt input { position: absolute; opacity: 0; pointer-events: none; }
  .opt.on { background: var(--seal-soft); border-color: var(--seal); color: var(--seal); }
  .opt:has(input:focus-visible) { outline: 2px solid var(--focus); outline-offset: 2px; }
  .facts { margin: 8px 0 0; padding-left: 18px; display: grid; gap: 6px; font-size: 13.5px; line-height: 1.5; color: var(--ink-2); }
  dl { margin: 8px 0 0; display: grid; gap: 6px; }
  dl div { display: grid; grid-template-columns: 120px 1fr; gap: 12px; font-size: 13.5px; }
  dt { color: var(--ink-3); }
  dd { margin: 0; overflow-wrap: anywhere; }
</style>
