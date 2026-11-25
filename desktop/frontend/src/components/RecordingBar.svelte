<script lang="ts">
  // Always-visible recording control: the app never records out of sight.
  import { api } from "../lib/api";
  import { app } from "../lib/state.svelte";
  import Icon from "./Icon.svelte";
  let now = $state(Date.now());
  $effect(() => { const t = setInterval(() => (now = Date.now()), 500); return () => clearInterval(t); });
  const secs = $derived(Math.max(0, Math.floor((now - app.recordingSince) / 1000)));
  const label = $derived(`${Math.floor(secs / 60)}:${String(secs % 60).padStart(2, "0")}`);
  let stopping = $state(false);
  async function stop() {
    stopping = true;
    try { await api.stopRecording(); app.toast("Recording saved to the report", "ok"); }
    catch (e) { app.error(e); }
    finally { stopping = false; app.recordingDraft = null; app.refreshLibrary(); }
  }
</script>

<div class="bar" role="status" aria-live="polite">
  <span class="pulse" aria-hidden="true"><Icon name="record" size={14} /></span>
  <span class="t">Recording your screen</span>
  <span class="mono time" aria-label="{secs} seconds">{label}</span>
  <span class="hint">Stops automatically after 5 minutes. You can also stop it from the menu bar.</span>
  <button class="btn stop" onclick={stop} disabled={stopping}><Icon name="stop" size={15} />{stopping ? "Saving…" : "Stop recording"}</button>
</div>

<style>
  .bar {
    position: relative; z-index: 3; margin: 44px 24px 0; display: flex; align-items: center; gap: 12px;
    background: color-mix(in oklab, var(--rec) 10%, var(--surface)); border: 1px solid color-mix(in oklab, var(--rec) 40%, transparent);
    border-radius: var(--r-md); padding: 8px 8px 8px 14px;
  }
  .pulse { color: var(--rec); display: inline-flex; animation: pulse 1.6s ease-in-out infinite; }
  .t { font-weight: 650; }
  .time { color: var(--rec); font-weight: 500; }
  .hint { color: var(--ink-3); font-size: 12.5px; flex: 1; }
  .stop { border-color: var(--rec); color: var(--rec); }
  @keyframes pulse { 50% { opacity: 0.35; } }
  @media (max-width: 1100px) { .hint { display: none; } .t { flex: 1; } }
</style>
