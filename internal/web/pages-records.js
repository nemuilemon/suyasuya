import { appState } from "./state.js";
import { request } from "./api.js";
import {
  select,
  escapeHTML,
  showNotice,
  handleAction,
  today,
  formatDate,
  clearForm,
} from "./ui.js";
import { openRecordEditor } from "./record-editor.js";
import { refreshJobs, setupJobActions } from "./pages-jobs.js";
export async function renderRecordPage() {
  clearInterval(appState.jobTimer);
  select("#content").innerHTML = `<h1>記録する</h1>
      <p>画像の読み取り結果を確かめて保存。主睡眠も昼寝も、数値だけで記録できます。</p>
      <div class="grid">
      <section class="card">
      <h2>スクリーンショットを読み取る</h2>
      <form id="upload-form">
      <label>画像<input type="file" name="image" accept="image/png,image/jpeg,image/webp" required>
      </label>
      <label>撮影日<input name="date" type="date" value="${today()}" required>
      </label>
      <small>撮影日と睡眠の日付は別です。読み取り後に確認してください。</small>
      <br>
      <button>アップロードして読み取る</button>
      <button type="button" id="clear-upload" class="quiet">クリア</button>
      </form>
      </section>
      <section class="card">
      <h2>数値から記録する</h2>
      <p>画像がない日や、検知されなかった昼寝にも。</p>
      <button id="manual">記録を追加</button>
      <p class="muted">時刻は ${escapeHTML(appState.settings.timezone)} で入力します。</p>
      </section>
      </div>
      <section class="card">
      <h2>読み取り・確認待ち</h2>
      <div class="task-toolbar">
      <label class="task-selection">
      <input type="checkbox" id="select-all-jobs">表示中をすべて選択</label>
      <button id="delete-selected-jobs" class="quiet" disabled>選択したタスクを削除</button>
      <button id="delete-completed-jobs" class="quiet">確認済みをまとめて削除</button>
      </div>
      <div id="jobs">読み込み中…</div>
      </section>`;
  select("#clear-upload").onclick = () => clearForm(select("#upload-form"));
  select("#manual").onclick = () => openRecordEditor({});
  select("#upload-form").onsubmit = handleAction(async (e) => {
    e.preventDefault();
    const button = e.target.querySelector("button");
    button.disabled = true;
    try {
      const f = new FormData(e.target),
        image = f.get("image");
      const upload = await request("/uploads", {
        content_type: image.type,
        size_bytes: image.size,
        capture_date: f.get("date"),
      });
      const put = await fetch(upload.url, {
        method: "PUT",
        headers: upload.headers,
        body: image,
      });
      if (!put.ok)
        throw Error(
          "画像を保存できませんでした。Cloud Storage の CORS・権限設定を確認してください。",
        );
      sessionStorage.setItem("pending-upload", upload.id);
      await request(`/uploads/${upload.id}/complete`, {});
      sessionStorage.removeItem("pending-upload");
      e.target.reset();
      e.target.elements.date.value = today();
      showNotice("読み取りを開始しました。結果を確認して承認してください。");
      await refreshJobs();
    } finally {
      button.disabled = false;
    }
  });

  const pending = sessionStorage.getItem("pending-upload");
  if (pending) {
    try {
      await request(`/uploads/${pending}/complete`, {});
      sessionStorage.removeItem("pending-upload");
    } catch (err) {
      showNotice("画像の完了確認が保留中です: " + err.message);
    }
  }
  setupJobActions();
  await refreshJobs();
  appState.jobTimer = setInterval(
    () => refreshJobs().catch((e) => showNotice(e.message)),
    5000,
  );
}
export async function renderRecordsPage() {
  clearInterval(appState.jobTimer);
  const records = await request("/records");
  select("#content").innerHTML = `<h1>記録一覧</h1>
      <section class="card">
      <div id="records-list">${records.length ? "" : "まだ記録がありません。"}</div>
      </section>`;
  for (const r of records) {
    const el = document.createElement("div");
    el.className = "record";
    el.innerHTML = `<div>
      <span class="tag">${r.sleep_kind === "nap" ? "昼寝" : "主睡眠"}</span> <strong>${escapeHTML(r.sleep_date)}</strong>
      <br>${r.duration_minutes ?? "—"} 分 <small>${escapeHTML(formatDate(r.started_at))} → ${escapeHTML(formatDate(r.ended_at))}</small>
      </div>
      <div class="actions">
      <button class="quiet">編集</button>
      <button class="quiet">履歴</button>
      </div>`;
    const buttons = el.querySelectorAll("button");
    buttons[0].onclick = () => openRecordEditor(r);
    buttons[1].onclick = handleAction(async () => {
      const data = await request(`/records/${r.id}/history`);
      select("#editor-content").innerHTML =
        "<h2>変更履歴</h2><pre>" +
        escapeHTML(JSON.stringify(data, null, 2)) +
        "</pre>";
      select("#editor").showModal();
    });
    select("#records-list").append(el);
  }
}
