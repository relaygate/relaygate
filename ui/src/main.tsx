import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"

import App from "@/App.tsx"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { SessionProvider } from "@/context/SessionContext"
import "@/i18n"
import "./index.css"

document.documentElement.classList.add("dark")

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <SessionProvider>
        <TooltipProvider>
          <App />
          <Toaster richColors closeButton position="top-right" />
        </TooltipProvider>
      </SessionProvider>
    </BrowserRouter>
  </StrictMode>,
)
