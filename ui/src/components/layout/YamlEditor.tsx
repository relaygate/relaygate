import { useMemo } from "react"
import CodeMirror from "@uiw/react-codemirror"
import { yaml } from "@codemirror/lang-yaml"
import { oneDark } from "@codemirror/theme-one-dark"
import { EditorView } from "@codemirror/view"

import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"

function resolveDark(theme: string): boolean {
  if (theme === "dark") return true
  if (theme === "light") return false
  return window.matchMedia("(prefers-color-scheme: dark)").matches
}

const lightTheme = EditorView.theme({
  "&": {
    backgroundColor: "transparent",
    fontSize: "12px",
  },
  ".cm-content": {
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    caretColor: "var(--foreground)",
  },
  ".cm-gutters": {
    backgroundColor: "transparent",
    color: "var(--muted-foreground)",
    border: "none",
  },
  "&.cm-focused": {
    outline: "none",
  },
})

const editorChrome = EditorView.theme({
  "&": {
    height: "100%",
  },
  ".cm-scroller": {
    overflow: "auto",
    fontFamily: "var(--font-mono, ui-monospace, monospace)",
    scrollbarWidth: "thin",
    scrollbarColor: "var(--border) transparent",
  },
  ".cm-scroller::-webkit-scrollbar": {
    width: "10px",
    height: "10px",
  },
  ".cm-scroller::-webkit-scrollbar-thumb": {
    backgroundColor: "var(--border)",
    borderRadius: "9999px",
    border: "2px solid transparent",
    backgroundClip: "content-box",
  },
  ".cm-scroller::-webkit-scrollbar-track": {
    background: "transparent",
  },
  ".cm-editor": {
    height: "100%",
  },
})

export function YamlEditor({
  value,
  onChange,
  readOnly = false,
  className,
  placeholder,
}: {
  value: string
  onChange?: (value: string) => void
  readOnly?: boolean
  className?: string
  placeholder?: string
}) {
  const { theme } = useTheme()
  const dark = resolveDark(theme)

  const extensions = useMemo(
    () => [yaml(), editorChrome, ...(dark ? [] : [lightTheme]), EditorView.lineWrapping],
    [dark],
  )

  return (
    <div
      className={cn(
        "min-h-[28rem] overflow-hidden rounded-lg border border-border bg-muted/30",
        "focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50",
        readOnly && "opacity-95",
        className,
      )}
    >
      <CodeMirror
        value={value}
        height="28rem"
        theme={dark ? oneDark : "light"}
        extensions={extensions}
        editable={!readOnly}
        readOnly={readOnly}
        basicSetup={{
          lineNumbers: true,
          foldGutter: true,
          highlightActiveLine: !readOnly,
          highlightSelectionMatches: !readOnly,
        }}
        placeholder={placeholder}
        onChange={(v) => onChange?.(v)}
        className="text-xs"
      />
    </div>
  )
}
