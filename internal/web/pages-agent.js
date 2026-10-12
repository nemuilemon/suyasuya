import { appState } from "./state.js";
import { request } from "./api.js";
import { select, escapeHTML, handleAction, formatDate } from "./ui.js";

const kindLabel = {
  extract_sleep: "画像の読み取り",
  plan_reminder: "催促の判断",
  weekly_report: "週間レポート",
};
const statusLabel = {
  pending: "待機中",
  running: "実行中",
  succeeded: "完了",
  failed: "失敗",
  cancelled: "中止",
};
const stepLabel = {
  start: "開始",
  model: "モデル",
  tool: "tool",
  final: "結果",
  error: "エラー",
};

const json = (v) => escapeHTML(JSON.stringify(v, null, 2));
const ms = (n) =>
  n == null ? "" : n < 1000 ? `${n}ms` : `${(n / 1000).toFixed(1)}s`;

export async function renderAgentPage() {
  select("#content").innerHTML = `<h1>agent ログ</h1>
  <section class="card">
    <p class="muted">agent が task ごとに使った tool・引数・結果と、モデルの各ラウンドを記録しています。直近50件。開くと詳細を読み込みます。</p>
    <div id="agent-runs"></div>
  </section>`;
  const runs = await request("/agent-runs");
  if (appState.page !== "agent") return;
  const host = select("#agent-runs");
  if (!runs.length) {
    host.textContent = "まだ task がありません。";
    return;
  }
  for (const run of runs) {
    const item = document.createElement("details");
    item.className = "agent-run";
    item.innerHTML = `<summary>
      <span class="tag">${escapeHTML(statusLabel[run.status] || run.status)}</span>
      <strong>${escapeHTML(kindLabel[run.kind] || run.kind)}</strong>
      <small>${escapeHTML(formatDate(run.created_at))} · ${escapeHTML(run.model)} · tool ${run.tool_calls}回 · ${run.steps} steps${run.attempts > 1 ? ` · 試行 ${run.attempts}` : ""}</small>
      ${run.error ? `<small class="agent-error">${escapeHTML(run.error)}</small>` : ""}
    </summary>
    <div class="agent-steps muted">読み込み中…</div>
    <footer class="agent-run-end"><button type="button" class="quiet">閉じる</button></footer>`;
    item.querySelector(".agent-run-end button").addEventListener("click", () => {
      item.open = false;
      if (item.getBoundingClientRect().top < 0) item.scrollIntoView({ block: "start" });
    });
    let loaded = false;
    item.addEventListener(
      "toggle",
      handleAction(async () => {
        if (!item.open || loaded) return;
        loaded = true;
        const steps = await request(`/jobs/${run.id}/steps`);
        item.querySelector(".agent-steps").outerHTML = renderSteps(steps);
      }),
    );
    host.append(item);
  }
}

function renderSteps(steps) {
  if (!steps.length)
    return `<div class="agent-steps"><p class="muted">記録がありません（記録機能の追加前の task か、まだ実行されていません）。</p></div>`;
  let html = "";
  let run = null;
  let runNo = 0;
  for (const step of steps) {
    if (step.run_id !== run) {
      if (run) html += "</ol>";
      run = step.run_id;
      runNo++;
      html += `<h3 class="agent-run-head">実行 ${runNo} <small>${escapeHTML(formatDate(step.created_at))}</small></h3><ol class="agent-step-list">`;
    }
    html += renderStep(step);
  }
  return `<div class="agent-steps">${html}</ol></div>`;
}

function renderStep(step) {
  const out = step.output || {};
  let body = "";
  if (step.kind === "tool") {
    body = `<details><summary>引数</summary><pre>${json(step.input || {})}</pre></details>
      <details${step.ok ? "" : " open"}><summary>結果</summary><pre>${json(out)}</pre></details>`;
  } else if (step.kind === "model") {
    const parts = [];
    if (out.tool_calls) parts.push(`tool を要求: ${out.tool_calls.map(escapeHTML).join(", ")}`);
    if (out.tokens) parts.push(`tokens ${out.tokens.total}（入力 ${out.tokens.prompt} / 出力 ${out.tokens.output} / 思考 ${out.tokens.thoughts}）`);
    body = `<small>${parts.join(" · ")}</small>`;
    if (out.text) body += `<pre>${escapeHTML(out.text)}</pre>`;
    if (out.error) body += `<pre class="agent-error">${escapeHTML(out.error)}</pre>`;
  } else if (step.kind === "start") {
    body = `<details><summary>入力</summary><pre>${json(step.input || {})}</pre></details>`;
  } else if (step.kind === "final") {
    body = `<pre>${json(out)}</pre>`;
  } else if (step.kind === "error") {
    body = `<pre class="agent-error">${escapeHTML(out.error || "")}</pre>`;
  }
  const label =
    step.kind === "model"
      ? out.final
        ? "最終回答"
        : `モデル ${out.round || ""}`
      : stepLabel[step.kind] || step.kind;
  return `<li class="agent-step agent-step-${escapeHTML(step.kind)}${step.ok ? "" : " failed"}">
    <div><span class="tag">${escapeHTML(label)}</span> <strong>${escapeHTML(step.name || "")}</strong> <small>${ms(step.duration_ms)}${step.ok ? "" : " · 失敗"}</small></div>
    ${body}
  </li>`;
}
