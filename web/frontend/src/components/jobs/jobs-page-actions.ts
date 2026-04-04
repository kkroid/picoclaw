import type { QueryClient } from "@tanstack/react-query"

import type { PublicJobDeliveryContext, PublicJobRecord, PublicNotification } from "@/api/system"
import { isExecutableNotificationAction, type ExecutableStatusAction } from "@/components/jobs/jobs-action-utils"
import type { JobsWorkspaceController } from "@/components/jobs/jobs-workspace-controller"

export async function refreshAllJobWorkspace(queryClient: QueryClient) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["appfactory", "jobs"] }),
    queryClient.invalidateQueries({ queryKey: ["appfactory", "notifications"] }),
    queryClient.invalidateQueries({ queryKey: ["system", "orchestrator-status"] }),
    queryClient.invalidateQueries({ queryKey: ["appfactory", "job"] }),
    queryClient.invalidateQueries({ queryKey: ["appfactory", "job-events"] }),
    queryClient.invalidateQueries({ queryKey: ["appfactory", "job-artifacts"] }),
  ])
}

export async function refreshOrchestratorStatus(queryClient: QueryClient) {
  await queryClient.invalidateQueries({
    queryKey: ["system", "orchestrator-status"],
  })
}

export async function executeSuggestedJobAction(
  job: PublicJobRecord,
  action: ExecutableStatusAction,
  deps: {
    submitPRDApproval: (prdId: string, jobId: string) => Promise<unknown>
    submitTemplateApproval: (templateId: string, jobId: string, prdId: string) => Promise<unknown>
    compilePrepareBundle: (jobId: string) => Promise<unknown>
    startJob: (jobId: string) => Promise<unknown>
  },
) {
  switch (action) {
    case "submit_prd_approval":
      return deps.submitPRDApproval(job.prd_id, job.job_id)
    case "submit_template_approval":
      return deps.submitTemplateApproval(job.template_id, job.job_id, job.prd_id)
    case "compile_prepare_bundle":
      return deps.compilePrepareBundle(job.job_id)
    case "start":
      return deps.startJob(job.job_id)
  }
}

export function runDeliverySuggestedAction({
  job,
  context,
  workspaceController,
}: {
  job: PublicJobRecord
  context: PublicJobDeliveryContext
  workspaceController: JobsWorkspaceController
}) {
  if (!isExecutableNotificationAction(context.suggested_action)) {
    return
  }
  workspaceController.openJobDelivery(
    job.job_id,
    context.suggested_action === "monitor_release_feedback"
      ? "jobs-delivery-follow-up"
      : "jobs-delivery-context",
  )
}

export function handleNotificationAction({
  item,
  workspaceController,
  refreshOrchestrator,
  watcherRunning,
  watcherStopping,
  watchLockState,
  startWatch,
}: {
  item: PublicNotification
  workspaceController: JobsWorkspaceController
  refreshOrchestrator: () => Promise<void>
  watcherRunning: boolean
  watcherStopping: boolean
  watchLockState?: string
  startWatch: () => void
}) {
  if (!isExecutableNotificationAction(item.suggested_action)) {
    return
  }

  switch (item.suggested_action) {
    case "inspect_execution_history":
      if (!item.job_id) {
        return
      }
      workspaceController.openJobExecution(item.job_id)
      return
    case "inspect_device_evidence":
    case "inspect_orchestrator_status":
    case "inspect_orchestrator_lock_owner":
      if (item.suggested_action === "inspect_device_evidence" && item.job_id) {
        workspaceController.openJobArtifacts(item.job_id)
        return
      }
      workspaceController.focusOrchestrator()
      void refreshOrchestrator()
      return
    case "restart_orchestrator_watch":
      workspaceController.focusOrchestrator()
      if (!watcherRunning && !watcherStopping && watchLockState !== "held_by_other") {
        startWatch()
        return
      }
      void refreshOrchestrator()
      return
    case "inspect_delivery_record":
    case "verify_staged_release":
    case "prepare_signing":
    case "stage_release":
    case "monitor_release_feedback":
    case "update_subject_and_resubmit":
      if (item.job_id) {
        workspaceController.openJobDelivery(
          item.job_id,
          item.suggested_action === "monitor_release_feedback"
            ? "jobs-delivery-follow-up"
            : "jobs-delivery-context",
        )
        return
      }
      workspaceController.focusTarget(
        item.suggested_action === "monitor_release_feedback"
          ? "jobs-delivery-follow-up"
          : "jobs-delivery-context",
      )
      return
  }
}