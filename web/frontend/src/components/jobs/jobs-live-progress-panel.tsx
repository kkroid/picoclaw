import type { PublicJobCurrentRound } from "@/api/system"
import type { JobProgressSummary } from "@/components/jobs/jobs-page-state"
import type { JobExecutionStageSummary } from "@/components/jobs/jobs-state-model"
import { DetailSection, KeyValue } from "@/components/jobs/jobs-page-primitives"
import { cn } from "@/lib/utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobLiveProgressPanel({
  t,
  progress,
  executionStage,
  latestSignalAt,
  latestSignal,
  nextActionLabel,
  currentRound,
}: {
  t: Translate
  progress: JobProgressSummary
  executionStage?: JobExecutionStageSummary
  latestSignalAt: string
  latestSignal: string
  nextActionLabel: string
  currentRound?: PublicJobCurrentRound
}) {
  const currentRoundSummary = currentRound
    ? t("jobs.events.roundSummaryLine", {
        index: 1,
        roundId: currentRound.round_id || t("jobs.detail.none"),
        attempt: currentRound.attempt || 1,
        status: "running",
        phase: currentRound.current_phase || t("jobs.detail.none"),
      })
    : undefined

  return (
    <DetailSection title={t("jobs.detail.liveProgress")}>
      <div
        className={cn(
          "rounded-lg border px-4 py-3",
          progress.tone === "running" && "border-sky-200 bg-sky-50 text-sky-950",
          progress.tone === "blocked" && "border-amber-200 bg-amber-50 text-amber-950",
          progress.tone === "failed" && "border-rose-200 bg-rose-50 text-rose-950",
          progress.tone === "completed" && "border-emerald-200 bg-emerald-50 text-emerald-950",
          progress.tone === "idle" && "border-border/60 bg-muted/30 text-foreground",
        )}
      >
        <div className="grid gap-3 sm:grid-cols-2">
          <KeyValue label={t("jobs.detail.currentActivity")} value={progress.headline} />
          <KeyValue label={t("jobs.detail.currentStage")} value={executionStage?.stageLabel} />
          <KeyValue label={t("jobs.detail.currentStep")} value={executionStage?.stepLabel} />
          <KeyValue label={t("jobs.detail.latestSignalAt")} value={latestSignalAt} />
          <KeyValue label={t("jobs.detail.latestSignal")} value={latestSignal} />
          <KeyValue label={t("jobs.detail.nextAction")} value={nextActionLabel} />
          <KeyValue label={t("jobs.detail.currentRound")} value={currentRoundSummary} />
        </div>
        {progress.detail ? <div className="mt-3 text-sm/6 opacity-85">{progress.detail}</div> : null}
        {executionStage?.detail ? (
          <div className="mt-2 text-xs/5 opacity-80">{executionStage.detail}</div>
        ) : null}
        {currentRound?.phase_trace && currentRound.phase_trace.length > 0 ? (
          <div className="mt-2 text-xs/5 opacity-80">
            {t("jobs.events.roundPhaseTrace")}: {currentRound.phase_trace.join(" -> ")}
          </div>
        ) : null}
        {currentRound?.target_paths && currentRound.target_paths.length > 0 ? (
          <div className="mt-1 text-xs/5 opacity-80">
            {t("jobs.detail.currentRoundTargets")}: {currentRound.target_paths.join(", ")}
          </div>
        ) : null}
      </div>
    </DetailSection>
  )
}