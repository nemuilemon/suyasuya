import { appState } from "./state.js";
import { showNotice, handleAction } from "./ui.js";
import { renderRecordPage, renderRecordsPage } from "./pages-records.js";
import { renderSettingsPage } from "./pages-settings.js";
import { renderPVTPage, isMeasuring } from "./pages-pvt.js";
import { renderAnalyticsPage } from "./pages-analytics.js";
import { renderAgentPage } from "./pages-agent.js";
const routes = {
  analytics: renderAnalyticsPage,
  record: renderRecordPage,
  records: renderRecordsPage,
  settings: renderSettingsPage,
  pvt: renderPVTPage,
  agent: renderAgentPage,
};
export async function render() {
  clearInterval(appState.jobTimer);
  if (!routes[appState.page]) appState.page = "records";
  document
    .querySelectorAll("[data-page]")
    .forEach((button) =>
      button.setAttribute(
        "aria-current",
        String(button.dataset.page === appState.page),
      ),
    );
  await routes[appState.page]();
}
export function setupRouter() {
  document.querySelectorAll("[data-page]").forEach(
    (button) =>
      (button.onclick = () => {
        if (isMeasuring()) {
          showNotice("測定を終了してから移動してください");
          return;
        }
        location.hash = "/" + button.dataset.page;
      }),
  );
  window.addEventListener(
    "hashchange",
    handleAction(async () => {
      const next = location.hash.slice(2) || "records";
      if (isMeasuring() && next !== appState.page) {
        history.replaceState(null, "", "#/" + appState.page);
        return;
      }
      appState.page = next;
      showNotice();
      if (appState.user) await render();
    }),
  );
  window.addEventListener(
    "refresh-page",
    handleAction(async () => {
      if (appState.user) await render();
    }),
  );
}
