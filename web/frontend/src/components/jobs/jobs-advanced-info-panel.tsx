import type { PublicJobRecord } from "@/api/system"
import { DetailSection, KeyValue } from "@/components/jobs/jobs-page-primitives"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobAdvancedInfoPanel({
  t,
  job,
  formatDate,
}: {
  t: Translate
  job: PublicJobRecord
  formatDate: (value?: string) => string
}) {
  return (
    <details className="rounded-xl border border-border/60 bg-muted/10 p-4">
      <summary className="cursor-pointer list-none text-sm font-medium">
        {t("jobs.workspace.advancedInfo")}
      </summary>
      <div className="mt-4 space-y-4">
        <DetailSection title={t("jobs.detail.runtime")}>
          <div className="grid gap-3 sm:grid-cols-2">
            <KeyValue label={t("jobs.detail.builder")} value={job.runtime?.builder} />
            <KeyValue label={t("jobs.detail.workerPool")} value={job.runtime?.worker_pool} />
            <KeyValue
              label={t("jobs.detail.networkPolicy")}
              value={job.runtime?.network_policy}
            />
            <KeyValue label={t("jobs.detail.phase")} value={job.phase} />
          </div>
        </DetailSection>

        <div className="grid gap-3 border-t border-border/60 pt-4 sm:grid-cols-2">
          <KeyValue label={t("jobs.detail.prd")} value={job.prd_id} />
          <KeyValue label={t("jobs.detail.template")} value={job.template_id} />
          <KeyValue label={t("jobs.detail.createdAt")} value={formatDate(job.created_at)} />
          <KeyValue label={t("jobs.detail.updatedAt")} value={formatDate(job.updated_at)} />
          <KeyValue label={t("jobs.detail.startedAt")} value={formatDate(job.started_at)} />
          <KeyValue label={t("jobs.detail.finishedAt")} value={formatDate(job.finished_at)} />
        </div>

        <DetailSection title={t("jobs.detail.budgets")}>
          <div className="grid gap-3 sm:grid-cols-2">
            <KeyValue
              label={t("jobs.detail.iterationBudget")}
              value={String(job.budgets.iteration_budget)}
            />
            <KeyValue label={t("jobs.detail.tokenBudget")} value={String(job.budgets.token_budget)} />
            <KeyValue
              label={t("jobs.detail.elapsedIterations")}
              value={String(job.budgets.elapsed_iterations ?? 0)}
            />
            <KeyValue
              label={t("jobs.detail.consumedTokens")}
              value={String(job.budgets.consumed_tokens ?? 0)}
            />
          </div>
        </DetailSection>

        <DetailSection title={t("jobs.detail.paths")}>
          <div className="space-y-2">
            <KeyValue label={t("jobs.detail.workspacePath")} value={job.workspace_path} code />
            <KeyValue label={t("jobs.detail.artifactDir")} value={job.artifact_dir} code />
            <KeyValue
              label={t("jobs.detail.builderInputPath")}
              value={job.builder_input_path}
              code
            />
            <KeyValue
              label={t("jobs.detail.builderOutputPath")}
              value={job.builder_output_path}
              code
            />
            <KeyValue label={t("jobs.detail.summaryLogPath")} value={job.logs?.summary_path} code />
            <KeyValue label={t("jobs.detail.eventLogPath")} value={job.logs?.event_log_path} code />
          </div>
        </DetailSection>
      </div>
    </details>
  )
}