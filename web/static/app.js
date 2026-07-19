/* Panel helpers: toast bridge + htmx error surfacing. Alpine/htmx cover UI interactions. */

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
