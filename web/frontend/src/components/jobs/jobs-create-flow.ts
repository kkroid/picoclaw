import { useMutation } from "@tanstack/react-query"
import { toast } from "sonner"

import {
  compilePRD,
  createJob,
  getJob,
  registerBuilder,
  startJob,
  type CompilePRDResponse,
  type PublicJobRecord,
} from "@/api/system"
import {
  createDefaultCreateJobForm,
  type CreateJobFormState,
} from "@/components/jobs/jobs-create-dialog"
import {
  APPFACTORY_AUTO_START_BUILDER_IMAGE,
  buildAutoStartBuilderRequest,
  getErrorMessage,
  isAlreadyRunningStartConflict,
  toOptionalString,
} from "@/components/jobs/jobs-page-utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

type CreateJobFlowResult = {
  job: PublicJobRecord
  compile: CompilePRDResponse
  started: boolean
  startError?: string
}

const JOB_CREATE_REQUIREMENT_SOURCE = "jobs-ui"

export function useJobsCreateFlow({
  t,
  form,
  refreshAll,
  setSelectedJobId,
  setStatusFilter,
  setShowCreateJobSheet,
  setCreateJobForm,
}: {
  t: Translate
  form: CreateJobFormState
  refreshAll: () => Promise<void>
  setSelectedJobId: (jobId: string) => void
  setStatusFilter: (filter: string) => void
  setShowCreateJobSheet: (show: boolean) => void
  setCreateJobForm: (form: CreateJobFormState) => void
}) {
  const createJobMutation = useMutation({
    mutationFn: async (nextForm: CreateJobFormState): Promise<CreateJobFlowResult> => {
      const compileResult = await compilePRD({
        requirement_text: nextForm.requirementText.trim(),
        requirement_source: JOB_CREATE_REQUIREMENT_SOURCE,
        title: toOptionalString(nextForm.title),
        real_checks: nextForm.realChecks,
        executor_image: APPFACTORY_AUTO_START_BUILDER_IMAGE,
      })

      const templateId = toOptionalString(compileResult.template_id)
      if (!templateId) {
        throw new Error(t("jobs.create.validation.templateIdMissing"))
      }

      const createdJob = await createJob({
        prd_id: compileResult.prd_id,
        template_id: templateId,
      })

      if (!nextForm.autoStartBuilder) {
        return { job: createdJob, compile: compileResult, started: false }
      }

      try {
        await registerBuilder(buildAutoStartBuilderRequest(createdJob.job_id))
        const startedJob = await startJob(createdJob.job_id)
        return { job: startedJob, compile: compileResult, started: true }
      } catch (error) {
        if (isAlreadyRunningStartConflict(error)) {
          const runningJob = await getJob(createdJob.job_id)
          return { job: runningJob, compile: compileResult, started: true }
        }

        return {
          job: createdJob,
          compile: compileResult,
          started: false,
          startError: getErrorMessage(error, t("jobs.toasts.jobStartFailed")),
        }
      }
    },
    onSuccess: async ({ job, started, startError }) => {
      setShowCreateJobSheet(false)
      setCreateJobForm(createDefaultCreateJobForm())
      setStatusFilter("all")
      setSelectedJobId(job.job_id)
      await refreshAll()

      if (started) {
        toast.success(t("jobs.toasts.jobCreatedAndStarted", { jobId: job.job_id }))
        return
      }
      if (startError) {
        toast.warning(
          t("jobs.toasts.jobCreatedButStartFailed", {
            jobId: job.job_id,
            reason: startError,
          }),
        )
        return
      }

      toast.success(t("jobs.toasts.jobCreated", { jobId: job.job_id }))
    },
    onError: (error) => {
      toast.error(getErrorMessage(error, t("jobs.toasts.jobCreateFailed")))
    },
  })

  const handleCreateJobSubmit = () => {
    if (form.title.trim() === "") {
      toast.error(t("jobs.create.validation.title"))
      return
    }
    if (form.requirementText.trim() === "") {
      toast.error(t("jobs.create.validation.requirementText"))
      return
    }

    createJobMutation.mutate(form)
  }

  return {
    createJobMutation,
    handleCreateJobSubmit,
  }
}