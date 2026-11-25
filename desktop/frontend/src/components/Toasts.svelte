<script lang="ts">
  import { app } from "../lib/state.svelte";
  import Icon from "./Icon.svelte";
</script>

<div class="toasts" role="region" aria-label="Notifications">
  <div aria-live="polite" class="stack">
    {#each app.toasts as t (t.id)}
      <div class="toast {t.tone}" role={t.tone === "error" ? "alert" : "status"}>
        <span class="ic"><Icon name={t.tone === "error" ? "warn" : t.tone === "ok" ? "check" : "note"} size={16} /></span>
        <span class="txt selectable">{t.text}</span>
        {#if t.action}<button class="btn quiet sm" onclick={() => { t.action!.run(); app.dismiss(t.id); }}>{t.action.label}</button>{/if}
        <button class="btn quiet icon sm" aria-label="Dismiss" onclick={() => app.dismiss(t.id)}><Icon name="x" size={14} /></button>
      </div>
    {/each}
  </div>
</div>

<style>
  .toasts { position: fixed; right: 24px; bottom: 72px; z-index: 50; pointer-events: none; }
  .stack { display: grid; gap: 8px; justify-items: end; }
  .toast {
    pointer-events: auto; display: flex; align-items: center; gap: 10px; max-width: 460px;
    background: var(--raised); color: var(--ink); border: 1px solid var(--rule); border-radius: var(--r-md);
    padding: 8px 8px 8px 12px; box-shadow: var(--shadow-pop); animation: in var(--t-med) var(--ease);
  }
  .ic { display: inline-flex; color: var(--seal); }
  .error .ic { color: var(--err); }
  .error { border-color: color-mix(in oklab, var(--err) 35%, var(--rule)); }
  .txt { font-size: 13.5px; line-height: 1.4; }
  .sm { height: 28px; font-size: 12.5px; }
  .icon.sm { width: 28px; }
  @keyframes in { from { transform: translateY(8px); opacity: 0; } }
</style>
