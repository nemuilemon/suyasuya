"use strict";
const $ = (s) => document.querySelector(s),
  esc = (s) =>
    String(s ?? "").replace(
      /[&<>"']/g,
      (c) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
          "'": "&#39;",
        })[c],
    );
let user,
  settings,
  page = location.hash.slice(2) || "records",
  jobTimer,
  pvt = null,
  pendingPVT = null;
const uid = () => crypto.randomUUID(),
  today = () => localDate(new Date()),
  localDate = (d) =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
function notice(s = "") {
  $("#notice").textContent = s;
}
async function api(path, body, method) {
  const r = await fetch("/api" + path, {
    method: method || (body ? "POST" : "GET"),
    headers: { "Content-Type": "application/json", "X-Suyasuya": "1" },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!r.ok) {
    const message = await r.text();
    if (r.status === 401 && path != "/login") {
      showLogin();
    }
    throw Error(message.trim());
  }
  return r.json();
}
function action(fn) {
  return async (e) => {
    try {
      await fn(e);
    } catch (err) {
      notice(err.message);
    }
  };
}
function showLogin() {
  clearInterval(jobTimer);
  $("#app").hidden = true;
  $("#login").hidden = false;
  $("#logout").hidden = true;
}
async function boot() {
  const me = await api("/me");
  user = me.user;
  settings = await api("/settings");
  $("#login").hidden = true;
  $("#app").hidden = false;
  $("#logout").hidden = false;
  await render();
}
$("#login-form").onsubmit = action(async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  await api("/login", Object.fromEntries(f));
  e.target.reset();
  notice();
  await boot();
});
$("#logout").onclick = action(async () => {
  if (pvt) return;
  await api("/logout", {});
  showLogin();
});
$("#close-editor").onclick = $("#back-editor").onclick = () =>
  $("#editor").close();
function navigate(next) {
  if (pvt) {
    notice("測定を終了してから移動してください");
    return;
  }
  location.hash = "/" + next;
}
document
  .querySelectorAll("[data-page]")
  .forEach((b) => (b.onclick = () => navigate(b.dataset.page)));
window.addEventListener(
  "hashchange",
  action(async () => {
    const next = location.hash.slice(2) || "records";
    if (pvt && next !== page) {
      history.replaceState(null, "", "#/" + page);
      notice("測定を終了してから移動してください");
      return;
    }
    page = next;
    notice();
    if (user) await render();
  }),
);

