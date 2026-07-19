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

document.querySelectorAll(".delete-server").forEach((btn) => {
  btn.addEventListener("click", async () => {
    const tr = btn.closest("tr");
    const name = tr.dataset.name;
    if (!confirm("确认删除 Server「" + name + "」？将同步删除其关联规则（含 production / canary），且不会自动 Apply。")) {
      return;
    }
    try {
      const data = await api("/api/servers/" + encodeURIComponent(name), { method: "DELETE" });
      msg("已删除 " + name + "（移除规则 " + (data.removed_rules || 0) + " 条，尚未 Apply）");
      location.reload();
    } catch (e) {
      msg("失败: " + e.message);
    }
  });
});

const addBtn = document.getElementById("add-server");
if (addBtn) {
  addBtn.addEventListener("click", async () => {
    const name = (document.getElementById("new-name").value || "").trim();
    const address = (document.getElementById("new-addr").value || "").trim();
    const tcp = Number(document.getElementById("new-tcp").value);
    const udp = Number(document.getElementById("new-udp").value);
    const health = Number(document.getElementById("new-health").value);
    const enabled = document.getElementById("new-en").checked;
    if (!name || !address) {
      msg("失败: name 与 address 为必填");
      return;
    }
    addBtn.disabled = true;
    try {
      const data = await api("/api/servers", {
        method: "POST",
        body: JSON.stringify({
          name,
          address,
          tcp_port: tcp,
          udp_port: udp,
          health_check_port: health,
          enabled,
        }),
      });
      const rules = (data.rules || []).map((r) => r.name).join(", ");
      msg("已添加 " + name + "，生成规则: " + (rules || "(无)") + "（尚未 Apply）");
      location.reload();
    } catch (e) {
      msg("失败: " + e.message);
    } finally {
      addBtn.disabled = false;
    }
  });
}

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
