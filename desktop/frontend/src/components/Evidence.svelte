<script lang="ts">
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { bytes, clock } from "../lib/fmt";
  import type { DraftAttachment } from "../lib/types";
  import Icon from "./Icon.svelte";
  import MarkupEditor from "./MarkupEditor.svelte";

  let { draftId, attachments, reload, reviewMode = false }: { draftId: string; attachments: DraftAttachment[]; reload: () => Promise<void>; reviewMode?: boolean } = $props();
  let editing = $state<DraftAttachment | null>(null);
  let removing = $state<string | null>(null);
  let notes = $state<Record<string, string>>({});
  $effect(() => { for (const a of attachments) if (notes[a.id] === undefined) notes[a.id] = a.note; });

  const media = (a: DraftAttachment) => a.mediaType.startsWith("image/") || a.mediaType.startsWith("video/");
  const shown = $derived(reviewMode ? attachments.filter(media) : attachments);

  async function act(fn: () => Promise<unknown>) {
    try { await fn(); await reload(); app.refreshLibrary(); } catch (e) { app.error(e); }
  }
  let noteTimer: any;
  function saveNote(a: DraftAttachment) {
    clearTimeout(noteTimer);
    noteTimer = setTimeout(() => api.setAttachmentNote(draftId, a.id, notes[a.id] ?? "").catch((e) => app.error(e)), 500);
  }
</script>

{#if shown.length}
<section aria-labelledby="ev-h-{reviewMode}">
  <h2 id="ev-h-{reviewMode}" class:vh={reviewMode}>Evidence</h2>
  <ul class="gal">
    {#each shown as a (a.id)}
      <li class:ok={reviewMode && a.reviewed}>
        <div class="thumb">
          {#if a.mediaType.startsWith("image/")}
            <img src={a.url} alt="{a.title}, captured at {clock(a.createdAt)}" draggable="false" />
          {:else if a.mediaType.startsWith("video/")}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video src={a.url} controls preload="metadata" aria-label={a.title}></video>
          {:else}
            <div class="doc"><Icon name="file" size={26} /><span>{a.title}</span></div>
          {/if}
          {#if a.annotated}<span class="tag">Marked up</span>{/if}
        </div>
        <div class="meta">
          <div class="row">
            <h3 title={a.title}>{a.title}</h3>
            <span class="muted">{clock(a.createdAt)}{a.durationMs ? `, ${Math.round(a.durationMs / 1000)} s` : ""}, {bytes(a.size)}</span>
          </div>
          {#if !reviewMode}
            <label class="vh" for="n-{a.id}">Note for {a.title}</label>
            <input id="n-{a.id}" class="input sm" placeholder="Add a caption" bind:value={notes[a.id]} oninput={() => saveNote(a)} />
          {/if}
          <div class="btns">
            {#if a.mediaType.startsWith("image/")}
              <button class="btn sm" onclick={() => (editing = a)}><Icon name="pen" size={14} />{reviewMode ? "Hide parts" : "Mark up"}</button>
            {/if}
            {#if reviewMode}
              <label class="check">
                <input type="checkbox" checked={a.reviewed} onchange={(e) => act(() => api.setReviewed(draftId, a.id, (e.currentTarget as HTMLInputElement).checked))} />
                {a.mediaType.startsWith("video/") ? "I watched it; nothing private is visible" : "Nothing private is visible"}
              </label>
            {:else if removing === a.id}
              <span class="confirm">Remove?</span>
              <button class="btn sm danger" onclick={() => act(() => api.removeAttachment(draftId, a.id))}>Remove</button>
              <button class="btn sm quiet" onclick={() => (removing = null)}>Keep</button>
            {:else}
              <button class="btn sm quiet" onclick={() => (removing = a.id)} aria-label="Remove {a.title}"><Icon name="trash" size={14} />Remove</button>
            {/if}
          </div>
        </div>
      </li>
    {/each}
  </ul>
</section>
{/if}

{#if editing}
  <MarkupEditor att={editing} onclose={() => (editing = null)} onsave={async (png, red) => {
    const a = editing!;
    editing = null;
    await act(() => api.saveMarkup(draftId, a.id, png, red));
    app.toast(red ? `Saved. ${red} hidden ${red === 1 ? "area is" : "areas are"} permanently blacked out.` : "Markup saved", "ok");
  }} />
{/if}

<style>
  h2 { font-size: 15px; margin-bottom: 10px; }
  .gal { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); gap: 14px; }
  li { background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); overflow: hidden; display: flex; flex-direction: column; transition: border-color var(--t-med); }
  li.ok { border-color: color-mix(in oklab, var(--seal) 55%, var(--rule)); }
  .thumb { position: relative; background: var(--sunk); aspect-ratio: 16 / 10; display: grid; place-items: center; overflow: hidden; }
  .thumb img, .thumb video { width: 100%; height: 100%; object-fit: contain; display: block; }
  .doc { display: grid; justify-items: center; gap: 6px; color: var(--ink-3); font-size: 12.5px; padding: 12px; text-align: center; }
  .tag { position: absolute; left: 8px; top: 8px; background: var(--raised); font-size: 11.5px; font-weight: 650; padding: 2px 8px; border-radius: 6px; box-shadow: var(--shadow-lift); }
  .meta { padding: 10px 12px 12px; display: grid; gap: 8px; }
  .row { display: grid; gap: 1px; }
  h3 { font-size: 13.5px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .row .muted { font-size: 12px; }
  .btns { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .sm { height: 28px; font-size: 12.5px; padding: 0 10px; }
  .input.sm { height: 30px; padding: 4px 9px; font-size: 13px; }
  .confirm { font-size: 12.5px; color: var(--err); font-weight: 600; }
  .check { display: flex; gap: 7px; align-items: center; font-size: 12.5px; font-weight: 600; }
  .check input { accent-color: var(--seal); width: 15px; height: 15px; margin: 0; }
</style>
