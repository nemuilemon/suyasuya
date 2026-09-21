import { escapeHTML } from "./ui.js";

export const durationLabel = (minutes) =>
  minutes == null ? "—" : `${Math.floor(minutes / 60)}時間${minutes % 60}分`;
export function clockLabel(minutes) {
  if (minutes == null) return "—";
  minutes = ((minutes % 1440) + 1440) % 1440;
  return `${String(Math.floor(minutes / 60)).padStart(2, "0")}:${String(minutes % 60).padStart(2, "0")}`;
}
const W = 860,
  H = 290,
  L = 64,
  R = 20,
  T = 22,
  B = 40;
function frame(title, content) {
  return `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="${escapeHTML(title)}"><title>${escapeHTML(title)}</title>${content}</svg>`;
}
function dateLabels(days, x) {
  const step = Math.max(1, Math.ceil(days.length / 9));
  return days
    .map((day, i) =>
      i % step === 0 || i === days.length - 1
        ? `<text x="${x(i)}" y="${H - 12}" text-anchor="middle">${day.date.slice(5).replace("-", "/")}</text>`
        : "",
    )
    .join("");
}
export function durationChart(days) {
  const width = (W - L - R) / days.length;
  const x = (i) => L + width * (i + 0.5);
  const max = Math.max(
    600,
    ...days.map((d) => (d.main_minutes || 0) + (d.nap_minutes || 0)),
  );
  const ceiling = Math.ceil(max / 120) * 120;
  const y = (minutes) => H - B - (minutes / ceiling) * (H - T - B);
  let drawing = "";
  for (let n = 0; n <= ceiling; n += 120) {
    drawing += `<line class="chart-grid" x1="${L}" x2="${W - R}" y1="${y(n)}" y2="${y(n)}"/><text x="${L - 10}" y="${y(n) + 4}" text-anchor="end">${n / 60}h</text>`;
  }
  days.forEach((day, i) => {
    let base = 0;
    for (const [field, label, css] of [
      ["main_minutes", "主睡眠", "chart-main"],
      ["nap_minutes", "昼寝", "chart-nap"],
    ]) {
      const value = day[field];
      if (value == null) continue;
      const incomplete =
        day[field === "main_minutes" ? "main_incomplete" : "nap_incomplete"];
      const text = `${day.date} ${label} ${durationLabel(value)}${incomplete ? "（時間不明の記録あり）" : ""}`;
      drawing += `<rect class="${css}" tabindex="0" aria-label="${escapeHTML(text)}" x="${x(i) - width * 0.33}" y="${y(base + value)}" width="${width * 0.66}" height="${Math.max(1, y(base) - y(base + value))}" rx="3"><title>${escapeHTML(text)}</title></rect>`;
      base += value;
    }
    if (day.main_minutes == null && day.nap_minutes == null)
      drawing += `<text x="${x(i)}" y="${y(0) - 8}" text-anchor="middle">—</text>`;
  });
  return frame(
    "日別の睡眠時間。濃い緑は主睡眠、薄い緑は昼寝",
    drawing + dateLabels(days, x),
  );
}
export function bedtimeChart(days, target) {
  let targetMinutes = null;
  if (target) {
    const [h, m] = target.split(":").map(Number);
    targetMinutes = h * 60 + m;
    if (targetMinutes < 720) targetMinutes += 1440;
  }
  const values = days.map((d) => d.bedtime).filter((v) => v != null);
  if (targetMinutes != null) values.push(targetMinutes);
  const min = Math.min(1080, ...values.map((v) => Math.floor(v / 120) * 120));
  const max = Math.max(1800, ...values.map((v) => Math.ceil(v / 120) * 120));
  const x = (i) => L + ((W - L - R) / days.length) * (i + 0.5);
  const y = (n) => T + ((n - min) / (max - min)) * (H - T - B);
  let drawing = "";
  for (let n = min; n <= max; n += 120)
    drawing += `<line class="chart-grid" x1="${L}" x2="${W - R}" y1="${y(n)}" y2="${y(n)}"/><text x="${L - 10}" y="${y(n) + 4}" text-anchor="end">${clockLabel(n)}</text>`;
  if (targetMinutes != null)
    drawing += `<line class="chart-target" x1="${L}" x2="${W - R}" y1="${y(targetMinutes)}" y2="${y(targetMinutes)}"><title>目標 ${clockLabel(targetMinutes)}</title></line>`;
  days.forEach((day, i) => {
    if (day.bedtime == null) return;
    if (i > 0 && days[i - 1].bedtime != null)
      drawing += `<line class="chart-trend" x1="${x(i - 1)}" y1="${y(days[i - 1].bedtime)}" x2="${x(i)}" y2="${y(day.bedtime)}"/>`;
    const text = `${day.date}の主睡眠 就寝 ${clockLabel(day.bedtime)}`;
    drawing += `<circle class="chart-main" tabindex="0" aria-label="${escapeHTML(text)}" cx="${x(i)}" cy="${y(day.bedtime)}" r="5"><title>${escapeHTML(text)}</title></circle>`;
  });
  return frame("就寝時刻の推移。破線は目標時刻", drawing + dateLabels(days, x));
}
