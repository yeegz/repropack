<script lang="ts">
  import { app } from "../lib/state.svelte";
  import Icon from "../components/Icon.svelte";
  import Tape from "../components/Tape.svelte";
  import type { RPEvent } from "../lib/types";

  let { onnew, onopen }: { onnew: () => void; onopen: () => void } = $props();

  // A small, made-up example of what a captured report looks like.
  const t0 = Date.UTC(2026, 0, 1, 9, 0, 0);
  const iso = (s: number) => new Date(t0 + s * 1000).toISOString();
  let n = 0;
  const ev = (s: number, e: Partial<RPEvent>): RPEvent => ({ seq: ++n, ts: iso(s), kind: "log", ...e }) as RPEvent;
  const sample: RPEvent[] = [
    ...Array.from({ length: 60 }, (_, i) => ev(i, { kind: "metric", metric: { name: "cpu", value: i > 38 ? 88 + ((i * 7) % 11) : 9 + ((i * 13) % 9), unit: "%" } })),
    ...Array.from({ length: 14 }, (_, i) => ev(4 + i * 2.6, { level: "info" })),
    ev(12, { kind: "breadcrumb" }), ev(21, { kind: "breadcrumb" }), ev(36, { kind: "breadcrumb" }),
    ev(39, { level: "warn" }), ev(43, { level: "warn" }),
    ev(44, { kind: "marker", source: "you" }),
    ev(47, { kind: "error", level: "error" }), ev(47.5, { kind: "error", level: "error" }),
  ];
  const steps = [
    { icon: "note", t: "Describe", d: "Say what you expected and what happened instead." },
    { icon: "window", t: "Capture", d: "Screenshot a window, record your screen, add logs, or watch the app." },
    { icon: "shield", t: "Review", d: "See every email, key and path that was found, and hide what you need to." },
    { icon: "share", t: "Share", d: "Save one file. Send it, or attach it to an issue yourself." },
  ];
</script>

<div class="home scroll">
  <div class="inner">
    <h1>What went wrong?</h1>
    <p class="lede">Collect what a developer needs to fix it: what you did, what you saw, and what the app was doing at the time. Everything stays on this Mac until you decide to share it.</p>
    <div class="cta">
      <button class="btn primary big" onclick={onnew}><Icon name="plus" size={18} />Start a report</button>
      <button class="btn big" onclick={onopen}><Icon name="open" size={18} />Open a report someone sent you</button>
    </div>

    <figure class="specimen" aria-labelledby="spec-cap">
      <Tape start={iso(0)} end={iso(60)} events={sample} pins={[{ id: "p", at: iso(45), label: "Screenshot", kind: "shot" }, { id: "q", at: iso(50), label: "Note", kind: "note" }]} />
      <figcaption id="spec-cap">An example report: the app slowed down, you marked the moment, and two errors followed.</figcaption>
    </figure>

    <ol class="steps">
      {#each steps as s, i}
        <li>
          <span class="num" aria-hidden="true">{i + 1}</span>
          <div><h2>{s.t}</h2><p>{s.d}</p></div>
        </li>
      {/each}
    </ol>

    {#if app.env && !app.env.capabilities.screenshots}
      <p class="note"><Icon name="warn" size={16} />{app.env.capabilities.note}</p>
    {/if}
  </div>
</div>

<style>
  .home { flex: 1; position: relative; z-index: 1; }
  .inner { max-width: 840px; padding: 76px 48px 64px; }
  h1 { font-size: clamp(34px, 4.4vw, 48px); line-height: 1.04; letter-spacing: -0.03em; font-weight: 700; }
  .lede { margin-top: 16px; font-size: 16.5px; line-height: 1.55; color: var(--ink-2); max-width: 58ch; }
  .cta { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 28px; }
  .specimen { margin: 48px 0 0; padding: 18px 18px 12px; background: var(--surface); border: 1px solid var(--rule); border-radius: var(--r-lg); }
  figcaption { margin-top: 10px; font-size: 12.5px; color: var(--ink-3); }
  .steps { list-style: none; padding: 0; margin: 40px 0 0; display: grid; grid-template-columns: repeat(4, 1fr); gap: 22px; }
  .steps li { display: grid; grid-template-columns: auto 1fr; gap: 10px; align-items: start; }
  .num { width: 24px; height: 24px; border-radius: 50%; border: 1.5px solid var(--rule-2); display: grid; place-items: center; font-size: 12.5px; font-weight: 700; color: var(--ink-2); }
  .steps h2 { font-size: 14.5px; }
  .steps p { margin-top: 3px; font-size: 13px; color: var(--ink-3); line-height: 1.45; }
  .note { margin-top: 32px; display: flex; gap: 8px; align-items: center; color: var(--warn); font-size: 13.5px; }
  @media (max-width: 1100px) { .steps { grid-template-columns: repeat(2, 1fr); } .inner { padding: 64px 32px; } }
</style>
