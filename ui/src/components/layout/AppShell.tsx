import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ActivityIcon,
  ArrowLeftRightIcon,
  FileClockIcon,
  GlobeIcon,
  LayoutDashboardIcon,
  LogOutIcon,
  PlayIcon,
  ServerIcon,
  ShieldIcon,
  WrenchIcon,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { changePanelLang, type PanelLang } from "@/i18n"
import { useSession } from "@/context/SessionContext"
import { cn } from "@/lib/utils"

const navItems = [
  { to: "/", labelKey: "nav.overview", icon: LayoutDashboardIcon, end: true },
  { to: "/servers", labelKey: "nav.servers", icon: ServerIcon },
  { to: "/rules", labelKey: "nav.rules", icon: ArrowLeftRightIcon },
  { to: "/acl", labelKey: "nav.acl", icon: ShieldIcon },
  { to: "/apply", labelKey: "nav.apply", icon: PlayIcon },
  { to: "/ops", labelKey: "nav.ops", icon: WrenchIcon },
  { to: "/changes", labelKey: "nav.changes", icon: FileClockIcon },
  { to: "/monitoring", labelKey: "nav.monitoring", icon: ActivityIcon },
] as const

function LangSwitch() {
  const { i18n } = useTranslation()
  const lang = i18n.language === "en" ? "en" : "zh-CN"

  async function switchLang(next: PanelLang) {
    if (next === lang) return
    await changePanelLang(next)
  }

  return (
    <div className="flex items-center gap-2 text-[11px]">
      <button
        type="button"
        onClick={() => switchLang("zh-CN")}
        className={cn(
          lang === "zh-CN" ? "font-semibold text-foreground" : "text-muted-foreground hover:text-foreground",
        )}
      >
        中文
      </button>
      <span className="text-border">|</span>
      <button
        type="button"
        onClick={() => switchLang("en")}
        className={cn(
          lang === "en" ? "font-semibold text-foreground" : "text-muted-foreground hover:text-foreground",
        )}
      >
        EN
      </button>
    </div>
  )
}

export function AppShell() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const location = useLocation()
  const { session, logout, standby } = useSession()

  function navActive(to: string, end?: boolean) {
    if (end) return location.pathname === to
    return location.pathname === to || location.pathname.startsWith(`${to}/`)
  }

  async function handleLogout() {
    await logout()
    navigate("/login", { replace: true })
  }

  const roleLabel = standby ? "standby" : session?.role ?? "primary"

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon" variant="inset">
        <SidebarHeader className="border-b border-sidebar-border">
          <div className="flex items-center gap-2.5 px-1 py-1">
            <img src="/favicon.svg" alt="" className="size-8 rounded-lg" />
            <div className="flex min-w-0 flex-col group-data-[collapsible=icon]:hidden">
              <span className="truncate text-sm font-semibold tracking-wide">RelayGate</span>
              <span className="text-[11px] text-muted-foreground">Panel</span>
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {navItems.map(({ to, labelKey, icon: Icon, ...rest }) => (
                  <SidebarMenuItem key={to}>
                    <SidebarMenuButton
                      isActive={navActive(to, "end" in rest ? rest.end : false)}
                      render={
                        <NavLink to={to} end={"end" in rest ? rest.end : false} />
                      }
                      tooltip={t(labelKey)}
                    >
                      <Icon />
                      <span>{t(labelKey)}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter className="gap-3 border-t border-sidebar-border">
          <div className="flex flex-col gap-2 px-1 group-data-[collapsible=icon]:hidden">
            <LangSwitch />
            <div className="flex items-center gap-2">
              <Badge variant={standby ? "secondary" : "outline"} className="font-mono text-[10px] uppercase">
                {roleLabel}
              </Badge>
              {standby ? (
                <span className="text-[11px] text-muted-foreground">{t("error.standby")}</span>
              ) : null}
            </div>
          </div>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton onClick={handleLogout} tooltip={t("nav.logout")}>
                <LogOutIcon data-icon="inline-start" />
                <span>{t("nav.logout")}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border px-4">
          <SidebarTrigger />
          <Separator orientation="vertical" className="mx-1 h-4" />
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <GlobeIcon className="size-3.5" />
            <span>RelayGate Panel</span>
          </div>
          {standby ? (
            <Badge variant="secondary" className="ml-auto font-mono text-[10px] uppercase">
              read-only
            </Badge>
          ) : null}
        </header>
        <div className="flex flex-1 flex-col p-4 md:p-6">
          <Outlet />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
