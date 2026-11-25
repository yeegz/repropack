import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

export default defineConfig({
  plugins: [svelte()],
  build: { target: "safari15", assetsInlineLimit: 0 },
  test: { environment: "node" },
} as any);
