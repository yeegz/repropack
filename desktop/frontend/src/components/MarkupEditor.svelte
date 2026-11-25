<script lang="ts">
  // Screenshot markup: boxes that hide (solid, burned into pixels on save),
  // outlines that highlight, and arrows that point. Keyboard: H hide,
  // O outline, A arrow, Cmd-Z undo, Escape closes.
  import { onMount } from "svelte";
  import type { DraftAttachment } from "../lib/types";
  import Icon from "./Icon.svelte";

  let { att, onclose, onsave }: { att: DraftAttachment; onclose: () => void; onsave: (png: string, redactions: number) => void } = $props();
  type Tool = "hide" | "outline" | "arrow";
  type Shape = { tool: Tool; x1: number; y1: number; x2: number; y2: number };
  let tool = $state<Tool>("hide");
  let shapes = $state<Shape[]>([]);
  let draft = $state<Shape | null>(null);
  let img: HTMLImageElement | null = $state(null);
  let canvas: HTMLCanvasElement | undefined = $state();
  let dialog: HTMLDivElement | undefined = $state();
  let ready = $state(false);
  let prevFocus: Element | null = null;

  const tools: { id: Tool; label: string; icon: string; key: string; hint: string }[] = [
    { id: "hide", label: "Hide", icon: "box", key: "H", hint: "Covers an area with solid black. Hidden content is removed from the image." },
    { id: "outline", label: "Outline", icon: "highlight", key: "O", hint: "Draws a box around something to point it out." },
    { id: "arrow", label: "Arrow", icon: "arrow", key: "A", hint: "Points at something." },
  ];

  onMount(() => {
    prevFocus = document.activeElement;
    const i = new Image();
    i.onload = () => { img = i; ready = true; draw(); };
    i.src = att.url;
    dialog?.focus();
    return () => (prevFocus as HTMLElement | null)?.focus?.();
  });

  function color() { return getComputedStyle(document.documentElement).getPropertyValue("--err").trim() || "#C83A2E"; }

  function render(ctx: CanvasRenderingContext2D, list: Shape[], forExport = false) {
    if (!img) return;
    ctx.clearRect(0, 0, img.naturalWidth, img.naturalHeight);
    ctx.drawImage(img, 0, 0);
    const lw = Math.max(3, img.naturalWidth / 260);
    for (const s of list) {
      const x = Math.min(s.x1, s.x2), y = Math.min(s.y1, s.y2), w = Math.abs(s.x2 - s.x1), h = Math.abs(s.y2 - s.y1);
      if (s.tool === "hide") {
        ctx.fillStyle = "#000";
        ctx.fillRect(x, y, w, h);
        if (!forExport) { ctx.strokeStyle = "rgba(255,255,255,.5)"; ctx.setLineDash([6, 4]); ctx.lineWidth = 1.5; ctx.strokeRect(x, y, w, h); ctx.setLineDash([]); }
      } else if (s.tool === "outline") {
        ctx.strokeStyle = color(); ctx.lineWidth = lw; ctx.lineJoin = "round";
        ctx.beginPath(); ctx.roundRect(x, y, w, h, lw * 2); ctx.stroke();
      } else {
        ctx.strokeStyle = color(); ctx.fillStyle = color(); ctx.lineWidth = lw; ctx.lineCap = "round";
        ctx.beginPath(); ctx.moveTo(s.x1, s.y1); ctx.lineTo(s.x2, s.y2); ctx.stroke();
        const a = Math.atan2(s.y2 - s.y1, s.x2 - s.x1), hl = lw * 5;
        ctx.beginPath(); ctx.moveTo(s.x2, s.y2);
        ctx.lineTo(s.x2 - hl * Math.cos(a - 0.45), s.y2 - hl * Math.sin(a - 0.45));
        ctx.lineTo(s.x2 - hl * Math.cos(a + 0.45), s.y2 - hl * Math.sin(a + 0.45));
        ctx.closePath(); ctx.fill();
      }
    }
  }
  function draw() {
    if (!canvas || !img) return;
    canvas.width = img.naturalWidth; canvas.height = img.naturalHeight;
    render(canvas.getContext("2d")!, draft ? [...shapes, draft] : shapes);
  }
  $effect(() => { shapes; draft; draw(); });

  function pt(e: PointerEvent) {
    const r = canvas!.getBoundingClientRect();
    return { x: ((e.clientX - r.left) / r.width) * canvas!.width, y: ((e.clientY - r.top) / r.height) * canvas!.height };
  }
  function down(e: PointerEvent) { const p = pt(e); draft = { tool, x1: p.x, y1: p.y, x2: p.x, y2: p.y }; canvas!.setPointerCapture(e.pointerId); }
  function move(e: PointerEvent) { if (draft) { const p = pt(e); draft = { ...draft, x2: p.x, y2: p.y }; } }
  function up() {
    if (draft && (Math.abs(draft.x2 - draft.x1) > 4 || Math.abs(draft.y2 - draft.y1) > 4)) shapes = [...shapes, draft];
    draft = null;
  }
  function key(e: KeyboardEvent) {
    if (e.key === "Escape") { e.preventDefault(); onclose(); return; }
    if ((e.metaKey || e.ctrlKey) && e.key === "z") { e.preventDefault(); shapes = shapes.slice(0, -1); return; }
    if (e.target instanceof HTMLInputElement) return;
    const t = tools.find((t) => t.key.toLowerCase() === e.key.toLowerCase());
    if (t && !e.metaKey && !e.ctrlKey) tool = t.id;
    if (e.key === "Tab") trap(e);
  }
  function trap(e: KeyboardEvent) {
    const f = [...dialog!.querySelectorAll<HTMLElement>("button:not(:disabled), [tabindex='0']")];
    if (!f.length) return;
    const i = f.indexOf(document.activeElement as HTMLElement);
    if (e.shiftKey && i <= 0) { e.preventDefault(); f[f.length - 1].focus(); }
    else if (!e.shiftKey && i === f.length - 1) { e.preventDefault(); f[0].focus(); }
  }
  function save() {
    const c = document.createElement("canvas");
    c.width = img!.naturalWidth; c.height = img!.naturalHeight;
    render(c.getContext("2d")!, shapes, true);
    onsave(c.toDataURL("image/png"), shapes.filter((s) => s.tool === "hide").length);
  }
  const hidden = $derived(shapes.filter((s) => s.tool === "hide").length);
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="dlg" role="dialog" aria-modal="true" aria-labelledby="mk-t" tabindex="-1" bind:this={dialog} onkeydown={key}>
    <header>
      <h2 id="mk-t">Mark up {att.title}</h2>
      <div class="tools" role="radiogroup" aria-label="Drawing tool">
        {#each tools as t}
          <button role="radio" aria-checked={tool === t.id} class:on={tool === t.id} onclick={() => (tool = t.id)} title="{t.hint} ({t.key})"><Icon name={t.icon} size={16} />{t.label}<kbd>{t.key}</kbd></button>
        {/each}
      </div>
      <button class="btn quiet" onclick={() => (shapes = shapes.slice(0, -1))} disabled={!shapes.length}><Icon name="undo" size={16} />Undo</button>
    </header>
    <div class="stage">
      {#if ready}
        <canvas bind:this={canvas} onpointerdown={down} onpointermove={move} onpointerup={up} aria-label="Screenshot. Drag to draw with the {tool} tool."></canvas>
      {:else}
        <p class="muted">Loading image…</p>
      {/if}
    </div>
    <footer>
      <p class="hint">{tools.find((t) => t.id === tool)!.hint}{#if hidden}{" "}{hidden} {hidden === 1 ? "area" : "areas"} will be blacked out for good when you save.{/if}</p>
      <button class="btn" onclick={onclose}>Cancel</button>
      <button class="btn primary" onclick={save} disabled={!ready}>Save markup</button>
    </footer>
  </div>
</div>

<style>
  .scrim { position: fixed; inset: 0; z-index: 40; background: rgba(8, 10, 9, 0.55); display: grid; place-items: center; padding: 32px; animation: fade var(--t-med) var(--ease); }
  .dlg { width: min(1180px, 100%); max-height: 100%; display: grid; grid-template-rows: auto 1fr auto; background: var(--raised); border-radius: var(--r-lg); box-shadow: var(--shadow-pop); overflow: hidden; animation: pop var(--t-med) var(--ease); }
  .dlg:focus-visible { outline: none; }
  header, footer { display: flex; align-items: center; gap: 12px; padding: 12px 16px; }
  header { border-bottom: 1px solid var(--rule); }
  footer { border-top: 1px solid var(--rule); }
  h2 { font-size: 15px; flex: 1; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .tools { display: flex; gap: 2px; padding: 3px; background: var(--sunk); border-radius: 10px; }
  .tools button { display: flex; align-items: center; gap: 6px; height: 30px; padding: 0 11px; border: 0; background: transparent; border-radius: 7px; font-weight: 600; font-size: 13px; color: var(--ink-2); }
  .tools button.on { background: var(--surface); color: var(--ink); box-shadow: var(--shadow-lift); }
  kbd { font: 500 11px var(--font); color: var(--ink-3); }
  .stage { min-height: 0; overflow: auto; display: grid; place-items: center; background: var(--sunk); padding: 16px; }
  canvas { max-width: 100%; max-height: calc(100vh - 220px); cursor: crosshair; box-shadow: var(--shadow-lift); touch-action: none; }
  .hint { flex: 1; font-size: 12.5px; color: var(--ink-3); }
  @keyframes fade { from { opacity: 0; } }
  @keyframes pop { from { transform: scale(0.98); opacity: 0; } }
</style>
