import { appState } from "./state.js";
import { request } from "./api.js";
import { select, escapeHTML, showNotice, handleAction } from "./ui.js";
export async function renderSettingsPage() {
  appState.settings = await request("/settings");
  select("#content").innerHTML = `<h1>設定</h1>
      <section class="card">
      <h2>生活の時刻</h2>
      <form id="settings-form">
      <label>タイムゾーン<input name="timezone" value="${escapeHTML(appState.settings.timezone)}" required>
      </label>
      <div class="row">
      <label>目標の就寝時刻<input type="time" name="target_bedtime" value="${escapeHTML((appState.settings.target_bedtime || "").slice(0, 5))}">
      </label>
      <label>PVT の測定時刻<input type="time" name="pvt_reminder_time" value="${escapeHTML((appState.settings.pvt_reminder_time || "").slice(0, 5))}">
      </label>
      </div>
      <p class="muted">MVP では設定を保存し、PVT 画面に表示します。Discord の催促はまだ行いません。</p>
      <button>保存</button>
      </form>
      </section>
      <div id="prompts">
      </div>`;
  select("#settings-form").onsubmit = handleAction(async (e) => {
    e.preventDefault();
    appState.settings = await request(
      "/settings",
      Object.fromEntries(new FormData(e.target)),
      "PUT",
    );
    showNotice("設定を保存しました");
  });
  if (appState.user.is_admin) {
    const prompts = await request("/prompts");
    for (const key of ["job.extract_sleep", "agent.loop"]) {
      const versions = prompts.filter((p) => p.config_key === key),
        active = versions.find((p) => p.is_active);
      const section = document.createElement("section");
      section.className = "card";
      section.innerHTML = `<h2>${key === "agent.loop" ? "Agent loop の指示" : "OCR の指示"}</h2>
      <p>${key === "agent.loop" ? "保存・微調整できます。複雑な agent 機能を実装するまで実行には使いません。" : "新しい OCR job から適用されます。実行済み・再試行の job は元の版を使います。"}</p>
      <form>
      <label>版<select>${versions.map((v) => `<option value="${v.id}" ${v.is_active ? "selected" : ""}>v${v.version}${v.is_active ? "（使用中）" : ""}</option>`).join("")}</select>
      </label>
      <label>指示<textarea required>${escapeHTML(active?.body || "")}</textarea>
      </label>
      <button>新しい版として保存</button>
      </form>`;
      section.querySelector("select").onchange = (e) =>
        (section.querySelector("textarea").value = versions.find(
          (v) => v.id === e.target.value,
        ).body);
      section.querySelector("form").onsubmit = handleAction(async (e) => {
        e.preventDefault();
        await request("/prompts", {
          config_key: key,
          body: section.querySelector("textarea").value,
        });
        showNotice("新しい版を保存しました");
        await renderSettingsPage();
      });
      select("#prompts").append(section);
    }
  }
}
