import { useMutation, type QueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

import { acknowledgeNotification, compilePrepareBundle, rebuildNotifications, startJob, submitPRDApproval, submitTemplateApproval, type PublicNotification, type PublicJobRecord } from "@/api/system"
import { executeSuggestedJobAction, handleNotificationAction as handleNotificationActionView } from "@/components/jobs/jobs-page-actions"
import { getErrorMessage } from "@/components/jobs/jobs-page-utils"
import { translateStatusAction, type ExecutableStatusAction } from "@/components/jobs/jobs-action-utils"
import type { JobsWorkspaceController } from "@/components/jobs/jobs-workspace-controller"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function useJobsWorkspaceActions({
  t,
  queryClient,
  refreshAll,
  refreshOrchestrator,
  startWatch,
  workspaceController,
  watcherRunning,
  watcherStopping,
  watchLockState,
}: {
  t: Translate
  queryClient: QueryClient
  refreshAll: () => Promise<void>
  refreshOrchestrator: () => Promise<void>
  startWatch: () => void
  workspaceController: JobsWorkspaceController
  watcherRunning: boolean
  watcherStopping: boolean
  watchLockState?: string
}) {
  const rebuildNotificationsMutation = useMutation({
    mutationFn: rebuildNotifications,
    onSuccess: async () => {
      toast.success(t("jobs.toasts.notificationsRebuilt"))
      await queryClient.invalidateQueries({
        queryKey: ["appfactory", "notifications"],
      })
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const acknowledgeMutation = useMutation({
    mutationFn: acknowledgeNotification,
    onSuccess: async () => {
      toast.success(t("jobs.toasts.notificationAcked"))
      await queryClient.invalidateQueries({
        queryKey: ["appfactory", "notifications"],
      })
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const statusActionMutation = useMutation({
    mutationFn: ({
      job,
      action,
    }: {
      job: PublicJobRecord
      action: ExecutableStatusAction
    }) =>
      executeSuggestedJobAction(job, action, {
        submitPRDApproval,
        submitTemplateApproval,
        compilePrepareBundle,
        startJob,
      }),
    onSuccess: async (_result, variables) => {
      toast.success(
        t("jobs.toasts.actionCompleted", {
          action: translateStatusAction(variables.action, t),
        }),
      )
      await refreshAll()
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.actionFailed")))
    },
  })

  const handleNotificationAction = (item: PublicNotification) =>
    handleNotificationActionView({
      item,
      workspaceController,
      refreshOrchestrator,
      watcherRunning,
      watcherStopping,
      watchLockState,
      startWatch,
    })

  return {
    rebuildNotificationsMutation,
    acknowledgeMutation,
    statusActionMutation,
    handleNotificationAction,
  }
}