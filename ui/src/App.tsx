import { Navigate, Route, Routes } from "react-router-dom"

import { RequireAuth } from "@/components/auth/RequireAuth"
import { AppShell } from "@/components/layout/AppShell"
import { SecurityPage } from "@/pages/SecurityPage"
import { ApplyPage } from "@/pages/ApplyPage"
import { ChangesPage } from "@/pages/ChangesPage"
import { ConfigPage } from "@/pages/ConfigPage"
import { LoginPage } from "@/pages/LoginPage"
import { MonitoringPage } from "@/pages/MonitoringPage"
import { FleetPage } from "@/pages/FleetPage"
import { OpsPage } from "@/pages/OpsPage"
import { OverviewPage } from "@/pages/OverviewPage"
import { RulesPage } from "@/pages/RulesPage"
import { ServersPage } from "@/pages/ServersPage"

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<RequireAuth />}>
        <Route element={<AppShell />}>
          <Route index element={<OverviewPage />} />
          <Route path="upstreams" element={<ServersPage />} />
          <Route path="forwards" element={<RulesPage />} />
          <Route path="security" element={<SecurityPage />} />
          <Route path="acl" element={<Navigate to="/security" replace />} />
          <Route path="config" element={<ConfigPage />} />
          <Route path="apply" element={<ApplyPage />} />
          <Route path="ops" element={<OpsPage />} />
          <Route path="fleet" element={<FleetPage />} />
          <Route path="changes" element={<ChangesPage />} />
          <Route path="monitoring" element={<MonitoringPage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
