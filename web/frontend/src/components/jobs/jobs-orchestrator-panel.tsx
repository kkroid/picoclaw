import {
  IconLoader2,
  IconLock,
  IconPlayerPlay,
  IconPower,
  IconRefresh,
} from "@tabler/icons-react"

import type { OrchestratorStatus } from "@/api/system"
import { KeyValue } from "@/components/jobs/jobs-page-primitives"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  getOrchestratorDetail,
  getOrchestratorLabel,
  getOrchestratorToneClass,
} from "@/lib/orchestrator-status"
import { cn } from "@/lib/utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsOrchestratorPanel({
  t,
  data,
  showSystemStatus,
  orchestratorBusy,
  watcherRunning,
  watcherStopping,
  lockIssue,
  startPending,
  stopPending,
  unlockPending,
  onToggleShowSystemStatus,
  onStartWatch,
  onStopWatch,
  onUnlockWatch,
  onRefresh,
}: {
  t: Translate
  data: OrchestratorStatus | undefined
  showSystemStatus: boolean
  orchestratorBusy: boolean
  watcherRunning: boolean
  watcherStopping: boolean
  lockIssue: boolean
  startPending: boolean
  stopPending: boolean
  unlockPending: boolean
  onToggleShowSystemStatus: () => void
  onStartWatch: () => void
  onStopWatch: () => void
  onUnlockWatch: () => void
  onRefresh: () => void
}) {
  return (
    <Card id="jobs-orchestrator-card" className="border-border/60 bg-card/80" size="sm">
      <CardHeader>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <CardTitle>{t("jobs.workspace.systemStatusTitle")}</CardTitle>
            <CardDescription>{getOrchestratorDetail(data, t)}</CardDescription>
          </div>
          <Button variant="outline" size="sm" onClick={onToggleShowSystemStatus}>
            {showSystemStatus
              ? t("jobs.workspace.hideSystemStatus")
              : t("jobs.workspace.showSystemStatus")}
          </Button>
        </div>
      </CardHeader>
      {showSystemStatus ? (
        <CardContent className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div className="space-y-1 text-sm">
            <div className="flex items-center gap-2">
              <span
                className={cn(
                  "inline-flex size-2 rounded-full",
                  getOrchestratorToneClass(data),
                )}
              />
              <span className="font-medium">{getOrchestratorLabel(data, t)}</span>
            </div>
            {data?.watch_lock_owner_id ? (
              <div className="text-muted-foreground text-xs">
                {t("header.orchestrator.menu.lockOwner", {
                  owner: data.watch_lock_owner_id,
                })}
              </div>
            ) : null}
            {data?.watch_runner_last_error ? (
              <div className="text-destructive text-xs">{data.watch_runner_last_error}</div>
            ) : null}
            {data ? (
              <div className="grid gap-2 pt-2 sm:grid-cols-3">
                <KeyValue
                  label={t("jobs.orchestrator.queuedRecoveries")}
                  value={String(data.queued_recoveries ?? 0)}
                  inline
                />
                <KeyValue
                  label={t("jobs.orchestrator.runningRecoveries")}
                  value={String(data.running_recoveries ?? 0)}
                  inline
                />
                <KeyValue
                  label={t("jobs.orchestrator.foreignLiveLeaseSkips")}
                  value={String(data.foreign_live_lease_skips ?? 0)}
                  inline
                />
              </div>
            ) : null}
          </div>

          <div className="flex flex-wrap gap-2">
            {watcherRunning || watcherStopping ? (
              <Button variant="outline" size="sm" disabled={orchestratorBusy} onClick={onStopWatch}>
                {stopPending ? (
                  <IconLoader2 className="size-4 animate-spin" />
                ) : (
                  <IconPower className="size-4" />
                )}
                {t("header.orchestrator.action.stopWatch")}
              </Button>
            ) : (
              <Button
                variant="default"
                size="sm"
                disabled={orchestratorBusy || data?.watch_lock_state === "held_by_other"}
                onClick={onStartWatch}
              >
                {startPending ? (
                  <IconLoader2 className="size-4 animate-spin" />
                ) : (
                  <IconPlayerPlay className="size-4" />
                )}
                {t("header.orchestrator.action.startWatch")}
              </Button>
            )}

            <Button
              variant="outline"
              size="sm"
              disabled={orchestratorBusy || !lockIssue}
              onClick={onUnlockWatch}
            >
              {unlockPending ? (
                <IconLoader2 className="size-4 animate-spin" />
              ) : (
                <IconLock className="size-4" />
              )}
              {t("header.orchestrator.action.forceUnlock")}
            </Button>

            <Button variant="outline" size="sm" disabled={orchestratorBusy} onClick={onRefresh}>
              <IconRefresh className="size-4" />
              {t("header.orchestrator.action.refresh")}
            </Button>
          </div>
        </CardContent>
      ) : null}
    </Card>
  )
}