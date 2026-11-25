import "./app.css";
import { mount } from "svelte";
import App from "./App.svelte";

// Follow system light/dark and contrast settings.
const dark = window.matchMedia("(prefers-color-scheme: dark)");
const more = window.matchMedia("(prefers-contrast: more)");
const apply = () => {
  document.documentElement.dataset.theme = dark.matches ? "dark" : "light";
  document.documentElement.dataset.contrast = more.matches ? "more" : "normal";
};
apply();
dark.addEventListener("change", apply);
more.addEventListener("change", apply);

export default mount(App, { target: document.getElementById("app")! });
