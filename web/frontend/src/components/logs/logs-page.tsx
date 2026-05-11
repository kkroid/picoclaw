import { IconLoader2 } from "@tabler/icons-react"
import { useQuery } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"

import { getOrchestratorStatus } from "@/api/system"
import { PageHeader } from "@/components/page-header"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  getOrchestratorLabel,
  getOrchestratorToneClass,
} from "@/lib/orchestrator-status"

export function LogsPage() {
  const { t } = useTranslation()
  const orchestratorQuery = useQuery({
    queryKey: ["system", "orchestrator-status"],
    queryFn: getOrchestratorStatus,
    refetchInterval: 5000,
  })

  const orchestrator = orchestratorQuery.data
  const toneClass = getOrchestratorToneClass(orchestrator)
  const label = getOrchestratorLabel(orchestrator, t)

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.logs")} />

      <div className="flex flex-1 flex-col gap-4 overflow-auto p-4 sm:p-8">
        <Card className="max-w-3xl rounded-lg">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              {orchestratorQuery.isLoading ? (
                <IconLoader2 className="size-4 animate-spin" />
              ) : (
                <span className={`inline-flex size-2 rounded-full ${toneClass}`} />
              )}
              {label}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3 text-sm text-muted-foreground">
            {orchestrator ? (
              <dl className="grid gap-2 sm:grid-cols-[12rem_1fr]">
                <dt>{t("pages.logs.state")}</dt>
                <dd className="text-foreground">{orchestrator.state}</dd>
                <dt>{t("pages.logs.lastTrigger")}</dt>
                <dd className="text-foreground">
                  {orchestrator.last_trigger || t("pages.logs.unavailable")}
                </dd>
                <dt>{t("pages.logs.updatedAt")}</dt>
                <dd className="text-foreground">{orchestrator.updated_at}</dd>
                <dt>{t("pages.logs.lastError")}</dt>
                <dd className="text-foreground">
                  {orchestrator.last_error || t("pages.logs.none")}
                </dd>
              </dl>
            ) : (
              <p>{t("pages.logs.loading")}</p>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
