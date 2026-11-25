<script lang="ts">
  // A small, consistent icon set drawn for ReproPack (1.6px strokes on a 20px grid).
  let { name, size = 18, label = "" }: { name: string; size?: number; label?: string } = $props();
  const paths: Record<string, string> = {
    pack: "M3.5 6.5h13v9a2 2 0 0 1-2 2h-9a2 2 0 0 1-2-2zM6 6.5V5a1.5 1.5 0 0 1 1.5-1.5h5A1.5 1.5 0 0 1 14 5v1.5M7 10.5h6M7 13.5h3.5",
    plus: "M10 4v12M4 10h12",
    open: "M3 6.5A1.5 1.5 0 0 1 4.5 5H8l1.5 1.5h6A1.5 1.5 0 0 1 17 8v6.5a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 3 14.5z",
    window: "M3 5.5A1.5 1.5 0 0 1 4.5 4h11A1.5 1.5 0 0 1 17 5.5v9a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 3 14.5zM3 7.5h14",
    area: "M3.5 7V4.5a1 1 0 0 1 1-1H7M13 3.5h2.5a1 1 0 0 1 1 1V7M16.5 13v2.5a1 1 0 0 1-1 1H13M7 16.5H4.5a1 1 0 0 1-1-1V13",
    record: "M10 15.5a5.5 5.5 0 1 0 0-11 5.5 5.5 0 0 0 0 11z",
    stop: "M6.5 5.5h7a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-7a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z",
    file: "M11.5 3H6a1.5 1.5 0 0 0-1.5 1.5v11A1.5 1.5 0 0 0 6 17h8a1.5 1.5 0 0 0 1.5-1.5V7zM11.5 3v4h4M10 10v4.5M7.75 12.25 10 10l2.25 2.25",
    pulse: "M2.5 10.5h3l2-5 3.5 9 2-4h4.5",
    flag: "M5 17V3.5M5 4h9l-2 3.25L14 10.5H5",
    note: "M5 3.5h10A1.5 1.5 0 0 1 16.5 5v7.5l-4 4H5A1.5 1.5 0 0 1 3.5 15V5A1.5 1.5 0 0 1 5 3.5zM16.5 12.5h-3a1 1 0 0 0-1 1v3M6.5 7.5h7M6.5 10.5h4",
    shield: "M10 3 4 5.5v4.2c0 3.6 2.5 6.3 6 7.3 3.5-1 6-3.7 6-7.3V5.5zM7.5 10l1.75 1.75L12.75 8.25",
    share: "M10 12.5V3.5M6.75 6.75 10 3.5l3.25 3.25M4.5 10.5v4.5a1.5 1.5 0 0 0 1.5 1.5h8a1.5 1.5 0 0 0 1.5-1.5v-4.5",
    check: "M4.5 10.5 8 14l7.5-8",
    x: "M5.5 5.5l9 9M14.5 5.5l-9 9",
    trash: "M4 6h12M8 6V4.5h4V6M5.5 6l.75 10a1 1 0 0 0 1 .9h5.5a1 1 0 0 0 1-.9L14.5 6",
    eye: "M2.5 10s2.75-5 7.5-5 7.5 5 7.5 5-2.75 5-7.5 5-7.5-5-7.5-5zM10 12.25a2.25 2.25 0 1 0 0-4.5 2.25 2.25 0 0 0 0 4.5z",
    pen: "M12.5 4.5 15.5 7.5 7.5 15.5H4.5V12.5zM11 6l3 3",
    box: "M4 5h12v10H4z",
    arrow: "M4.5 15.5 15 5M9 5h6v6",
    highlight: "M3.5 4.5h13v11h-13z",
    undo: "M7.5 6 4 9.5 7.5 13M4 9.5h8a4 4 0 0 1 0 8h-1",
    gear: "M10 12.5a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5zM10 2.5v2M10 15.5v2M17.5 10h-2M4.5 10h-2M15.3 4.7l-1.4 1.4M6.1 13.9l-1.4 1.4M15.3 15.3l-1.4-1.4M6.1 6.1 4.7 4.7",
    warn: "M10 3.5 2.75 16h14.5zM10 8.5v3.5M10 14.25v.25",
    clock: "M10 17a7 7 0 1 0 0-14 7 7 0 0 0 0 14zM10 6v4l2.5 2",
    link: "M8.5 11.5a3 3 0 0 0 4.24 0l2.5-2.5a3 3 0 0 0-4.24-4.24l-.75.75M11.5 8.5a3 3 0 0 0-4.24 0l-2.5 2.5a3 3 0 0 0 4.24 4.24l.75-.75",
    search: "M9 15a6 6 0 1 0 0-12 6 6 0 0 0 0 12zM13.5 13.5 17 17",
    chevron: "M7.5 4.5 13 10l-5.5 5.5",
    back: "M12.5 4.5 7 10l5.5 5.5",
    html: "M7 6.5 3.5 10 7 13.5M13 6.5l3.5 3.5-3.5 3.5M11.25 4.5l-2.5 11",
    lock: "M5.5 9h9a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1zM7 9V6.5a3 3 0 0 1 6 0V9",
  };
</script>

<svg width={size} height={size} viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"
  role={label ? "img" : undefined} aria-label={label || undefined} aria-hidden={label ? undefined : "true"} focusable="false">
  {#if name === "record"}
    <circle cx="10" cy="10" r="5.5" fill="currentColor" stroke="none" />
  {:else}
    <path d={paths[name] ?? ""} />
  {/if}
</svg>
