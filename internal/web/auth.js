import { appState } from "./state.js";
import { request } from "./api.js";
import { select, showNotice, handleAction } from "./ui.js";
import { render } from "./router.js";
import { isMeasuring } from "./pages-pvt.js";
function showLogin() {
  clearInterval(appState.jobTimer);
  appState.user = null;
  select("#app").hidden = true;
  select("#login").hidden = false;
  select("#logout").hidden = true;
  select("#session-loading").hidden = true;
  if (select("#editor").open) select("#editor").close();
}
export async function restoreSession() {
  select("#login").hidden = true;
  select("#session-loading").hidden = false;
  try {
    const me = await request("/me");
    appState.settings = await request("/settings");
    appState.user = me.user;
    select("#app").hidden = false;
    select("#logout").hidden = false;
    await render();
  } catch (error) {
    if (error.status === 401) showLogin();
    else {
      showNotice(error.message);
      select("#retry-session").hidden = false;
    }
  } finally {
    select("#session-loading").hidden = true;
  }
}
export function setupAuth() {
  window.addEventListener("session-expired", showLogin);
  select("#retry-session").onclick = () => {
    select("#retry-session").hidden = true;
    restoreSession();
  };
  select("#login-form").onsubmit = handleAction(async (event) => {
    event.preventDefault();
    await request("/login", Object.fromEntries(new FormData(event.target)));
    event.target.reset();
    showNotice();
    await restoreSession();
  });
  select("#logout").onclick = handleAction(async () => {
    if (isMeasuring()) return;
    await request("/logout", {});
    showLogin();
  });
}
