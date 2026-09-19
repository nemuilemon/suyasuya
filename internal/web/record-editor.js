import { appState } from "./state.js";
import { request } from "./api.js";
import {
  select,
  escapeHTML,
  showNotice,
  handleAction,
  newID,
  localInput,
  refreshPage,
  clearForm,
} from "./ui.js";
export function openCandidateEditor(job, record, candidateIndex) {
  openRecordEditor(
    {
      source_job_id: job.id,
      candidate_index: candidateIndex,
      sleep_date: "",
      sleep_kind: record.sleep_kind || "",
      duration_minutes: record.duration_minutes,
    },
    { ...job.output, selected: record, upload_id: job.upload_id },
  );
}
export function openRecordEditor(record, ocr) {
  const isEdit = !!record.id;
  const html = `<h2>${isEdit ? "記録を編集" : ocr ? "読み取り結果を確認" : "睡眠を記録"}</h2>${
    ocr
      ? `<p>画像を見ながら、日付・主睡眠／昼寝・日時を確認してください。表示された時刻は下の抽出結果にあります。</p>
      <details open>
      <summary>抽出結果</summary>
      <pre>${escapeHTML(`表示日付: ${ocr.displayed_date || "不明"}\n開始: ${ocr.selected.start_time || "不明"} / 終了: ${ocr.selected.end_time || "不明"}\n睡眠時間: ${ocr.selected.duration_minutes ?? "不明"} 分\n${ocr.uncertainties.join("\n")}`)}</pre>
      </details>`
      : ""
  }<form id="record-form">
      <div class="row">
      <label>睡眠日（起床した日）<input name="sleep_date" type="date" value="${escapeHTML(record.sleep_date || "")}" required>
      </label>
      <label>種類<select name="sleep_kind" required>
      <option value="">選択してください</option>
      <option value="main" ${record.sleep_kind === "main" ? "selected" : ""}>主睡眠</option>
      <option value="nap" ${record.sleep_kind === "nap" ? "selected" : ""}>昼寝</option>
      </select>
      </label>
      </div>
      <div class="row">
      <label>開始（任意）<input name="started_at" type="datetime-local" value="${escapeHTML(localInput(record.started_at))}">
      </label>
      <label>終了（任意）<input name="ended_at" type="datetime-local" value="${escapeHTML(localInput(record.ended_at))}">
      </label>
      </div>
      <label>睡眠時間（分・任意）<input name="duration_minutes" type="number" min="0" max="1440" value="${record.duration_minutes ?? ""}">
      </label>
      <small>${escapeHTML(appState.settings.timezone)} · 日付に加えて、日時か睡眠時間を入力してください。</small>
      <br>
      <button type="button" id="clear-record" class="quiet">クリア</button>
      <button name="status" value="approved">${ocr ? "承認して保存" : "保存"}</button>${ocr ? '<button name="status" value="rejected" class="quiet">候補を却下</button>' : ""}${isEdit ? '<button name="status" value="deleted" class="quiet">削除</button>' : ""}</form>`;
  select("#editor-content").innerHTML = html;
  select("#editor").showModal();
  const editorToken = newID();
  select("#editor").dataset.token = editorToken;
  select("#clear-record").onclick = () => clearForm(select("#record-form"));
  if (ocr) {
    const form = select("#record-form");
    form.elements.sleep_date.onchange = () => {
      const date = form.elements.sleep_date.value;
      if (!date) return;
      const v = ocr.selected;
      if (v.end_time) form.elements.ended_at.value = date + "T" + v.end_time;
      if (v.start_time) {
        let startDate = date;
        if (v.end_time && v.start_time > v.end_time) {
          const d = new Date(date + "T12:00:00Z");
          d.setUTCDate(d.getUTCDate() - 1);
          startDate = d.toISOString().slice(0, 10);
        }
        form.elements.started_at.value = startDate + "T" + v.start_time;
      }
    };
    request(`/uploads/${ocr.upload_id}/view`)
      .then((result) => {
        if (
          !select("#editor").open ||
          !select("#record-form") ||
          select("#editor").dataset.token !== editorToken
        )
          return;
        const image = document.createElement("img");
        image.className = "photo";
        image.alt = "確認する睡眠記録";
        image.src = result.url;
        select("#editor-content").prepend(image);
      })
      .catch((e) => showNotice(e.message));
  }
  const submission = newID();
  select("#record-form").onsubmit = handleAction(async (e) => {
    e.preventDefault();
    const submitButton = e.submitter;
    if (
      submitButton.value === "deleted" &&
      !confirm("この記録を削除しますか？履歴は残ります。")
    )
      return;
    const data = Object.fromEntries(new FormData(e.target));
    data.duration_minutes =
      data.duration_minutes === "" ? null : Number(data.duration_minutes);
    Object.assign(data, {
      id: record.id || "",
      revision: record.revision || 0,
      submission_id: submission,
      source_job_id: record.source_job_id || "",
      candidate_index: record.candidate_index ?? null,
      status: submitButton.value,
      timezone: appState.settings.timezone,
    });
    submitButton.disabled = true;
    try {
      await request("/records", data);
      select("#editor").close();
      showNotice("記録を保存しました");
      await refreshPage();
    } finally {
      submitButton.disabled = false;
    }
  });
}