async function render() {
  clearInterval(jobTimer);
  document
    .querySelectorAll("[data-page]")
    .forEach((b) =>
      b.setAttribute("aria-current", String(b.dataset.page === page)),
    );
  if (page === "records") await recordsPage();
  else if (page === "record") await recordPage();
  else if (page === "pvt") await pvtPage();
  else await settingsPage();
}
function fmtDate(s) {
  if (!s) return "—";
  return new Date(s).toLocaleString("ja-JP", {
    timeZone: settings.timezone,
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}
function localInput(s) {
  if (!s) return "";
  const parts = new Intl.DateTimeFormat("sv-SE", {
    timeZone: settings.timezone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  }).format(new Date(s));
  return parts.replace(" ", "T");
}
async function recordPage() {
  clearInterval(jobTimer);
  $("#content").innerHTML =
    `<h1>記録する</h1><p>画像の読み取り結果を確かめて保存。主睡眠も昼寝も、数値だけで記録できます。</p><div class="grid"><section class="card"><h2>スクリーンショットを読み取る</h2><form id="upload-form"><label>画像<input type="file" name="image" accept="image/png,image/jpeg,image/webp" required></label><label>撮影日<input name="date" type="date" value="${today()}" required></label><small>撮影日と睡眠の日付は別です。読み取り後に確認してください。</small><br><button>アップロードして読み取る</button></form></section><section class="card"><h2>数値から記録する</h2><p>画像がない日や、検知されなかった昼寝にも。</p><button id="manual">記録を追加</button><p class="muted">時刻は ${esc(settings.timezone)} で入力します。</p></section></div><section class="card"><h2>読み取り・確認待ち</h2><div id="jobs">読み込み中…</div></section>`;
  $("#manual").onclick = () => editRecord({});
  $("#upload-form").onsubmit = action(async (e) => {
    e.preventDefault();
    const button = e.target.querySelector("button");
    button.disabled = true;
    try {
      const f = new FormData(e.target),
        image = f.get("image");
      const upload = await api("/uploads", {
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
      await api(`/uploads/${upload.id}/complete`, {});
      sessionStorage.removeItem("pending-upload");
      e.target.reset();
      e.target.elements.date.value = today();
      notice("読み取りを開始しました。結果を確認して承認してください。");
      await loadJobs();
    } finally {
      button.disabled = false;
    }
  });

  const pending = sessionStorage.getItem("pending-upload");
  if (pending) {
    try {
      await api(`/uploads/${pending}/complete`, {});
      sessionStorage.removeItem("pending-upload");
    } catch (err) {
      notice("画像の完了確認が保留中です: " + err.message);
    }
  }
  await loadJobs();
  jobTimer = setInterval(
    () => loadJobs().catch((e) => notice(e.message)),
    5000,
  );
}

async function recordsPage() {
  clearInterval(jobTimer);
  const records = await api("/records");
  $("#content").innerHTML =
    `<h1>記録一覧</h1><section class="card"><div id="records-list">${records.length ? "" : "まだ記録がありません。"}</div></section>`;
  for (const r of records) {
    const el = document.createElement("div");
    el.className = "record";
    el.innerHTML = `<div><span class="tag">${r.sleep_kind === "nap" ? "昼寝" : "主睡眠"}</span> <strong>${esc(r.sleep_date)}</strong><br>${r.duration_minutes ?? "—"} 分 <small>${esc(fmtDate(r.started_at))} → ${esc(fmtDate(r.ended_at))}</small></div><div class="actions"><button class="quiet">編集</button><button class="quiet">履歴</button></div>`;
    const buttons = el.querySelectorAll("button");
    buttons[0].onclick = () => editRecord(r);
    buttons[1].onclick = action(async () => {
      const data = await api(`/records/${r.id}/history`);
      $("#editor-content").innerHTML =
        "<h2>変更履歴</h2><pre>" +
        esc(JSON.stringify(data, null, 2)) +
        "</pre>";
      $("#editor").showModal();
    });
    $("#records-list").append(el);
  }
}
async function loadJobs() {
  const jobs = await api("/jobs");
  if (page !== "record") return;
  const host = $("#jobs");
  host.replaceChildren();
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
    card.innerHTML = `<div><span class="tag">${status[job.status]}</span> <small>${esc(fmtDate(job.created_at))} · prompt v${job.prompt_version}</small><div class="candidates"></div></div><div class="actions"></div>`;
    const actions = card.querySelector(".actions");
    function btn(label, fn) {
      const b = document.createElement("button");
      b.className = "quiet";
      b.textContent = label;
      b.onclick = action(fn);
      actions.append(b);
    }
    btn("画像", async () => {
      const result = await api(`/uploads/${job.upload_id}/view`);
      $("#editor-content").innerHTML =
        `<img class="photo" alt="アップロードした睡眠記録" src="${esc(result.url)}">`;
      $("#editor").showModal();
    });
    if (["failed", "cancelled"].includes(job.status))
      btn("再試行", async () => {
        await api(`/jobs/${job.id}/retry`, {});
        await loadJobs();
      });
    if (["pending", "running"].includes(job.status))
      btn("中止", async () => {
        await api(`/jobs/${job.id}/cancel`, {});
        await loadJobs();
      });
    if (job.error) {
      const p = document.createElement("p");
      p.textContent = job.error;
      card.querySelector(".candidates").append(p);
    }
    if (job.output) {
      job.output.records.forEach((r, i) => {
        const b = document.createElement("button");
        b.className = "quiet";
        b.textContent = job.reviewed.includes(i)
          ? `候補 ${i + 1} 確認済み`
          : `候補 ${i + 1} を確認 (${r.duration_minutes ?? "不明"} 分)`;
        b.disabled = job.reviewed.includes(i);
        b.onclick = () => editCandidate(job, r, i);
        card.querySelector(".candidates").append(b);
      });
    }
    host.append(card);
  }
}
function editCandidate(job, r, i) {
  editRecord(
    {
      source_job_id: job.id,
      candidate_index: i,
      sleep_date: "",
      sleep_kind: r.sleep_kind || "",
      duration_minutes: r.duration_minutes,
    },
    { ...job.output, selected: r, upload_id: job.upload_id },
  );
}
function editRecord(r, ocr) {
  const isEdit = !!r.id;
  const html = `<h2>${isEdit ? "記録を編集" : ocr ? "読み取り結果を確認" : "睡眠を記録"}</h2>${ocr ? `<p>画像を見ながら、日付・主睡眠／昼寝・日時を確認してください。表示された時刻は下の抽出結果にあります。</p><details open><summary>抽出結果</summary><pre>${esc(`表示日付: ${ocr.displayed_date || "不明"}\n開始: ${ocr.selected.start_time || "不明"} / 終了: ${ocr.selected.end_time || "不明"}\n睡眠時間: ${ocr.selected.duration_minutes ?? "不明"} 分\n${ocr.uncertainties.join("\n")}`)}</pre></details>` : ""}<form id="record-form"><div class="row"><label>睡眠日（起床した日）<input name="sleep_date" type="date" value="${esc(r.sleep_date || "")}" required></label><label>種類<select name="sleep_kind" required><option value="">選択してください</option><option value="main" ${r.sleep_kind === "main" ? "selected" : ""}>主睡眠</option><option value="nap" ${r.sleep_kind === "nap" ? "selected" : ""}>昼寝</option></select></label></div><div class="row"><label>開始（任意）<input name="started_at" type="datetime-local" value="${esc(localInput(r.started_at))}"></label><label>終了（任意）<input name="ended_at" type="datetime-local" value="${esc(localInput(r.ended_at))}"></label></div><label>睡眠時間（分・任意）<input name="duration_minutes" type="number" min="0" max="1440" value="${r.duration_minutes ?? ""}"></label><small>${esc(settings.timezone)} · 日付に加えて、日時か睡眠時間を入力してください。</small><br><button name="status" value="approved">${ocr ? "承認して保存" : "保存"}</button>${ocr ? '<button name="status" value="rejected" class="quiet">候補を却下</button>' : ""}${isEdit ? '<button name="status" value="deleted" class="quiet">削除</button>' : ""}</form>`;
  $("#editor-content").innerHTML = html;
  $("#editor").showModal();
  if (ocr) {
    const form = $("#record-form");
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
    api(`/uploads/${ocr.upload_id}/view`)
      .then((result) => {
        if (!$("#editor").open || !$("#record-form")) return;
        const image = document.createElement("img");
        image.className = "photo";
        image.alt = "確認する睡眠記録";
        image.src = result.url;
        $("#editor-content").prepend(image);
      })
      .catch((e) => notice(e.message));
  }
  const submission = uid();
  $("#record-form").onsubmit = action(async (e) => {
    e.preventDefault();
    const b = e.submitter;
    if (
      b.value === "deleted" &&
      !confirm("この記録を削除しますか？履歴は残ります。")
    )
      return;
    const data = Object.fromEntries(new FormData(e.target));
    data.duration_minutes =
      data.duration_minutes === "" ? null : Number(data.duration_minutes);
    Object.assign(data, {
      id: r.id || "",
      revision: r.revision || 0,
      submission_id: submission,
      source_job_id: r.source_job_id || "",
      candidate_index: r.candidate_index ?? null,
      status: b.value,
      timezone: settings.timezone,
    });
    b.disabled = true;
    try {
      await api("/records", data);
      $("#editor").close();
      notice("記録を保存しました");
      await render();
    } finally {
      b.disabled = false;
    }
  });
}
async function settingsPage() {
  settings = await api("/settings");
  $("#content").innerHTML =
    `<h1>設定</h1><section class="card"><h2>生活の時刻</h2><form id="settings-form"><label>タイムゾーン<input name="timezone" value="${esc(settings.timezone)}" required></label><div class="row"><label>目標の就寝時刻<input type="time" name="target_bedtime" value="${esc((settings.target_bedtime || "").slice(0, 5))}"></label><label>PVT の測定時刻<input type="time" name="pvt_reminder_time" value="${esc((settings.pvt_reminder_time || "").slice(0, 5))}"></label></div><p class="muted">MVP では設定を保存し、PVT 画面に表示します。Discord の催促はまだ行いません。</p><button>保存</button></form></section><div id="prompts"></div>`;
  $("#settings-form").onsubmit = action(async (e) => {
    e.preventDefault();
    settings = await api(
      "/settings",
      Object.fromEntries(new FormData(e.target)),
      "PUT",
    );
    notice("設定を保存しました");
  });
  if (user.is_admin) {
    const prompts = await api("/prompts");
    for (const key of ["job.extract_sleep", "agent.loop"]) {
      const versions = prompts.filter((p) => p.config_key === key),
        active = versions.find((p) => p.is_active);
      const section = document.createElement("section");
      section.className = "card";
      section.innerHTML = `<h2>${key === "agent.loop" ? "Agent loop の指示" : "OCR の指示"}</h2><p>${key === "agent.loop" ? "保存・微調整できます。複雑な agent 機能を実装するまで実行には使いません。" : "新しい OCR job から適用されます。実行済み・再試行の job は元の版を使います。"}</p><form><label>版<select>${versions.map((v) => `<option value="${v.id}" ${v.is_active ? "selected" : ""}>v${v.version}${v.is_active ? "（使用中）" : ""}</option>`).join("")}</select></label><label>指示<textarea required>${esc(active?.body || "")}</textarea></label><button>新しい版として保存</button></form>`;
      section.querySelector("select").onchange = (e) =>
        (section.querySelector("textarea").value = versions.find(
          (v) => v.id === e.target.value,
        ).body);
      section.querySelector("form").onsubmit = action(async (e) => {
        e.preventDefault();
        await api("/prompts", {
          config_key: key,
          body: section.querySelector("textarea").value,
        });
        notice("新しい版を保存しました");
        await settingsPage();
      });
      $("#prompts").append(section);
    }
  }
}
async function pvtPage() {
  const list = await api("/pvt");
  $("#content").innerHTML =
    `<h1>1 分の反応時間テスト</h1><p>表示が「押して」に変わったら、スペースキーか画面のボタンを押してください。</p><section class="card"><p>測定時刻：${esc(settings.pvt_reminder_time || "未設定")} (${esc(settings.timezone)})</p><p class="muted">静かな場所で、できるだけ同じ端末を使ってください。別タブへ移動すると測定を中断します。</p><div class="row"><div id="pvt-clock">1:00</div><button id="pvt-start">測定を始める</button><button id="pvt-stop" class="quiet" hidden>中断する</button></div><p id="pvt-feedback" role="status"></p><button id="pvt-target" disabled>準備ができたら開始</button><button id="pvt-save" hidden>保存を再試行</button></section><section class="card"><h2>測定履歴</h2>${list.map((s) => `<div class="record"><div>${esc(fmtDate(s.started_at))} <span class="tag">${s.completed ? "完了" : "中断"}</span><br>中央値 <strong>${s.median_rt_ms == null ? "—" : Math.round(s.median_rt_ms)} ms</strong> <small>遅延 ${s.lapses ?? 0} 回 · フライング ${s.false_starts ?? 0} 回</small></div></div>`).join("") || "まだ測定がありません。"}</section>`;
  $("#pvt-start").onclick = startPVT;
  $("#pvt-stop").onclick = () => finishPVT(false);
  $("#pvt-target").onpointerdown = (e) => {
    e.preventDefault();
    respond("pointer");
  };
  $("#pvt-save").hidden = !pendingPVT;
  $("#pvt-save").onclick = action(async () => {
    await savePVT();
    await pvtPage();
  });
}
function startPVT() {
  if (pvt) return;
  notice();
  pvt = {
    id: uid(),
    wall: new Date(),
    start: performance.now(),
    stimulus: null,
    trials: [],
    methods: new Set(),
    timer: null,
    end: null,
    tick: null,
  };
  $("#pvt-start").disabled = true;
  $("#pvt-stop").hidden = false;
  $("#pvt-target").disabled = false;
  scheduleStimulus();
  pvt.end = setTimeout(() => finishPVT(true), 60000);
  pvt.tick = setInterval(() => {
    if (!pvt) return;
    const left = Math.max(
      0,
      Math.ceil((60000 - (performance.now() - pvt.start)) / 1000),
    );
    $("#pvt-clock").textContent =
      `${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
  }, 200);
}
function scheduleStimulus() {
  if (!pvt) return;
  pvt.stimulus = null;
  $("#pvt-target").textContent = "待って…";
  $("#pvt-target").classList.remove("ready");
  pvt.timer = setTimeout(
    () => {
      if (!pvt) return;
      requestAnimationFrame(() => {
        if (!pvt) return;
        pvt.stimulus = performance.now() - pvt.start;
        $("#pvt-target").textContent = "押して";
        $("#pvt-target").classList.add("ready");
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
  $("#pvt-feedback").textContent =
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
    timezone: settings.timezone,
    device: navigator.userAgent.slice(0, 256),
    input_method:
      state.methods.size > 1 ? "mixed" : [...state.methods][0] || "pointer",
    completed,
    trials: state.trials,
  };
  $("#pvt-target").disabled = true;
  $("#pvt-stop").hidden = true;
  $("#pvt-target").textContent = completed
    ? "測定が終わりました"
    : "測定を中断しました";
  try {
    await savePVT();
    await pvtPage();
  } catch (e) {
    notice(e.message);
    $("#pvt-save").hidden = false;
  }
}
async function savePVT() {
  if (!pendingPVT) return;
  await api("/pvt", pendingPVT);
  notice(
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
boot().catch((e) => {
  showLogin();
  if (!e.message.includes("ログインしてください")) notice(e.message);
});
