<script lang="ts">
  import { onMount, untrack } from "svelte";
  import { api, on } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import type { DraftDetail, DraftText } from "../lib/types";
  import Icon from "../components/Icon.svelte";
  import StepDescribe from "./StepDescribe.svelte";
  import StepCapture from "./StepCapture.svelte";
  import StepReview from "./StepReview.svelte";
  import StepShare from "./StepShare.svelte";

  let { id, initialStep = 0 }: { id: string; initialStep?: number } = $props();
  let step = $state(untrack(() => initialStep));
  let detail = $state<DraftDetail | null>(null);
  let loadError = $state("");
  let saving = $state<"idle" | "saving" | "saved">("idle");
  let confirmDelete = $state(false);
  let reviewReady = $state(false);
  let headingEl: HTMLElement | undefined = $state();

  const steps = [
    { t: "Describe", icon: "note" },
    { t: "Capture", icon: "window" },
    { t: "Review", icon: "shield" },
    { t: "Share", icon: "share" },
  ];

  async function load() {
    try { detail = await api.getDraft(id); loadError = ""; }
    catch (e) { loadError = (e as Error).message; }
  }
  onMount(() => {
    load();
    const off1 = on("sample", (d: any) => { if (d.draft === id) throttledLoad(); });
    const off2 = on("recording", (d: any) => { if (d.draft === id) load(); });
    const off3 = on("watch", (d: any) => { if (d.draft === id) { app.toast("The app you were watching has quit. Its last measurements are in the report."); load(); } });
    return () => { off1(); off2(); off3(); };
  });
  let lastLoad = 0;
  function throttledLoad() { if (Date.now() - lastLoad > 2000) { lastLoad = Date.now(); load(); } }

  let timer: any;
  function saveText(t: DraftText) {
    if (!detail) return;
    Object.assign(detail.draft, t);
    saving = "saving";
    clearTimeout(timer);
    timer = setTimeout(async () => {
      try { await api.saveDraftText(id, t); saving = "saved"; app.refreshLibrary(); }
      catch (e) { app.error(e); saving = "idle"; }
    }, 400);
  }

  function goto(i: number) {
    step = i;
    queueMicrotask(() => headingEl?.focus());
  }
  async function del() {
    try { await api.deleteDraft(id); app.refreshLibrary(); app.go({ name: "home" }); app.toast("Report deleted", "ok"); }
    catch (e) { app.error(e); }
  }
  const canContinue = $derived(step !== 0 || !!detail?.draft.title.trim());
</script>

<div class="draft">
  <header class="head">
    <nav aria-label="Report steps">
      <ol class="stepper">
        {#each steps as s, i}
          <li>
            <button class:on={step === i} class:done={step > i} aria-current={step === i ? "step" : undefined} onclick={() => goto(i)} disabled={i > 0 && !detail?.draft.title.trim()}>
              <span class="n" aria-hidden="true">{#if step > i}<Icon name="check" size={13} />{:else}{i + 1}{/if}</span>{s.t}
            </button>
          </li>
        {/each}
      </ol>
    </nav>
    <div class="right">
      <span class="save" aria-live="polite">{saving === "saving" ? "Saving…" : saving === "saved" ? "Saved on this Mac" : ""}</span>
      <button class="btn quiet icon" aria-label="Delete this report" title="Delete this report" onclick={() => (confirmDelete = true)}><Icon name="trash" size={16} /></button>
    </div>
  </header>

  {#if confirmDelete}
    <div class="confirm" role="alertdialog" aria-labelledby="del-t" aria-describedby="del-d">
      <p><strong id="del-t">Delete this report?</strong> <span id="del-d">Its screenshots, recordings and notes are removed from this Mac. Files you already exported are kept.</span></p>
      <!-- svelte-ignore a11y_autofocus -->
      <button class="btn" autofocus onclick={() => (confirmDelete = false)}>Keep it</button>
      <button class="btn danger" onclick={del}>Delete report</button>
    </div>
  {/if}

  <div class="body scroll">
    {#if loadError}
      <div class="failed"><p>This report could not be loaded: {loadError}</p><button class="btn" onclick={load}>Try again</button></div>
    {:else if detail}
      <div class="page">
        <h1 tabindex="-1" bind:this={headingEl} class="vh">{steps[step].t}</h1>
        {#if step === 0}
          <StepDescribe draft={detail.draft} onchange={saveText} />
        {:else if step === 1}
          <StepCapture {detail} reload={load} />
        {:else if step === 2}
          <StepReview {id} onready={(ok) => (reviewReady = ok)} {detail} reload={load} />
        {:else}
          <StepShare {id} draft={detail.draft} reload={load} />
        {/if}
      </div>
    {/if}
  </div>

  <footer class="foot">
    {#if step > 0}<button class="btn" onclick={() => goto(step - 1)}><Icon name="back" size={16} />Back</button>{/if}
    <span class="grow"></span>
    {#if step < 3}
      {#if step === 0 && !canContinue}<span class="why">Add a short title to continue</span>{/if}
      {#if step === 2 && !reviewReady}<span class="why">Check every image and recording first</span>{/if}
      <button class="btn primary" disabled={!canContinue || (step === 2 && !reviewReady)} onclick={() => goto(step + 1)}>
        {["Continue to capture", "Continue to review", "Continue to share"][step]}<Icon name="chevron" size={16} />
      </button>
    {/if}
  </footer>
</div>

<style>
  .draft { position: relative; z-index: 1; display: flex; flex-direction: column; height: 100%; min-height: 0; }
  .head { display: flex; align-items: center; gap: 16px; padding: 44px 28px 12px; }
  .stepper { list-style: none; margin: 0; padding: 3px; display: flex; gap: 2px; background: var(--sunk); border-radius: 11px; }
  .stepper button {
    display: flex; align-items: center; gap: 8px; height: 32px; padding: 0 14px 0 8px; border: 0; border-radius: 8px;
    background: transparent; font-weight: 600; font-size: 13.5px; color: var(--ink-2);
    transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
  }
  .stepper button:hover:not(:disabled) { color: var(--ink); }
  .stepper button:disabled { opacity: 0.45; }
  .stepper button.on { background: var(--surface); color: var(--ink); box-shadow: var(--shadow-lift); }
  .n { width: 20px; height: 20px; border-radius: 50%; display: grid; place-items: center; font-size: 11.5px; font-weight: 700; background: var(--rule); color: var(--ink-2); }
  .on .n { background: var(--seal); color: var(--seal-ink); }
  .done .n { background: var(--seal-soft); color: var(--seal); }
  .right { margin-left: auto; display: flex; align-items: center; gap: 8px; }
  .save { font-size: 12.5px; color: var(--ink-3); }
  .confirm { margin: 0 28px 8px; display: flex; align-items: center; gap: 10px; padding: 10px 12px; border-radius: var(--r-md); background: var(--err-soft); }
  .confirm p { flex: 1; font-size: 13.5px; }
  .body { flex: 1; min-height: 0; }
  .page { padding: 12px 28px 40px; max-width: 1120px; }
  .failed { padding: 48px 28px; display: grid; gap: 12px; justify-items: start; }
  .foot { display: flex; align-items: center; gap: 12px; padding: 12px 28px; border-top: 1px solid var(--rule); background: var(--paper); }
  .grow { flex: 1; }
  .why { font-size: 12.5px; color: var(--ink-3); }
</style>
