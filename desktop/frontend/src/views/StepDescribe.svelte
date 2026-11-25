<script lang="ts">
  import type { Draft, DraftText } from "../lib/types";
  import Icon from "../components/Icon.svelte";

  let { draft, onchange }: { draft: Draft; onchange: (t: DraftText) => void } = $props();
  let title = $state(draft.title);
  let actual = $state(draft.actual);
  let expected = $state(draft.expected);
  let description = $state(draft.description);
  let frequency = $state(draft.frequency);
  let steps = $state<string[]>(draft.steps.length ? [...draft.steps] : [""]);
  const freqs = ["Every time", "Sometimes", "Once"];

  function emit() {
    onchange({ title, actual, expected, description, frequency, steps: steps.map((s) => s.trim()).filter(Boolean) });
  }
  function addStep(after = steps.length - 1) {
    steps = [...steps.slice(0, after + 1), "", ...steps.slice(after + 1)];
    queueMicrotask(() => (document.getElementById(`step-${after + 1}`) as HTMLInputElement)?.focus());
  }
  function removeStep(i: number) {
    steps = steps.length === 1 ? [""] : steps.filter((_, j) => j !== i);
    emit();
    queueMicrotask(() => (document.getElementById(`step-${Math.max(0, i - 1)}`) as HTMLInputElement)?.focus());
  }
  function move(i: number, d: number) {
    const j = i + d;
    if (j < 0 || j >= steps.length) return;
    const s = [...steps];
    [s[i], s[j]] = [s[j], s[i]];
    steps = s;
    emit();
    queueMicrotask(() => (document.getElementById(`step-${j}`) as HTMLInputElement)?.focus());
  }
  function stepKey(e: KeyboardEvent, i: number) {
    if (e.key === "Enter") { e.preventDefault(); addStep(i); }
    else if (e.key === "Backspace" && steps[i] === "" && steps.length > 1) { e.preventDefault(); removeStep(i); }
    else if (e.altKey && e.key === "ArrowUp") { e.preventDefault(); move(i, -1); }
    else if (e.altKey && e.key === "ArrowDown") { e.preventDefault(); move(i, 1); }
  }
</script>

<div class="describe">
  <div class="col">
    <label class="field titlefield">
      <span>In one line, what's wrong?</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input title" bind:value={title} oninput={emit} maxlength="300" placeholder="For example: Export to PDF freezes on long invoices" autofocus={!draft.title} />
    </label>

    <div class="pair">
      <label class="field">
        <span>What happened</span>
        <textarea class="textarea" bind:value={actual} oninput={emit} rows="4" placeholder="The progress bar stopped at 80% and the window stopped responding."></textarea>
      </label>
      <label class="field">
        <span>What you expected</span>
        <textarea class="textarea" bind:value={expected} oninput={emit} rows="4" placeholder="The PDF saves in a few seconds."></textarea>
      </label>
    </div>

    <fieldset class="field steps">
      <legend><span>Steps to make it happen again</span><small>Press Return for a new step. Option-arrow keys reorder.</small></legend>
      <ol>
        {#each steps as _, i}
          <li>
            <span class="n" aria-hidden="true">{i + 1}</span>
            <input id="step-{i}" class="input" bind:value={steps[i]} oninput={emit} onkeydown={(e) => stepKey(e, i)} aria-label="Step {i + 1}" placeholder={i === 0 ? "Open an invoice with more than 200 lines" : ""} />
            <button class="btn quiet icon" aria-label="Remove step {i + 1}" onclick={() => removeStep(i)}><Icon name="x" size={14} /></button>
          </li>
        {/each}
      </ol>
      <button class="btn quiet add" onclick={() => addStep()}><Icon name="plus" size={15} />Add a step</button>
    </fieldset>
  </div>

  <aside class="col side">
    <fieldset class="field">
      <legend><span>How often does it happen?</span></legend>
      <div class="chips" role="radiogroup" aria-label="How often">
        {#each freqs as f}
          <label class="chip" class:on={frequency === f}>
            <input type="radio" name="freq" value={f} bind:group={frequency} onchange={emit} />{f}
          </label>
        {/each}
      </div>
    </fieldset>
    <label class="field">
      <span>Anything else</span>
      <textarea class="textarea" bind:value={description} oninput={emit} rows="6" placeholder="When it started, what you already tried, error messages you saw."></textarea>
      <small>You can review and hide anything sensitive before sharing.</small>
    </label>
  </aside>
</div>

<style>
  .describe { display: grid; grid-template-columns: minmax(0, 1fr) 300px; gap: 36px; padding-top: 16px; }
  .col { display: grid; gap: 22px; align-content: start; }
  .title { font-size: 20px; font-weight: 600; padding: 11px 14px; letter-spacing: -0.01em; }
  .titlefield > span { font-size: 14px; color: var(--ink); }
  .pair { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  fieldset { border: 0; margin: 0; padding: 0; min-width: 0; }
  legend { display: grid; gap: 2px; padding: 0; margin-bottom: 8px; }
  legend span { font-weight: 600; font-size: 13px; color: var(--ink-2); }
  legend small { color: var(--ink-3); font-size: 12.5px; }
  .steps ol { list-style: none; margin: 0; padding: 0; display: grid; gap: 6px; }
  .steps li { display: grid; grid-template-columns: 26px 1fr auto; align-items: center; gap: 6px; }
  .steps .n { font-weight: 700; font-size: 13px; color: var(--ink-3); text-align: right; font-variant-numeric: tabular-nums; }
  .add { justify-self: start; margin-top: 4px; margin-left: 26px; }
  .chips { display: flex; gap: 6px; flex-wrap: wrap; }
  .chip { display: inline-flex; align-items: center; height: 32px; padding: 0 13px; border: 1px solid var(--rule-2); border-radius: 999px; font-weight: 600; font-size: 13px; background: var(--surface); }
  .chip input { position: absolute; opacity: 0; pointer-events: none; }
  .chip.on { background: var(--seal-soft); border-color: var(--seal); color: var(--seal); }
  .chip:has(input:focus-visible) { outline: 2px solid var(--focus); outline-offset: 2px; }
  @media (max-width: 1100px) { .describe { grid-template-columns: 1fr; } .pair { grid-template-columns: 1fr; } }
</style>
