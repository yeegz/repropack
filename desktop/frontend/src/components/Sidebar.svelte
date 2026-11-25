<script lang="ts">
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { ago } from "../lib/fmt";
  import { reports } from "../lib/reports";
  import Icon from "./Icon.svelte";
  import type { Draft, Recent } from "../lib/types";

  let { onnew, onopen }: { onnew: () => void; onopen: () => void } = $props();
  let drafts = $state<Draft[]>([]);
  let recents = $state<Recent[]>([]);

  $effect(() => {
    app.libraryVersion;
    api.listDrafts().then((d) => (drafts = d)).catch(() => {});
    api.recents().then((r) => (recents = r)).catch(() => {});
  });

  async function openRecent(r: Recent) {
    try {
      const v = await api.openReport(r.path);
      reports.set(v.token, v);
      app.go({ name: "report", token: v.token });
      app.refreshLibrary();
    } catch (e) {
      app.toast(`Could not open ${r.title}: ${(e as Error).message}`, "error", { label: "Remove from list", run: () => api.forgetRecent(r.path).then(() => app.refreshLibrary()) });
    }
  }
  const active = (id: string) => app.route.name === "draft" && app.route.id === id;
</script>

<nav class="side" aria-label="Library">
  <div class="drag" style="--wails-draggable:drag"></div>
  <div class="brand">
    <span class="logo"><Icon name="pack" size={20} /></span>
    <span class="word">ReproPack</span>
  </div>
  <div class="actions">
    <button class="btn primary new" onclick={onnew}><Icon name="plus" size={16} />New report<kbd>⌘N</kbd></button>
    <button class="btn quiet open" onclick={onopen}><Icon name="open" size={16} />Open a report…<kbd>⌘O</kbd></button>
  </div>

  <div class="lists scroll">
    <section aria-labelledby="h-drafts">
      <h2 id="h-drafts">In progress</h2>
      {#if drafts.length === 0}
        <p class="empty">Reports you start appear here until you export them.</p>
      {:else}
        <ul>
          {#each drafts as d (d.id)}
            <li>
              <button class="item" class:on={active(d.id)} aria-current={active(d.id) ? "page" : undefined} onclick={() => app.go({ name: "draft", id: d.id })}>
                <span class="t">{d.title || "Untitled report"}</span>
                <span class="m">{d.exportedPath ? "Exported" : "Edited"} {ago(d.updatedAt)}{#if app.recordingDraft === d.id}<span class="rec">Recording</span>{/if}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </section>
    {#if recents.length}
      <section aria-labelledby="h-recent">
        <h2 id="h-recent">Opened reports</h2>
        <ul>
          {#each recents as r (r.path)}
            <li>
              <button class="item" class:missing={!r.exists} onclick={() => openRecent(r)} title={r.path}>
                <span class="t">{r.title}</span>
                <span class="m">{r.exists ? `Opened ${ago(r.openedAt)}` : "File moved or deleted"}</span>
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>

  <div class="foot">
    <button class="btn quiet settings" class:on={app.route.name === "settings"} onclick={() => app.go({ name: "settings" })}><Icon name="gear" size={16} />Settings</button>
    <p class="local"><Icon name="lock" size={14} />Stored on this Mac only</p>
  </div>
</nav>

<style>
  .side { position: relative; display: flex; flex-direction: column; background: var(--sidebar); border-right: 1px solid var(--rule); min-width: 0; height: 100vh; }
  .drag { height: 38px; flex: none; }
  .brand { display: flex; align-items: center; gap: 9px; padding: 0 18px 14px; }
  .logo { color: var(--seal); display: inline-flex; }
  .word { font-weight: 700; font-size: 15.5px; letter-spacing: -0.015em; }
  .actions { display: grid; gap: 4px; padding: 0 12px 10px; }
  .new, .open { justify-content: flex-start; width: 100%; }
  kbd { margin-left: auto; font: 500 11.5px var(--font); opacity: 0.7; }
  .lists { flex: 1; padding: 6px 8px 12px; }
  section + section { margin-top: 18px; }
  h2 { font-size: 12.5px; font-weight: 650; color: var(--ink-3); padding: 8px 10px 6px; letter-spacing: 0; }
  ul { list-style: none; margin: 0; padding: 0; display: grid; gap: 1px; }
  .item { width: 100%; text-align: left; border: 0; background: transparent; padding: 7px 10px; border-radius: var(--r-sm); display: grid; gap: 1px; }
  .item:hover { background: color-mix(in oklab, var(--ink) 5%, transparent); }
  .item.on { background: var(--surface); box-shadow: var(--shadow-lift); }
  .item .t { font-weight: 600; font-size: 13.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .item .m { font-size: 12px; color: var(--ink-3); display: flex; gap: 8px; }
  .item.missing .t { color: var(--ink-3); }
  .rec { color: var(--rec); font-weight: 650; }
  .empty { padding: 4px 10px; color: var(--ink-3); font-size: 12.5px; line-height: 1.45; }
  .foot { padding: 10px 12px 14px; border-top: 1px solid var(--rule); display: grid; gap: 6px; }
  .settings { justify-content: flex-start; width: 100%; }
  .settings.on { background: var(--surface); }
  .local { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--ink-3); padding: 0 10px; }
</style>
