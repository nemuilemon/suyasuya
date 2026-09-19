export async function request(path, body, method) {
  const r = await fetch("/api" + path, {
    method: method || (body ? "POST" : "GET"),
    headers: { "Content-Type": "application/json", "X-Suyasuya": "1" },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!r.ok) {
    const message = await r.text();
    if (r.status === 401 && path != "/login") {
      window.dispatchEvent(new Event("session-expired"));
    }
    const error = new Error(message.trim());
    error.status = r.status;
    throw error;
  }
  return r.json();
}
