import { appState } from "./state.js";
import { request } from "./api.js";
import {
  select,
  escapeHTML,
  showNotice,
  handleAction,
  newID,
  formatDate,
} from "./ui.js";
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
      <h2>Discord Webhook</h2>
      <p>${appState.settings.webhook_configured ? "設定済み" : "未設定"}</p>
      <label>Webhook URL<input type="password" name="discord_webhook_url" autocomplete="new-password" placeholder="${appState.settings.webhook_configured ? "変更する場合のみ入力" : "https://discord.com/api/webhooks/…"}"></label>
      ${appState.settings.webhook_configured ? '<label><input type="checkbox" name="clear_webhook">Webhook 設定を削除</label>' : ""}
      <label><input type="checkbox" name="notifications_enabled" ${appState.settings.notifications_enabled ? "checked" : ""}>自動通知を有効にする</label>
      <p class="muted">就寝時刻・PVT 測定時刻に各1日1回送ります。時刻を空欄にすると、その通知を止められます。</p>
      <button>保存</button>
      </form>
      </section>
      <section class="card">
      <h2>通知</h2>
      <button id="test-notification" ${appState.settings.webhook_configured ? "" : "disabled"}>接続テストを送信</button>
      <button id="refresh-notifications" class="secondary">履歴を更新</button>
      <div id="notification-history"></div>
      </section>
      <div id="prompts">
      </div>`;
  select("#settings-form").onsubmit = handleAction(async (e) => {
    e.preventDefault();
    appState.settings = await request(
      "/settings",
      {
        ...Object.fromEntries(new FormData(e.target)),
        clear_webhook: new FormData(e.target).has("clear_webhook"),
        notifications_enabled: new FormData(e.target).has(
          "notifications_enabled",
        ),
      },
      "PUT",
    );
    showNotice("設定を保存しました");
    await renderSettingsPage();
  });
  select("#test-notification").onclick = handleAction(async () => {
    await request("/notifications/test", { submission_id: newID() });
    showNotice("接続テストを送信待ちにしました");
    await renderNotificationHistory();
    setTimeout(() => {
      if (select("#notification-history"))
        renderNotificationHistory().catch(() => {});
    }, 6000);
  });
  select("#refresh-notifications").onclick = handleAction(
    renderNotificationHistory,
  );
  await renderNotificationHistory();
  if (appState.user.is_admin) {
    const prompts = await request("/prompts");
    for (const key of [
      "job.extract_sleep",
      "agent.loop",
      "job.plan_reminder",
    ]) {
      const versions = prompts.filter((p) => p.config_key === key),
        active = versions.find((p) => p.is_active);
      const section = document.createElement("section");
      section.className = "card";
      section.innerHTML = `<h2>${{ "agent.loop": "Agent loop の指示", "job.extract_sleep": "OCR の指示", "job.plan_reminder": "催促の指示" }[key]}</h2>
      <p>${key === "job.extract_sleep" ? "新しい画像の読み取りから適用されます。" : "次に作成する催促から適用されます。"}</p>
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

async function renderNotificationHistory() {
  const notifications = await request("/notifications");
  const container = select("#notification-history");
  if (!container) return;
  const statuses = {
    pending: "送信待ち",
    running: "文面を作成中",
    sending: "送信中",
    sent: "送信済み",
    failed: "失敗",
    cancelled: "取り消し",
  };
  const kinds = { bedtime: "就寝の催促", pvt: "PVT", feedback: "接続テスト" };
  container.innerHTML = notifications.length
    ? notifications
        .map(
          (n) => `<article>
 <h3>${escapeHTML(kinds[n.kind] || n.kind)} · ${escapeHTML(statuses[n.status] || n.status)}</h3>
 <small>${escapeHTML(formatDate(n.sent_at || n.scheduled_at))}</small>
 <p>${escapeHTML(n.body)}</p>
 ${n.error ? `<p>${escapeHTML(n.error)}</p>` : ""}
 </article>`,
        )
        .join("")
    : '<p class="muted">通知履歴はまだありません。</p>';
}
