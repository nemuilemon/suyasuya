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
let pvt = null,
  pendingPVT = null;
export function isMeasuring() {
  return pvt !== null;
}
export async function renderPVTPage() {
  const list = await request("/pvt");
  select("#content").innerHTML = `<h1>1 分の反応時間テスト</h1>
      <p>表示が「押して」に変わったら、スペースキーか画面のボタンを押してください。</p>
      <section class="card">
      <p>測定時刻：${escapeHTML(appState.settings.pvt_reminder_time || "未設定")} (${escapeHTML(appState.settings.timezone)})</p>
      <p class="muted">静かな場所で、できるだけ同じ端末を使ってください。別タブへ移動すると測定を中断します。</p>
      <div class="row">
      <div id="pvt-clock">1:00</div>
      <button id="pvt-start">測定を始める</button>
      <button id="pvt-stop" class="quiet" hidden>中断する</button>
      </div>
      <p id="pvt-feedback" role="status">
      </p>
      <button id="pvt-target" disabled>準備ができたら開始</button>
      <button id="pvt-save" hidden>保存を再試行</button>
      </section>
      <section class="card">
      <h2>測定履歴</h2>${
        list
          .map(
            (s) => `<div class="record">
      <div>${escapeHTML(formatDate(s.started_at))} <span class="tag">${s.completed ? "完了" : "中断"}</span>
      <br>中央値 <strong>${s.median_rt_ms == null ? "—" : Math.round(s.median_rt_ms)} ms</strong> <small>遅延 ${s.lapses ?? 0} 回 · フライング ${s.false_starts ?? 0} 回</small>
      </div>
      </div>`,
          )
          .join("") || "まだ測定がありません。"
      }</section>`;
  select("#pvt-start").onclick = startPVT;
  select("#pvt-stop").onclick = () => finishPVT(false);
  select("#pvt-target").onpointerdown = (e) => {
    e.preventDefault();
    respond("pointer");
  };
  select("#pvt-save").hidden = !pendingPVT;
  select("#pvt-save").onclick = handleAction(async () => {
    await savePVT();
    await renderPVTPage();
  });
}
function startPVT() {
  if (pvt) return;
  showNotice();
  pvt = {
    id: newID(),
    wall: new Date(),
    start: performance.now(),
    stimulus: null,
    trials: [],
    methods: new Set(),
    timer: null,
    end: null,
    tick: null,
  };
  select("#pvt-start").disabled = true;
  select("#pvt-stop").hidden = false;
  select("#pvt-target").disabled = false;
  scheduleStimulus();
  pvt.end = setTimeout(() => finishPVT(true), 60000);
  pvt.tick = setInterval(() => {
    if (!pvt) return;
    const left = Math.max(
      0,
      Math.ceil((60000 - (performance.now() - pvt.start)) / 1000),
    );
    select("#pvt-clock").textContent =
      `${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
  }, 200);
}
function scheduleStimulus() {
  if (!pvt) return;
  pvt.stimulus = null;
  select("#pvt-target").textContent = "待って…";
  select("#pvt-target").classList.remove("ready");
  pvt.timer = setTimeout(
    () => {
      if (!pvt) return;
      requestAnimationFrame(() => {
        if (!pvt) return;
        pvt.stimulus = performance.now() - pvt.start;
        select("#pvt-target").textContent = "押して";
        select("#pvt-target").classList.add("ready");
      });
    },
    1000 + Math.random() * 3000,
  );
}
function respond(method) {
  if (!pvt) return;
  const response = performance.now() - pvt.start;
  if (response >= 60000) {
    finishPVT(true);
    return;
  }
  pvt.methods.add(method);
  const rt = pvt.stimulus == null ? null : response - pvt.stimulus;
  pvt.trials.push({
    stimulus_at_ms: pvt.stimulus,
    response_at_ms: response,
    rt_ms: rt,
    false_start: rt == null || rt < 100,
  });
  select("#pvt-feedback").textContent =
    rt == null || rt < 100 ? "少し早かったです" : `${Math.round(rt)} ms`;
  clearTimeout(pvt.timer);
  scheduleStimulus();
}
async function finishPVT(completed) {
  if (!pvt) return;
  const state = pvt;
  pvt = null;
  clearTimeout(state.timer);
  clearTimeout(state.end);
  clearInterval(state.tick);
  pendingPVT = {
    id: state.id,
    started_at: state.wall.toISOString(),
    finished_at: new Date(
      state.wall.getTime() + Math.min(performance.now() - state.start, 60000),
    ).toISOString(),
    timezone: appState.settings.timezone,
    device: navigator.userAgent.slice(0, 256),
    input_method:
      state.methods.size > 1 ? "mixed" : [...state.methods][0] || "pointer",
    completed,
    trials: state.trials,
  };
  select("#pvt-target").disabled = true;
  select("#pvt-stop").hidden = true;
  select("#pvt-target").textContent = completed
    ? "測定が終わりました"
    : "測定を中断しました";
  try {
    await savePVT();
    await renderPVTPage();
  } catch (e) {
    showNotice(e.message);
    select("#pvt-save").hidden = false;
  }
}
async function savePVT() {
  if (!pendingPVT) return;
  await request("/pvt", pendingPVT);
  showNotice(
    pendingPVT.completed
      ? "測定結果を保存しました"
      : "中断した測定を保存しました",
  );
  pendingPVT = null;
}
document.addEventListener("keydown", (e) => {
  if (pvt && e.code === "Space") {
    e.preventDefault();
    if (!e.repeat) respond("keyboard");
  }
});
document.addEventListener("visibilitychange", () => {
  if (document.hidden && pvt) finishPVT(false);
});
window.addEventListener("blur", () => {
  if (pvt) finishPVT(false);
});
window.addEventListener("beforeunload", (e) => {
  if (pvt || pendingPVT) {
    e.preventDefault();
    e.returnValue = "";
  }
});
