<script lang="ts">
  import { onMount } from "svelte";
  import { api, on } from "./lib/api";
  import { app } from "./lib/state.svelte";
  import Sidebar from "./components/Sidebar.svelte";
  import Toasts from "./components/Toasts.svelte";
  import RecordingBar from "./components/RecordingBar.svelte";
  import Home from "./views/Home.svelte";
  import DraftView from "./views/DraftView.svelte";
  import ReportViewer from "./views/ReportViewer.svelte";
  import Settings from "./views/Settings.svelte";

  async function newReport() {
    try {
      const d = await api.newDraft();
      app.refreshLibrary();
      app.go({ name: "draft", id: d.id, step: 0 });
    } catch (e) { app.error(e); }
  }
  async function openReport() {
    try {
      const v = await api.openReportDialog();
      if (v) { app.refreshLibrary(); app.go({ name: "report", token: v.token }); reports.set(v.token, v); }
    } catch (e) { app.error(e); }
  }
  // Opened reports are kept here so the viewer can re-render without re-reading.
  import { reports } from "./lib/reports";

  onMount(() => {
    api.environment().then((e) => (app.env = e)).catch(() => {});
    const offMenu = on("menu", (what: string) => (what === "new" ? newReport() : openReport()));
    const offRec = on("recording", (d: any) => {
      if (d.state === "started") { app.recordingDraft = d.draft; app.recordingSince = Date.now(); }
      else { app.recordingDraft = null; app.refreshLibrary(); }
    });
    const key = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey)) return;
      if (e.key === "n") { e.preventDefault(); newReport(); }
      if (e.key === "o") { e.preventDefault(); openReport(); }
      if (e.key === ",") { e.preventDefault(); app.go({ name: "settings" }); }
    };
    window.addEventListener("keydown", key);
    return () => { offMenu(); offRec(); window.removeEventListener("keydown", key); };
  });
</script>

<a class="skip" href="#main">Skip to main content</a>
<div class="frame">
  <Sidebar onnew={newReport} onopen={openReport} />
  <main class="main" id="main" tabindex="-1">
    <div class="drag" style="--wails-draggable:drag" aria-hidden="true"></div>
    {#if app.recordingDraft}<RecordingBar />{/if}
    {#key app.route}
      {#if app.route.name === "home"}
        <Home onnew={newReport} onopen={openReport} />
      {:else if app.route.name === "draft"}
        <DraftView id={app.route.id} initialStep={app.route.step ?? 0} />
      {:else if app.route.name === "report"}
        <ReportViewer token={app.route.token} />
      {:else if app.route.name === "settings"}
        <Settings />
      {/if}
    {/key}
  </main>
</div>
<Toasts />

<style>
  .skip { position: fixed; inset: 12px auto auto 12px; z-index: 20; padding: 10px 14px; border-radius: 8px; background: var(--surface); color: var(--ink); transform: translateY(calc(-100% - 18px)); }
  .skip:focus { transform: none; }
  .frame { display: grid; grid-template-columns: 264px 1fr; height: 100vh; }
  .main { position: relative; min-width: 0; height: 100vh; display: flex; flex-direction: column; background: var(--paper); }
  .drag { position: absolute; inset: 0 0 auto 0; height: 38px; z-index: 0; }
  @media (max-width: 980px) { .frame { grid-template-columns: 220px 1fr; } }
</style>
