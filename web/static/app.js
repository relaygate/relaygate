async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    credentials: "same-origin",
    ...opts,
  });
  const text = await res.text();
  let data;
  try { data = JSON.parse(text); } catch { data = { raw: text }; }
  if (!res.ok) throw new Error(data.error || text || res.statusText);
  return data;
}

function msg(text) {
  const el = document.getElementById("msg");
  if (el) el.textContent = text;
}

document.querySelectorAll(".save-server").forEach((btn) => {
  btn.addEventListener("click", async () => {
    const tr = btn.closest("tr");
    const name = tr.dataset.name;
    try {
      await api("/api/servers/" + encodeURIComponent(name), {
        method: "PUT",
        body: JSON.stringify({
          address: tr.querySelector(".addr").value,
          tcp_port: Number(tr.querySelector(".tcp").value),
          udp_port: Number(tr.querySelector(".udp").value),
          health_check_port: Number(tr.querySelector(".health").value),
          enabled: tr.querySelector(".en").checked,
        }),
      });
      msg("已保存 " + name + "（尚未 Apply）");
    } catch (e) {
      msg("失败: " + e.message);
    }
  });
});

document.querySelectorAll(".rule-toggle").forEach((el) => {
  el.addEventListener("change", async () => {
    const name = el.dataset.name;
    try {
      await api("/api/rules/" + encodeURIComponent(name), {
        method: "PATCH",
        body: JSON.stringify({ enabled: el.checked }),
      });
      msg((el.checked ? "已启用 " : "已禁用 ") + name + "（尚未 Apply）");
    } catch (e) {
      el.checked = !el.checked;
      msg("失败: " + e.message);
    }
  });
});

const applyBtn = document.getElementById("do-apply");
if (applyBtn) {
  applyBtn.addEventListener("click", async () => {
    applyBtn.disabled = true;
    msg("应用中…");
    try {
      const data = await api("/api/apply", { method: "POST", body: "{}" });
      msg((data.summary || "") + "\n" + (data.output || "OK"));
    } catch (e) {
      msg("失败: " + e.message);
    } finally {
      applyBtn.disabled = false;
    }
  });
}
