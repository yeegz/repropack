<script lang="ts">
  import { onMount } from "svelte";
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import { keepFor } from "../lib/keep";
  import type { DraftDetail, Review, ReviewFinding } from "../lib/types";
  import Icon from "../components/Icon.svelte";
  import Evidence from "../components/Evidence.svelte";

  let { id, detail, reload, onready }: { id: string; detail: DraftDetail; reload: () => Promise<void>; onready: (ok: boolean) => void } = $props();
  let review = $state<Review | null>(null);
  let keep = $state(new Set(keepFor(id)));

  async function load() {
    try { review = await api.review(id); onready(review.unreviewed === 0); }
    catch (e) { app.error(e); }
  }
  onMount(load);
  async function reloadAll() { await reload(); await load(); }

  const groups = $derived.by(() => {
    const m = new Map<string, ReviewFinding[]>();
    for (const f of review?.findings ?? []) {
      const k = f.rule || f.ruleId;
      m.set(k, [...(m.get(k) ?? []), f]);
    }
    return [...m.entries()];
  });
  const keepable = (f: ReviewFinding) => f.field !== "attrs" && f.field !== "attachment";
  function toggle(f: ReviewFinding, k: boolean) {
    const s = new Set(keep);
    k ? s.add(f.key) : s.delete(f.key);
    keep = s;
    const shared = keepFor(id);
    shared.clear();
    s.forEach((x) => shared.add(x));
  }
  const total = $derived(review?.findings.length ?? 0);
  const hiding = $derived(total - keep.size);
</script>

<div class="review">
  <section class="block" aria-labelledby="txt-h">
    <div class="bh">
      <span class="ic"><Icon name="shield" /></span>
      <div>
        <h2 id="txt-h">{#if !review}Checking your report…{:else if total === 0}No private details found in the text{:else}{hiding} of {total} private {total === 1 ? "detail" : "details"} will be hidden{/if}</h2>
        <p class="muted">ReproPack looked for email addresses, passwords, keys and tokens, card numbers, account names in file paths and public IP addresses. Anything you keep is marked in the report as kept on purpose.</p>
      </div>
    </div>
    {#if review && total > 0}
      {#each groups as [rule, items]}
        <div class="group">
          <h3>{rule.charAt(0).toUpperCase() + rule.slice(1)} <span class="muted">{items.length}</span></h3>
          <ul>
            {#each items as f (f.key)}
              <li class:kept={keep.has(f.key)}>
                <div class="where"><span class="lab">{f.label}</span><span class="ctx selectable">{f.context || f.preview}</span></div>
                {#if keepable(f)}
                  <div class="seg" role="radiogroup" aria-label="{f.label}: hide or keep">
                    <button role="radio" aria-checked={!keep.has(f.key)} class:on={!keep.has(f.key)} onclick={() => toggle(f, false)}>Hide</button>
                    <button role="radio" aria-checked={keep.has(f.key)} class:on={keep.has(f.key)} onclick={() => toggle(f, true)}>Keep</button>
                  </div>
                {:else}
                  <span class="always">Always hidden</span>
                {/if}
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    {/if}
  </section>

  <section class="block" aria-labelledby="img-h">
    <div class="bh">
      <span class="ic"><Icon name="eye" /></span>
      <div>
        <h2 id="img-h">{#if !review}…{:else if review.media.length === 0}No screenshots or recordings to check{:else if review.unreviewed === 0}All images and recordings checked{:else}Check {review.unreviewed} {review.unreviewed === 1 ? "image or recording" : "images and recordings"}{/if}</h2>
        <p class="muted">Automatic checks can't see inside pictures. Look for names, messages, email, other apps or anything else you don't want to share. Use Hide parts to black out an area for good.</p>
      </div>
    </div>
    {#if review?.media.length}
      <Evidence draftId={id} attachments={detail.attachments} reload={reloadAll} reviewMode />
    {/if}
  </section>
</div>

<style>
  .review { display: grid; gap: 26px; padding-top: 12px; max-width: 980px; }
  .block { display: grid; gap: 14px; }
  .bh { display: grid; grid-template-columns: 40px 1fr; gap: 14px; align-items: start; }
  .ic { width: 40px; height: 40px; border-radius: 12px; background: var(--seal-soft); color: var(--seal); display: grid; place-items: center; }
  h2 { font-size: 18px; letter-spacing: -0.015em; }
  .bh p { margin-top: 4px; font-size: 13.5px; max-width: 72ch; line-height: 1.5; }
  .group { background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); overflow: hidden; }
  h3 { font-size: 13.5px; padding: 10px 16px; border-bottom: 1px solid var(--rule); background: var(--sunk); }
  h3 .muted { font-weight: 500; margin-left: 4px; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { display: grid; grid-template-columns: 1fr auto; gap: 16px; align-items: center; padding: 10px 16px; }
  li + li { border-top: 1px solid var(--rule); }
  li.kept { background: var(--warn-soft); }
  .where { display: grid; gap: 2px; min-width: 0; }
  .lab { font-size: 12px; color: var(--ink-3); font-weight: 600; }
  .ctx { font-family: var(--mono); font-size: 12.5px; overflow-wrap: anywhere; }
  .seg { display: flex; padding: 2px; border-radius: 8px; background: var(--sunk); }
  .seg button { border: 0; background: transparent; height: 26px; padding: 0 12px; border-radius: 6px; font-weight: 600; font-size: 12.5px; color: var(--ink-2); }
  .seg button.on { background: var(--surface); color: var(--ink); box-shadow: var(--shadow-lift); }
  li.kept .seg button.on { color: var(--warn); }
  .always { font-size: 12px; color: var(--ink-3); }
</style>
