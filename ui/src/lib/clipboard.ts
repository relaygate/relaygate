/**
 * Copy text to the clipboard.
 * Prefers navigator.clipboard (secure contexts); falls back to a hidden
 * textarea + document.execCommand("copy") for http://IP:port Panel access.
 */
export async function copyText(text: string): Promise<boolean> {
  if (!text) return false

  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Non-secure context (e.g. http://203.0.113.10:9000) or permission denied.
    }
  }

  return copyTextFallback(text)
}

function copyTextFallback(text: string): boolean {
  if (typeof document === "undefined") return false

  const ta = document.createElement("textarea")
  ta.value = text
  ta.setAttribute("readonly", "")
  ta.style.position = "fixed"
  ta.style.top = "0"
  ta.style.left = "0"
  ta.style.width = "1px"
  ta.style.height = "1px"
  ta.style.padding = "0"
  ta.style.border = "none"
  ta.style.outline = "none"
  ta.style.boxShadow = "none"
  ta.style.background = "transparent"
  ta.style.opacity = "0"
  // Avoid iOS zoom / keyboard; keep in viewport for selection.
  ta.style.fontSize = "16px"

  document.body.appendChild(ta)

  const selection = document.getSelection()
  const previousRange = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null

  ta.focus()
  ta.select()
  ta.setSelectionRange(0, text.length)

  let ok = false
  try {
    ok = document.execCommand("copy")
  } catch {
    ok = false
  }

  document.body.removeChild(ta)

  if (previousRange && selection) {
    selection.removeAllRanges()
    selection.addRange(previousRange)
  }

  return ok
}
