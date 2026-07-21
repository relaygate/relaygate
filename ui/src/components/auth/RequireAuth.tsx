import { Navigate, Outlet, useLocation } from "react-router-dom"

import { Spinner } from "@/components/ui/spinner"
import { useSession } from "@/context/SessionContext"

export function RequireAuth() {
  const { session, loading } = useSession()
  const location = useLocation()

  if (loading) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Spinner className="size-6 text-primary" />
      </div>
    )
  }

  if (!session?.authenticated) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }

  return <Outlet />
}
