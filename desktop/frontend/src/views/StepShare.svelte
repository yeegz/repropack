<script lang="ts">
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { keepFor } from "../lib/keep";
  import { bytes } from "../lib/fmt";
  import type { Draft, ExportResult } from "../lib/types";
  import Icon from "../components/Icon.svelte";

  let { id, draft, reload }: { id: string; draft: Draft; reload: () => Promise<void> } = $props();
  let withHTML = $state(true);
  let exporting = $state(false);
  let result = $state<ExportResult | null>(null);
  let host = $state(app.env?.issueHost || "github");
  let repo = $state(app.env?.issueRepo || "");
  let issueErr = $state("");

  async function exp() {
    exporting = true;
    try {
      const r = await api.exportDraft(id, [...keepFor(id)], withHTML);
      if (r) { result = r; app.toast("Report saved and checked", "ok"); await reload(); app.refreshLibrary(); }
    } catch (e) { app.error(e); }
    finally { exporting = false; }
  }
  async function issue() {
    issueErr = "";
    try {
      const u = await api.issueLink(result!.path, host, repo);
      await api.openURL(u);
      if (app.env) { app.env.issueRepo = repo; app.env.issueHost = host; }
    } catch (e) { issueErr = (e as Error).message; }
  }
  const name = (p: string) => p.split(/[\\/]/).pop();
</script>

<div class="share">
  {#if !result}
    <section class="ready">
      <h2>Save “{draft.title || "Untitled report"}”</h2>
      <p class="muted">You'll choose where to save one <span class="mono">.repropack</span> file. Before it's saved, ReproPack hides what you chose, then reopens the file to check its integrity and scan it again. Nothing is uploaded.</p>
      <ul class="what">
        <li><Icon name="note" size={16} />Your description and {draft.steps.length || "no"} {draft.steps.length === 1 ? "step" : "steps"}</li>
        <li><Icon name="window" size={16} />{draft.attachmentCount} {draft.attachmentCount === 1 ? "attachment" : "attachments"}</li>
        <li><Icon name="pulse" size={16} />{draft.eventCount} timeline {draft.eventCount === 1 ? "event" : "events"}{draft.targetName ? ` including ${draft.targetName}'s CPU and memory` : ""}</li>
        <li><Icon name="clock" size={16} />Your system: {app.env?.osName} {app.env?.osVersion}, {app.env?.arch}</li>
      </ul>
      <label class="check"><input type="checkbox" bind:checked={withHTML} />Also save a web page summary for people without ReproPack</label>
      <button class="btn primary big" onclick={exp} disabled={exporting}><Icon name="share" />{exporting ? "Saving and checking…" : "Save report…"}</button>
    </section>
  {:else}
    <section class="done" aria-live="polite">
      <div class="seal" aria-hidden="true"><Icon name="check" size={26} /></div>
      <div class="dtxt">
        <h2>Report saved</h2>
        <p class="file selectable mono">{result.path}</p>
        <dl>
          <div><dt>Size</dt><dd>{bytes(result.size)}</dd></div>
          <div><dt>Contents</dt><dd>{result.events} events, {result.attachments} attachments</dd></div>
          <div><dt>Hidden</dt><dd>{result.removed} private {result.removed === 1 ? "detail" : "details"}{result.kept ? `, ${result.kept} kept on purpose` : ""}</dd></div>
          <div><dt>Integrity</dt><dd>Checked. Fingerprint <span class="mono selectable" title={result.digest}>{result.digest.slice(7, 19)}</span></dd></div>
        </dl>
        <div class="row">
          <button class="btn" onclick={() => api.revealFile(result!.path)}><Icon name="open" size={16} />Show in Finder</button>
          {#if result.htmlPath}<button class="btn" onclick={() => api.revealFile(result!.htmlPath!)}><Icon name="html" size={16} />Show web page</button>{/if}
          <button class="btn quiet" onclick={() => (result = null)}>Save another copy</button>
        </div>
      </div>
    </section>

    <section class="issue" aria-labelledby="iss-h">
      <h3 id="iss-h">Report it to the developers</h3>
      <p class="muted">Opens a new issue in your browser with the summary filled in. Drag <strong>{name(result.path)}</strong> into the issue to attach it. ReproPack never uploads it for you.</p>
      <form class="issueform" onsubmit={(e) => { e.preventDefault(); issue(); }}>
        <div class="seg" role="radiogroup" aria-label="Issue tracker">
          <button type="button" role="radio" aria-checked={host === "github"} class:on={host === "github"} onclick={() => (host = "github")}>GitHub</button>
          <button type="button" role="radio" aria-checked={host === "gitlab"} class:on={host === "gitlab"} onclick={() => (host = "gitlab")}>GitLab</button>
        </div>
        <label class="vh" for="repo">Repository</label>
        <input id="repo" class="input repo" bind:value={repo} placeholder="owner/project" aria-invalid={!!issueErr} aria-describedby="repo-err" />
        <button class="btn primary" type="submit" disabled={!repo.trim()}><Icon name="link" size={16} />Open new issue</button>
      </form>
      {#if issueErr}<p id="repo-err" class="err" role="alert">{issueErr}</p>{/if}
    </section>
  {/if}
</div>

<style>
  .share { padding-top: 12px; max-width: 760px; display: grid; gap: 22px; }
  h2 { font-size: 22px; letter-spacing: -0.02em; }
  .ready { display: grid; gap: 16px; justify-items: start; }
  .ready p { font-size: 14px; line-height: 1.55; max-width: 64ch; }
  .what { list-style: none; margin: 0; padding: 14px 16px; display: grid; gap: 8px; background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); width: 100%; }
  .what li { display: flex; gap: 10px; align-items: center; font-size: 13.5px; color: var(--ink-2); }
  .check { display: flex; gap: 8px; align-items: center; font-size: 13.5px; font-weight: 600; }
  .check input { accent-color: var(--seal); width: 16px; height: 16px; margin: 0; }
  .done { display: grid; grid-template-columns: 56px 1fr; gap: 18px; background: var(--surface); border: 1px solid color-mix(in oklab, var(--seal) 45%, var(--rule)); border-radius: var(--r-lg); padding: 20px; }
  .seal { width: 56px; height: 56px; border-radius: 50%; display: grid; place-items: center; background: var(--seal); color: var(--seal-ink); animation: stamp 420ms var(--ease); }
  @keyframes stamp { 0% { transform: scale(0.6) rotate(-12deg); opacity: 0; } 70% { transform: scale(1.06) rotate(2deg); } }
  .dtxt { display: grid; gap: 10px; min-width: 0; }
  .file { font-size: 12.5px; color: var(--ink-2); overflow-wrap: anywhere; }
  dl { margin: 0; display: grid; gap: 4px; }
  dl div { display: grid; grid-template-columns: 90px 1fr; gap: 10px; font-size: 13.5px; }
  dt { color: var(--ink-3); }
  dd { margin: 0; }
  .row { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 4px; }
  .issue { display: grid; gap: 10px; }
  .issue h3 { font-size: 16px; }
  .issue p { font-size: 13.5px; line-height: 1.5; }
  .issueform { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .repo { width: 260px; height: 34px; }
  .seg { display: flex; padding: 2px; border-radius: 9px; background: var(--sunk); }
  .seg button { border: 0; background: transparent; height: 30px; padding: 0 12px; border-radius: 7px; font-weight: 600; font-size: 13px; color: var(--ink-2); }
  .seg button.on { background: var(--surface); color: var(--ink); box-shadow: var(--shadow-lift); }
  .err { color: var(--err); font-size: 13px; }
</style>
