import { appState } from "./state.js";
import { request } from "./api.js";
import { select, escapeHTML, handleAction, showNotice, newID } from "./ui.js";
import {
  durationChart,
  bedtimeChart,
  dayDetail,
  durationLabel,
  clockLabel,
} from "./sleep-charts.js";

function shiftDate(date, days) {
  const d = new Date(date + "T00:00:00Z");
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}
function monday(date) {
  return shiftDate(date, -(new Date(date + "T00:00:00Z").getUTCDay() + 6) % 7);
}
export async function renderAnalyticsPage() {
  const today = new Intl.DateTimeFormat("sv-SE", {
    timeZone: appState.settings.timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
  select("#content").innerHTML = `<h1>睡眠のふりかえり</h1>
  <section class="card">
    <form id="chart-range" class="row"><label>開始日<input type="date" name="from" value="${shiftDate(today, -27)}" required></label><label>終了日<input type="date" name="to" value="${today}" required></label><button>表示</button></form>
    <p class="muted">承認済みの記録 · ${escapeHTML(appState.settings.timezone)} · 未記録は空欄で表示</p>
    <div id="sleep-metrics" class="summary-metrics"></div>
  </section>
  <section class="card"><h2>睡眠時間</h2><p class="chart-legend"><span>● 主睡眠</span><span>● 昼寝</span></p><div id="duration-chart" class="sleep-chart"></div><div class="chart-detail" aria-live="polite"><span class="muted">棒や点を選ぶと、その日の詳細を表示します。</span></div></section>
  <section class="card"><h2>就寝時刻</h2><p class="muted">主睡眠の日付は起床日。複数記録がある日は最初の就寝時刻を表示します。破線は目標時刻です。</p><div id="bedtime-chart" class="sleep-chart"></div><div class="chart-detail" aria-live="polite"><span class="muted">棒や点を選ぶと、その日の詳細を表示します。</span></div><details><summary>日別の数値</summary><div id="sleep-table" class="table-scroll"></div></details></section>
  <section class="card"><h2>週間レポート</h2>
    <form id="report-form"><label>対象週（この日を含む月〜日）<input type="date" name="week_start" value="${monday(today)}" max="${today}" required></label>
    <label class="task-selection"><input type="checkbox" name="deliver" ${appState.settings.webhook_configured ? "" : "disabled"}>Discord にも送る</label>
    <button>レポートを生成</button></form>
    <p class="muted">生成時点の承認済み記録をまとめます。今週は途中経過です。週次の自動送信は設定画面で変更できます。</p>
    <div id="reports-list"></div>
  </section>`;
  let chartVersion = 0;
  async function refreshCharts() {
    const version = ++chartVersion;
    const query = new URLSearchParams(new FormData(select("#chart-range")));
    const data = await request("/analytics?" + query);
    if (version !== chartVersion || appState.page !== "analytics") return;
    select("#sleep-metrics").innerHTML =
      `<div><small>平均主睡眠（${data.duration_days}日分）</small><strong>${durationLabel(data.average_main)}</strong></div><div><small>平均就寝時刻</small><strong>${clockLabel(data.average_bedtime)}</strong></div><div><small>主睡眠の記録日数</small><strong>${data.main_days}日</strong></div><div><small>昼寝</small><strong>${data.nap_count}回</strong></div>`;
    select("#duration-chart").innerHTML = durationChart(data.days);
    select("#bedtime-chart").innerHTML = bedtimeChart(
      data.days,
      appState.settings.target_bedtime,
    );
    selectDay(data.days, null);
    select("#sleep-table").innerHTML =
      `<table><thead><tr><th>日付</th><th>主睡眠</th><th>昼寝</th><th>就寝</th><th>起床</th></tr></thead><tbody>${data.days.map((day) => `<tr><td>${day.date}</td><td>${durationLabel(day.main_minutes)}${day.main_incomplete ? "（一部不明）" : ""}</td><td>${durationLabel(day.nap_minutes)}${day.nap_incomplete ? "（一部不明）" : ""}</td><td>${clockLabel(day.bedtime)}</td><td>${clockLabel(day.wake)}</td></tr>`).join("")}</tbody></table>`;
  }
  // 2 つのグラフで同じ日を選択状態にし、それぞれの下の欄に詳細を出す
  let chartDays = [];
  function selectDay(days, index) {
    chartDays = days;
    for (const chart of document.querySelectorAll(".sleep-chart")) {
      for (const shape of chart.querySelectorAll("[data-day]"))
        shape.classList.toggle("selected", Number(shape.dataset.day) === index);
      const panel = chart.nextElementSibling;
      if (panel)
        panel.innerHTML =
          index == null
            ? '<span class="muted">棒や点を選ぶと、その日の詳細を表示します。</span>'
            : dayDetail(days[index]);
    }
  }
  for (const id of ["#duration-chart", "#bedtime-chart"]) {
    const chart = select(id);
    const choose = (e) => {
      const shape = e.target.closest("[data-day]");
      if (shape) selectDay(chartDays, Number(shape.dataset.day));
    };
    chart.addEventListener("click", choose);
    chart.addEventListener("keydown", (e) => {
      if (e.key !== "Enter" && e.key !== " ") return;
      e.preventDefault();
      choose(e);
    });
  }
  select("#chart-range").onsubmit = handleAction(async (e) => {
    e.preventDefault();
    await refreshCharts();
  });
  let lastReports = "";
  async function refreshReports() {
    const reports = await request("/reports");
    const container = select("#reports-list");
    if (!container || appState.page !== "analytics") return;
    const serialized = JSON.stringify(reports);
    if (serialized === lastReports) return;
    lastReports = serialized;
    const statuses = {
      pending: "生成待ち",
      running: "生成中",
      succeeded: "生成済み",
      failed: "失敗",
      cancelled: "取り消し",
    };
    const delivery = {
      pending: "送信待ち",
      sending: "送信中",
      sent: "送信済み",
      failed: "送信失敗",
      cancelled: "送信取り消し",
    };
    container.innerHTML = reports.length
      ? reports
          .map(
            (r, i) =>
              `<details ${i === 0 ? "open" : ""} data-report-id="${r.id}"><summary>${escapeHTML(r.report.start)}〜${escapeHTML(r.report.end)} · ${statuses[r.status] || escapeHTML(r.status)}${r.delivery_status ? " · " + (delivery[r.delivery_status] || escapeHTML(r.delivery_status)) : ""}</summary>${r.invalidated_at ? "<p>生成後に記録が変更されています。最新の内容は再生成してください。</p>" : ""}<p class="report-body">${escapeHTML(r.output?.body || r.output?.reason || r.error || "生成しています…")}</p></details>`,
          )
          .join("")
      : '<p class="muted">レポートはまだありません。</p>';
  }
  select("#report-form").onsubmit = handleAction(async (e) => {
    e.preventDefault();
    const button = e.target.querySelector("button");
    button.disabled = true;
    try {
      const form = new FormData(e.target);
      await request("/reports", {
        week_start: form.get("week_start"),
        deliver: form.has("deliver"),
        submission_id: newID(),
      });
      showNotice("週間レポートの生成を開始しました");
      await refreshReports();
    } finally {
      button.disabled = false;
    }
  });
  await Promise.all([refreshCharts(), refreshReports()]);
  if (appState.page === "analytics")
    appState.jobTimer = setInterval(
      () => refreshReports().catch((e) => showNotice(e.message)),
      3000,
    );
}
