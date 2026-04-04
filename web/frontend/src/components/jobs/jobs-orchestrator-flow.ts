import { useMutation } from "@tanstack/react-query"
import { toast } from "sonner"

import { startOrchestratorWatch, stopOrchestratorWatch, type OrchestratorStatus, unlockOrchestratorWatch } from "@/api/system"
import { getErrorMessage } from "@/components/jobs/jobs-page-utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function useJobsOrchestratorFlow({
  t,
  data,
  loading,
  refreshOrchestrator,
  intervalSeconds = 15,
}: {
  t: Translate
  data?: OrchestratorStatus
  loading: boolean
  refreshOrchestrator: () => Promise<void>
  intervalSeconds?: number
}) {
  const startWatchMutation = useMutation({
    mutationFn: () => startOrchestratorWatch(intervalSeconds),
    onSuccess: async () => {
      toast.success(t("header.orchestrator.toast.started"))
      await refreshOrchestrator()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("header.orchestrator.toast.error")))
    },
  })

  const stopWatchMutation = useMutation({
    mutationFn: stopOrchestratorWatch,
    onSuccess: async () => {
      toast.success(t("header.orchestrator.toast.stopped"))
      await refreshOrchestrator()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("header.orchestrator.toast.error")))
    },
  })

  const unlockWatchMutation = useMutation({
    mutationFn: () => unlockOrchestratorWatch(true),
    onSuccess: async () => {
      toast.success(t("header.orchestrator.toast.unlocked"))
      await refreshOrchestrator()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("header.orchestrator.toast.error")))
    },
  })

  const orchestratorBusy =
    loading ||
    startWatchMutation.isPending ||
    stopWatchMutation.isPending ||
    unlockWatchMutation.isPending
  const watcherRunning = data?.watch_runner_state === "running"
  const watcherStopping = data?.watch_runner_state === "stopping"
  const lockIssue =
    data?.watch_lock_state === "held_by_other" ||
    data?.watch_lock_state === "stale"

  return {
    startWatchMutation,
    stopWatchMutation,
    unlockWatchMutation,
    orchestratorBusy,
    watcherRunning,
    watcherStopping,
    lockIssue,
  }
}