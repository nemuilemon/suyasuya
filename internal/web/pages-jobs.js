import { appState } from "./state.js";
import { request } from "./api.js";
import {
  select,
  escapeHTML,
  showNotice,
  handleAction,
  formatDate,
} from "./ui.js";
import { openCandidateEditor } from "./record-editor.js";
export async function refreshJobs() {
  const jobs = await request("/jobs");
  if (appState.page !== "record") return;
  const host = select("#jobs");
  const visibleIDs = new Set(jobs.map((job) => job.id));
  for (const id of selectedJobs)
    if (!visibleIDs.has(id)) selectedJobs.delete(id);
  host.replaceChildren();
  updateSelection(jobs);
  if (!jobs.length) {
    host.textContent = "画像を追加すると、ここに読み取り結果が届きます。";
    return;
  }
  for (const job of jobs) {
    const card = document.createElement("div");
    card.className = "record";
    const status = {
      pending: "待機中",
      running: "読み取り中",
      succeeded: "読み取り完了",
      failed: "失敗",
      cancelled: "中止",
    };
    card.innerHTML = `<div>
      <span class="tag">${status[job.status]}</span> <small>${escapeHTML(formatDate(job.created_at))} · prompt v${job.prompt_version}</small>
      <div class="candidates">
      </div>
      </div>
      <div class="actions">
      </div>`;
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = selectedJobs.has(job.id);
    checkbox.setAttribute("aria-label", "タスクを選択");
    checkbox.onchange = () => {
      if (checkbox.checked) selectedJobs.add(job.id);
      else selectedJobs.delete(job.id);
      updateSelection(jobs);
    };
    card.prepend(checkbox);
    card.dataset.jobId = job.id;
    const actions = card.querySelector(".actions");
    function appendAction(label, fn) {
      const button = document.createElement("button");
      button.className = "quiet";
      button.textContent = label;
      button.onclick = handleAction(fn);
      actions.append(button);
    }
    appendAction("削除", () =>
      removeJobs(
        { ids: [job.id] },
        "このタスクを削除しますか？ 保存した睡眠記録は残ります。",
      ),
    );
    appendAction("画像", async () => {
      const result = await request(`/uploads/${job.upload_id}/view`);
      select("#editor-content").innerHTML =
        `<img class="photo" alt="アップロードした睡眠記録" src="${escapeHTML(result.url)}">`;
      select("#editor").showModal();
    });
    if (["failed", "cancelled"].includes(job.status))
      appendAction("再試行", async () => {
        await request(`/jobs/${job.id}/retry`, {});
        await refreshJobs();
      });
    if (["pending", "running"].includes(job.status))
      appendAction("中止", async () => {
        await request(`/jobs/${job.id}/cancel`, {});
        await refreshJobs();
      });
    if (job.error) {
      const p = document.createElement("p");
      p.textContent = job.error;
      card.querySelector(".candidates").append(p);
    }
    if (job.output) {
      job.output.records.forEach((candidate, candidateIndex) => {
        const button = document.createElement("button");
        button.className = "quiet";
        button.textContent = job.reviewed.includes(candidateIndex)
          ? `候補 ${candidateIndex + 1} 確認済み`
          : `候補 ${candidateIndex + 1} を確認 (${candidate.duration_minutes ?? "不明"} 分)`;
        button.disabled = job.reviewed.includes(candidateIndex);
        button.onclick = () =>
          openCandidateEditor(job, candidate, candidateIndex);
        card.querySelector(".candidates").append(button);
      });
    }
    host.append(card);
  }
}
// Selection survives polling, but not leaving/re-entering the input page.
const selectedJobs = new Set();
function updateSelection(jobs) {
  select("#delete-selected-jobs").disabled = selectedJobs.size === 0;
  const all = select("#select-all-jobs");
  all.checked =
    jobs.length > 0 && jobs.every((job) => selectedJobs.has(job.id));
  all.indeterminate = selectedJobs.size > 0 && !all.checked;
}
async function removeJobs(input, message) {
  if (!confirm(message)) return;
  const result = await request("/jobs", input, "DELETE");
  selectedJobs.clear();
  showNotice(`${result.deleted} 件のタスクを削除しました`);
  await refreshJobs();
}
export function setupJobActions() {
  selectedJobs.clear();
  select("#select-all-jobs").onchange = (event) => {
    for (const card of select("#jobs").querySelectorAll("[data-job-id]")) {
      const checkbox = card.querySelector('input[type="checkbox"]');
      checkbox.checked = event.target.checked;
      if (checkbox.checked) selectedJobs.add(card.dataset.jobId);
      else selectedJobs.delete(card.dataset.jobId);
    }
    select("#delete-selected-jobs").disabled = selectedJobs.size === 0;
    select("#select-all-jobs").indeterminate = false;
  };
  select("#delete-selected-jobs").onclick = handleAction(() =>
    removeJobs(
      { ids: [...selectedJobs] },
      `${selectedJobs.size} 件のタスクを削除しますか？ 未承認の候補も削除されます。保存した睡眠記録は残ります。`,
    ),
  );
  select("#delete-completed-jobs").onclick = handleAction(() =>
    removeJobs(
      { completed: true },
      "候補をすべて確認済みのタスクを削除しますか？ 保存した睡眠記録は残ります。",
    ),
  );
}
