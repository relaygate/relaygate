/* Panel helpers: toast bridge + htmx error surfacing + CSRF header. */

function panelCSRFToken() {
  const meta = document.querySelector('meta[name="csrf-token"]');
  if (meta && meta.content) return meta.content;
  const match = document.cookie.match(/(?:^|;\s*)panel_csrf=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : "";
}

document.body.addEventListener("htmx:configRequest", (ev) => {
  const token = panelCSRFToken();
  if (token) {
    ev.detail.headers["X-CSRF-Token"] = token;
  }
});

document.body.addEventListener("htmx:responseError", (ev) => {
  const xhr = ev.detail && ev.detail.xhr;
  let message = "请求失败";
  if (xhr) {
    try {
      const data = JSON.parse(xhr.responseText);
      if (data && data.error) message = data.error;
      else if (xhr.responseText) message = xhr.responseText.slice(0, 240);
    } catch {
      if (xhr.responseText) message = xhr.responseText.slice(0, 240);
    }
  }
  window.dispatchEvent(new CustomEvent("show-toast", { detail: { message, kind: "error" } }));
});

document.body.addEventListener("htmx:sendError", () => {
  window.dispatchEvent(new CustomEvent("show-toast", {
    detail: { message: "网络错误", kind: "error" },
  }));
});
