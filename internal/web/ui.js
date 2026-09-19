import { appState } from "./state.js";
export const select = (s) => document.querySelector(s),
  escapeHTML = (s) =>
    String(s ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
export const newID = () => crypto.randomUUID(),
  today = () => localDate(new Date()),
  localDate = (d) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

export function showNotice(s = "") {
  select("#notice").textContent = s;
}
export function handleAction(fn) {
  return async (e) => {
    try {
      await fn(e);
    } catch (err) {
      showNotice(err.message);
    }
  };
}
export function formatDate(s) {
  if (!s) return "—";
  return new Date(s).toLocaleString("ja-JP", {
    timeZone: appState.settings.timezone,
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
export function localInput(s) {
  if (!s) return "";
  const parts = new Intl.DateTimeFormat("sv-SE", {
    timeZone: appState.settings.timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(new Date(s));
  return parts.replace(" ", "T");
}
export function refreshPage() {
  window.dispatchEvent(new Event("refresh-page"));
}
export function clearForm(form) {
  for (const field of form.elements) {
    if (field.matches("input,select,textarea")) {
      if (field.type === "checkbox" || field.type === "radio")
        field.checked = false;
      else field.value = "";
    }
  }
}
