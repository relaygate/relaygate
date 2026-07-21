import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"

import App from "@/App.tsx"
import { ThemeProvider } from "@/components/theme-provider"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { SessionProvider } from "@/context/SessionContext"
import "@/i18n"
import "./index.css"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="dark" storageKey="relaygate-theme">
      <BrowserRouter>
        <SessionProvider>
          <TooltipProvider>
            <App />
            <Toaster richColors closeButton position="top-right" />
          </TooltipProvider>
        </SessionProvider>
      </BrowserRouter>
    </ThemeProvider>
  </StrictMode>,
)
