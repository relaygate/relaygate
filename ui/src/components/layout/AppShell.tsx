import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom"
import { useTranslation } from "react-i18next"
import {
  ActivityIcon,
  ArrowLeftRightIcon,
  FileClockIcon,
  FileCodeIcon,
  LanguagesIcon,
  LayoutDashboardIcon,
  LogOutIcon,
  MoonIcon,
  MonitorIcon,
  PlayIcon,
  ServerIcon,
  ShieldIcon,
  SunIcon,
  UserRoundIcon,
  WrenchIcon,
} from "lucide-react"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Sidebar,
  SidebarContent,
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
import { useTheme } from "@/components/theme-provider"
import { cn } from "@/lib/utils"


const navButtonClass = cn(
  "h-9 gap-2.5 px-2.5 text-[13px] text-muted-foreground transition-colors duration-150",
  "hover:bg-muted/50 hover:text-foreground",
  "data-active:bg-muted/70 data-active:font-medium data-active:text-primary",
  "data-active:hover:bg-muted/70 data-active:hover:text-primary",
  "[&_svg]:text-current",
)

const navItems = [
  { to: "/", labelKey: "nav.overview", icon: LayoutDashboardIcon, end: true },
  { to: "/servers", labelKey: "nav.servers", icon: ServerIcon },
  { to: "/rules", labelKey: "nav.rules", icon: ArrowLeftRightIcon },
  { to: "/acl", labelKey: "nav.acl", icon: ShieldIcon },
  { to: "/config", labelKey: "nav.config", icon: FileCodeIcon },
  { to: "/apply", labelKey: "nav.apply", icon: PlayIcon },
  { to: "/ops", labelKey: "nav.ops", icon: WrenchIcon },
  { to: "/changes", labelKey: "nav.changes", icon: FileClockIcon },
] as const

function LangMenu() {
  const { t, i18n } = useTranslation()
  const lang = i18n.language === "en" ? "en" : "zh-CN"

  async function switchLang(next: string) {
    if (next !== "en" && next !== "zh-CN") return
    if (next === lang) return
    await changePanelLang(next as PanelLang)
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="ghost" size="sm" className="gap-1.5 text-muted-foreground" />
        }
      >
        <LanguagesIcon data-icon="inline-start" />
        <span className="hidden sm:inline">{lang === "en" ? "EN" : "中文"}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-36">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t("shell.language")}</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={lang} onValueChange={switchLang}>
            <DropdownMenuRadioItem value="zh-CN">{t("lang.zh_CN")}</DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="en">{t("lang.en")}</DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function ThemeMenu() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="ghost" size="icon-sm" className="text-muted-foreground" />
        }
      >
        {theme === "light" ? <SunIcon /> : theme === "system" ? <MonitorIcon /> : <MoonIcon />}
        <span className="sr-only">{t("shell.theme")}</span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-40">
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t("shell.theme")}</DropdownMenuLabel>
          <DropdownMenuRadioGroup
            value={theme}
            onValueChange={(v) => setTheme(v as "dark" | "light" | "system")}
          >
            <DropdownMenuRadioItem value="dark">
              <MoonIcon />
              {t("shell.theme_dark")}
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="light">
              <SunIcon />
              {t("shell.theme_light")}
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="system">
              <MonitorIcon />
              {t("shell.theme_system")}
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function UserMenu() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { session, logout, standby } = useSession()
  const roleLabel = standby ? "standby" : session?.role ?? "operator"

  async function handleLogout() {
    await logout()
    navigate("/login", { replace: true })
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button variant="ghost" size="sm" className="gap-2 px-1.5 text-muted-foreground" />
        }
      >
        <Avatar className="size-6">
          <AvatarFallback className="bg-primary/15 text-primary">
            <UserRoundIcon className="size-3.5" aria-hidden />
          </AvatarFallback>
        </Avatar>
        <span className="hidden max-w-24 truncate text-xs font-medium text-foreground sm:inline">
          {roleLabel}
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-48">
        <DropdownMenuGroup>
          <DropdownMenuLabel className="font-normal">
            <div className="flex flex-col gap-0.5">
              <span className="text-sm font-medium text-foreground">{t("shell.signed_in")}</span>
              <span className="font-mono text-[11px] text-muted-foreground">{roleLabel}</span>
            </div>
          </DropdownMenuLabel>
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuGroup>
          <DropdownMenuItem variant="destructive" onClick={handleLogout}>
            <LogOutIcon />
            {t("nav.logout")}
          </DropdownMenuItem>
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function AppShell() {
  const { t } = useTranslation()
  const location = useLocation()
  const { standby } = useSession()
  function navActive(to: string, end?: boolean) {
    if (end) return location.pathname === to
    return location.pathname === to || location.pathname.startsWith(`${to}/`)
  }

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon" variant="sidebar" className="border-r border-sidebar-border">
        <SidebarHeader className="h-12 justify-center border-b border-sidebar-border px-2">
          <div className="flex items-center gap-2 px-1">
            <img src="/favicon.svg" alt="" className="size-6 rounded-md" />
            <div className="flex min-w-0 flex-col group-data-[collapsible=icon]:hidden">
              <span className="truncate text-[13px] font-semibold tracking-wide">RelayGate</span>
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent className="px-2 py-2">
          <SidebarGroup className="p-0">
            <SidebarGroupContent>
              <SidebarMenu className="gap-1">
                {navItems.map(({ to, labelKey, icon: Icon, ...rest }) => (
                  <SidebarMenuItem key={to}>
                    <SidebarMenuButton
                      isActive={navActive(to, "end" in rest ? rest.end : false)}
                      render={
                        <NavLink to={to} end={"end" in rest ? rest.end : false} />
                      }
                      tooltip={t(labelKey)}
                      className={navButtonClass}
                    >
                      <Icon />
                      <span>{t(labelKey)}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
                <SidebarMenuItem>
                  <SidebarMenuButton
                    tooltip={t("nav.monitoring")}
                    render={<NavLink to="/monitoring" />}
                    isActive={navActive("/monitoring")}
                    className={navButtonClass}
                  >
                    <ActivityIcon />
                    <span>{t("nav.monitoring")}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <header className="sticky top-0 z-20 flex h-12 shrink-0 items-center gap-2 border-b border-border/70 bg-background/90 px-3 backdrop-blur-sm">
          <SidebarTrigger className="-ml-0.5" />
          {/* Avoid Separator's data-vertical:self-stretch fighting h-4 (looks like full-height / double rules). */}
          <span aria-hidden className="mx-1 h-4 w-px shrink-0 bg-border" />
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-xs font-medium text-muted-foreground">
              RelayGate Panel
            </span>
            {standby ? (
              <Badge variant="secondary" className="font-mono text-[10px] uppercase">
                read-only
              </Badge>
            ) : null}
          </div>
          <div className="ml-auto flex items-center gap-1.5">
            <LangMenu />
            <ThemeMenu />
            <span aria-hidden className="mx-0.5 h-4 w-px shrink-0 bg-border" />
            <UserMenu />
          </div>
        </header>
        <div className="flex flex-1 flex-col p-4 md:p-5">
          <Outlet key={location.pathname} />
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}
