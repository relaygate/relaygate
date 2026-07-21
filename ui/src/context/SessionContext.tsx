import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"

import { ApiError, getSession, login as apiLogin, logout as apiLogout } from "@/lib/api"
import { syncLangFromSession } from "@/i18n"
import type { Session } from "@/lib/types"

interface SessionContextValue {
  session: Session | null
  loading: boolean
  standby: boolean
  refresh: () => Promise<void>
  login: (password: string) => Promise<void>
  logout: () => Promise<void>
}

const SessionContext = createContext<SessionContextValue | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null)
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    const data = await getSession()
    setSession(data)
    if (data?.lang) await syncLangFromSession(data.lang)
  }, [])

  useEffect(() => {
    refresh()
      .catch(() => setSession(null))
      .finally(() => setLoading(false))
  }, [refresh])

  const login = useCallback(async (password: string) => {
    const res = await apiLogin(password)
    if (!res.ok) throw new ApiError("Login failed", 401)
    await refresh()
  }, [refresh])

  const logout = useCallback(async () => {
    await apiLogout()
    setSession(null)
  }, [])

  const value = useMemo(
    () => ({
      session,
      loading,
      standby: session?.standby === true,
      refresh,
      login,
      logout,
    }),
    [session, loading, refresh, login, logout],
  )

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
}

export function useSession() {
  const ctx = useContext(SessionContext)
  if (!ctx) throw new Error("useSession must be used within SessionProvider")
  return ctx
}

export function useStandby() {
  return useSession().standby
}
