import { DetailSection, EmptyBlock, ErrorBlock, KeyValue, LoadingBlock } from "@/components/jobs/jobs-page-primitives"
import { cn } from "@/lib/utils"
import type { PublicJobCurrentRound } from "@/api/system"

type Translate = (key: string, options?: Record<string, unknown>) => string

export type ExecutionCheckpointState = "pending" | "running" | "completed" | "failed"

export type ExecutionCheckpointItem = {
  key: string
  state: ExecutionCheckpointState
  stageLabel: string
  stepLabel: string
  detail?: string
  at?: string
}

export type ExecutionEventItem = {
  at: string
  type: string
  summary?: string
  round_id?: string
  attempt?: number
  checkpoint_key?: string
  current_phase?: string
  phase_trace?: string[]
  round_summaries?: {
    round_id?: string
    attempt?: number
    status?: string
    summary?: string
    current_phase?: string
    phase_trace?: string[]
    target_paths?: string[]
    modified_paths?: string[]
    file_facts?: {
      path: string
      state?: string
      change_type?: string
    }[]
    failed_checks?: string[]
    failure_signatures?: string[]
  }[]
  stage?: string
  status?: string
  target_paths?: string[]
  affected_paths?: string[]
  file_facts?: {
    path: string
    state?: string
    change_type?: string
  }[]
  delivery_status?: string
  device_verification_status?: string
  release_follow_up_status?: string
  failure_domain?: string
  failure_category?: string
  snapshot_path?: string
  delivery_record_path?: string
  release_channel?: string
  recommended_resume_mode?: string
}

type EventFactItem = {
  key: string
  label: string
  value: string | string[]
  mono?: boolean
}

type FileFactItem = {
  path: string
  state?: string
  change_type?: string
}

type RoundActivityTimelineItem = {
  key: string
  at?: string
  type: string
  summary: string
  checkpointKey?: string
  currentPhase?: string
  targetPaths: string[]
  affectedPaths: string[]
}

type RoundActivityGroup = {
  key: string
  roundId: string
  attempt: number
  status?: string
  currentPhase?: string
  checkpointKey?: string
  phaseTrace: string[]
  eventTypes: string[]
  targetPaths: string[]
  affectedPaths: string[]
  latestAt?: string
  latestSummary?: string
  timeline: RoundActivityTimelineItem[]
}

export function JobsExecutionPanel({
  t,
  loading,
  errorMessage,
  hasBreakdown,
  currentStage,
  currentStep,
  latestSignalAt,
  stageDetail,
  currentRound,
  eventCount,
  stateSummary,
  checkpoints,
  recentEvents,
  totalEventCount,
  allEvents,
  formatDate,
  renderDeliveryStatusLabel,
  renderDeviceVerificationStatusLabel,
  renderReleaseFollowUpStatusLabel,
  renderFailureDomainLabel,
  renderFailureCategoryLabel,
}: {
  t: Translate
  loading: boolean
  errorMessage?: string
  hasBreakdown: boolean
  currentStage?: string
  currentStep?: string
  latestSignalAt: string
  stageDetail?: string
  currentRound?: PublicJobCurrentRound
  eventCount: number
  stateSummary: Record<ExecutionCheckpointState, number>
  checkpoints: ExecutionCheckpointItem[]
  recentEvents: ExecutionEventItem[]
  totalEventCount: number
  allEvents: ExecutionEventItem[]
  formatDate: (value?: string) => string
  renderDeliveryStatusLabel: (status: string) => string
  renderDeviceVerificationStatusLabel: (status: string) => string
  renderReleaseFollowUpStatusLabel: (status: string) => string
  renderFailureDomainLabel: (domain: string) => string
  renderFailureCategoryLabel: (category: string) => string
}) {
  const roundActivityGroups = buildRoundActivityGroups(allEvents, t)

  return (
    <div id="jobs-events" className="space-y-6">
      {hasBreakdown ? (
        <DetailSection title={t("jobs.detail.executionSnapshot")}>
          <div className="rounded-xl border border-border/60 bg-muted/20 p-4">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <KeyValue label={t("jobs.detail.currentStage")} value={currentStage} />
              <KeyValue label={t("jobs.detail.currentStep")} value={currentStep} />
              <KeyValue label={t("jobs.detail.latestSignalAt")} value={latestSignalAt} />
              <KeyValue label={t("jobs.detail.eventCount")} value={String(eventCount)} />
              <KeyValue label={t("jobs.detail.currentRound")} value={buildCurrentRoundSummary(currentRound, t)} />
            </div>
            <div className="mt-4 flex flex-wrap gap-2">
              {([
                "running",
                "pending",
                "completed",
                "failed",
              ] as ExecutionCheckpointState[]).map((state) => (
                <span
                  key={state}
                  className={cn(
                    "rounded-md px-2.5 py-1 text-xs font-medium",
                    state === "running" && "bg-sky-100 text-sky-800",
                    state === "pending" && "bg-muted text-muted-foreground",
                    state === "completed" && "bg-emerald-100 text-emerald-800",
                    state === "failed" && "bg-rose-100 text-rose-800",
                  )}
                >
                  {t(`jobs.progress.state.${state}`)} {stateSummary[state]}
                </span>
              ))}
            </div>
            {stageDetail ? (
              <div className="mt-3 text-sm/6 text-muted-foreground">{stageDetail}</div>
            ) : null}
            {currentRound?.phase_trace && currentRound.phase_trace.length > 0 ? (
              <div className="mt-2 text-sm/6 text-muted-foreground">
                {t("jobs.events.roundPhaseTrace")}: {currentRound.phase_trace.join(" -> ")}
              </div>
            ) : null}
            {currentRound?.target_paths && currentRound.target_paths.length > 0 ? (
              <div className="mt-1 text-sm/6 text-muted-foreground">
                {t("jobs.detail.currentRoundTargets")}: {currentRound.target_paths.join(", ")}
              </div>
            ) : null}
          </div>
        </DetailSection>
      ) : null}

      {hasBreakdown ? (
        <DetailSection title={t("jobs.detail.executionBreakdown")}>
          <div className="grid gap-3">
            {checkpoints.map((item) => (
              <ExecutionCheckpointRow
                key={item.key}
                item={item}
                t={t}
                formatDate={formatDate}
              />
            ))}
          </div>
        </DetailSection>
      ) : null}

      {roundActivityGroups.length > 0 ? (
        <DetailSection title={t("jobs.events.roundActivityTitle")}>
          <div className="grid gap-3">
            {roundActivityGroups.map((group) => (
              <RoundActivityCard
                key={group.key}
                group={group}
                t={t}
                formatDate={formatDate}
              />
            ))}
          </div>
        </DetailSection>
      ) : null}

      <DetailSection title={t("jobs.events.recentTitle")}>
        {loading ? (
          <LoadingBlock label={t("labels.loading")} />
        ) : errorMessage ? (
          <ErrorBlock message={errorMessage} />
        ) : totalEventCount === 0 ? (
          <EmptyBlock message={t("jobs.events.empty")} />
        ) : (
          <div className="space-y-3">
            <div className="text-muted-foreground text-sm/6">
              {t("jobs.events.recentDescription", {
                count: recentEvents.length,
                total: totalEventCount,
              })}
            </div>
            {recentEvents.map((event, index) => (
              <EventRow
                key={`${event.at}-${event.type}-${index}`}
                event={event}
                t={t}
                formatDate={formatDate}
                renderDeliveryStatusLabel={renderDeliveryStatusLabel}
                renderDeviceVerificationStatusLabel={renderDeviceVerificationStatusLabel}
                renderReleaseFollowUpStatusLabel={renderReleaseFollowUpStatusLabel}
                renderFailureDomainLabel={renderFailureDomainLabel}
                renderFailureCategoryLabel={renderFailureCategoryLabel}
              />
            ))}
            {totalEventCount > recentEvents.length ? (
              <details className="rounded-xl border border-border/60 bg-muted/10 p-4">
                <summary className="cursor-pointer list-none text-sm font-medium">
                  {t("jobs.events.fullTitle")}
                </summary>
                <div className="mt-4 space-y-3">
                  <div className="text-muted-foreground text-sm/6">
                    {t("jobs.events.description")}
                  </div>
                  {allEvents.map((event, index) => (
                    <EventRow
                      key={`${event.at}-${event.type}-full-${index}`}
                      event={event}
                      t={t}
                      formatDate={formatDate}
                      renderDeliveryStatusLabel={renderDeliveryStatusLabel}
                      renderDeviceVerificationStatusLabel={renderDeviceVerificationStatusLabel}
                      renderReleaseFollowUpStatusLabel={renderReleaseFollowUpStatusLabel}
                      renderFailureDomainLabel={renderFailureDomainLabel}
                      renderFailureCategoryLabel={renderFailureCategoryLabel}
                    />
                  ))}
                </div>
              </details>
            ) : null}
          </div>
        )}
      </DetailSection>
    </div>
  )
}

function buildCurrentRoundSummary(currentRound: PublicJobCurrentRound | undefined, t: Translate) {
  if (!currentRound) {
    return undefined
  }

  return t("jobs.events.roundSummaryLine", {
    index: 1,
    roundId: currentRound.round_id || t("jobs.detail.none"),
    attempt: currentRound.attempt || 1,
    status: "running",
    phase: currentRound.current_phase || t("jobs.detail.none"),
  })
}

function EventRow({
  event,
  t,
  formatDate,
  renderDeliveryStatusLabel,
  renderDeviceVerificationStatusLabel,
  renderReleaseFollowUpStatusLabel,
  renderFailureDomainLabel,
  renderFailureCategoryLabel,
}: {
  event: ExecutionEventItem
  t: Translate
  formatDate: (value?: string) => string
  renderDeliveryStatusLabel: (status: string) => string
  renderDeviceVerificationStatusLabel: (status: string) => string
  renderReleaseFollowUpStatusLabel: (status: string) => string
  renderFailureDomainLabel: (domain: string) => string
  renderFailureCategoryLabel: (category: string) => string
}) {
  const eventFacts: EventFactItem[] = []

  if (event.round_id || event.attempt || event.current_phase || (event.phase_trace && event.phase_trace.length > 0)) {
    eventFacts.push({
      key: "round_context",
      label: t("jobs.events.roundContext"),
      value: buildHeartbeatRoundFact(event, t),
    })
  }
  if (event.round_summaries && event.round_summaries.length > 0) {
    eventFacts.push({
      key: "round_summaries",
      label: t("jobs.events.roundSummaries"),
      value: event.round_summaries.map((round, index) => buildRoundSummaryFact(round, index + 1, t)),
    })
  }
  if (event.snapshot_path) {
    eventFacts.push({
      key: "snapshot_path",
      label: t("jobs.events.snapshotPath"),
      value: event.snapshot_path,
      mono: true,
    })
  }
  if (event.target_paths && event.target_paths.length > 0) {
    eventFacts.push({
      key: "target_paths",
      label: t("jobs.events.targetPaths"),
      value: event.target_paths,
      mono: true,
    })
  }
  if (event.checkpoint_key) {
    eventFacts.push({
      key: "checkpoint_key",
      label: t("jobs.events.checkpointKey"),
      value: event.checkpoint_key,
      mono: true,
    })
  }
  if (event.affected_paths && event.affected_paths.length > 0) {
    eventFacts.push({
      key: "affected_paths",
      label: t("jobs.events.affectedPaths"),
      value: event.affected_paths,
      mono: true,
    })
  }
  if (event.file_facts && event.file_facts.length > 0) {
    eventFacts.push({
      key: "file_facts",
      label: t("jobs.events.fileFacts"),
      value: event.file_facts.map((item) => renderFileFact(item, t)),
      mono: true,
    })
  }
  if (event.delivery_record_path) {
    eventFacts.push({
      key: "delivery_record_path",
      label: t("jobs.events.deliveryRecordPath"),
      value: event.delivery_record_path,
      mono: true,
    })
  }
  if (event.release_channel) {
    eventFacts.push({
      key: "release_channel",
      label: t("jobs.events.releaseChannel"),
      value: event.release_channel,
    })
  }
  if (event.device_verification_status) {
    eventFacts.push({
      key: "device_verification_status",
      label: t("jobs.events.deviceVerificationStatus"),
      value: renderDeviceVerificationStatusLabel(event.device_verification_status),
    })
  }
  if (event.release_follow_up_status) {
    eventFacts.push({
      key: "release_follow_up_status",
      label: t("jobs.events.releaseFollowUpStatus"),
      value: renderReleaseFollowUpStatusLabel(event.release_follow_up_status),
    })
  }
  if (event.failure_category) {
    eventFacts.push({
      key: "failure_category",
      label: t("jobs.events.failureCategory"),
      value: renderFailureCategoryLabel(event.failure_category),
    })
  }
  if (event.recommended_resume_mode) {
    eventFacts.push({
      key: "recommended_resume_mode",
      label: t("jobs.events.recommendedResumeMode"),
      value: event.recommended_resume_mode,
    })
  }

  return (
    <div className="rounded-xl border border-border/60 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium">
          {renderEventTypeLabel(event.type, t)}
        </span>
        {event.stage ? <span className="text-muted-foreground text-xs">{event.stage}</span> : null}
        {event.status ? <span className="text-muted-foreground text-xs">{event.status}</span> : null}
        {event.delivery_status ? (
          <span className="text-muted-foreground text-xs">
            {renderDeliveryStatusLabel(event.delivery_status)}
          </span>
        ) : null}
        {event.device_verification_status ? (
          <span className="text-muted-foreground text-xs">
            {renderDeviceVerificationStatusLabel(event.device_verification_status)}
          </span>
        ) : null}
        {event.release_follow_up_status ? (
          <span className="text-muted-foreground text-xs">
            {renderReleaseFollowUpStatusLabel(event.release_follow_up_status)}
          </span>
        ) : null}
        {event.failure_domain ? (
          <span className="rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">
            {renderFailureDomainLabel(event.failure_domain)}
          </span>
        ) : null}
      </div>
      <div className="mt-2 text-sm font-medium">{renderEventSummary(event, t)}</div>
      <div className="text-muted-foreground mt-2 space-y-1 text-xs">
        <div>{formatDate(event.at)}</div>
      </div>
      {eventFacts.length > 0 ? (
        <div className="mt-3 grid gap-2 sm:grid-cols-2">
          {eventFacts.map((item) => (
            <div key={item.key} className="rounded-lg border border-border/60 bg-muted/20 px-3 py-2">
              <div className="text-muted-foreground text-[11px] font-medium uppercase tracking-wide">
                {item.label}
              </div>
              {Array.isArray(item.value) ? (
                <div className="mt-1 space-y-1">
                  {item.value.map((entry) => (
                    <div
                      key={`${item.key}-${entry}`}
                      className={cn("text-xs leading-5", item.mono && "font-mono break-all")}
                    >
                      {entry}
                    </div>
                  ))}
                </div>
              ) : (
                <div className={cn("mt-1 text-xs leading-5", item.mono && "font-mono break-all")}>
                  {item.value}
                </div>
              )}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}

function buildHeartbeatRoundFact(event: ExecutionEventItem, t: Translate) {
  const segments = [
    t("jobs.events.roundSummaryLine", {
      index: 1,
      roundId: event.round_id || t("jobs.detail.none"),
      attempt: event.attempt || 1,
      status: event.status || t("jobs.detail.none"),
      phase: event.current_phase || t("jobs.detail.none"),
    }),
  ]

  if (event.phase_trace && event.phase_trace.length > 0) {
    segments.push(`${t("jobs.events.roundPhaseTrace")}: ${event.phase_trace.join(" -> ")}`)
  }
  if (event.checkpoint_key) {
    segments.push(`${t("jobs.events.checkpointKey")}: ${event.checkpoint_key}`)
  }

  return segments.join(" | ")
}

function renderEventTypeLabel(type: string, t: Translate) {
  const key = `jobs.events.type.${type}`
  const translated = t(key)
  return translated === key ? type : translated
}

function renderEventSummary(event: ExecutionEventItem, t: Translate) {
  if (event.summary?.trim()) {
    return event.summary
  }
  if (event.type === "run_patch_generation_started") {
    return t("jobs.events.summary.runPatchGenerationStartedFallback", {
      roundId: event.round_id || t("jobs.detail.none"),
      count: event.target_paths?.length || 0,
    })
  }
  if (event.type === "run_round_started") {
    return t("jobs.events.summary.runRoundStartedFallback", {
      roundId: event.round_id || t("jobs.detail.none"),
      attempt: event.attempt || 1,
    })
  }
  if (event.type === "run_patch_generated") {
    return t("jobs.events.summary.runPatchGeneratedFallback", {
      roundId: event.round_id || t("jobs.detail.none"),
      count: event.target_paths?.length || 0,
    })
  }
  if (event.type === "run_patch_generation_failed") {
    return t("jobs.events.summary.runPatchGenerationFailedFallback", {
      roundId: event.round_id || t("jobs.detail.none"),
      count: event.target_paths?.length || 0,
    })
  }
  if (event.type === "run_patch_applied") {
    return t("jobs.events.summary.runPatchAppliedFallback", {
      roundId: event.round_id || t("jobs.detail.none"),
      count: event.affected_paths?.length || 0,
    })
  }
  if (event.type === "run_patch_apply_failed") {
    return t("jobs.events.summary.runPatchApplyFailedFallback", {
      roundId: event.round_id || t("jobs.detail.none"),
      count: event.target_paths?.length || 0,
    })
  }
  return renderEventTypeLabel(event.type, t)
}

function buildRoundSummaryFact(
  round: NonNullable<ExecutionEventItem["round_summaries"]>[number],
  index: number,
  t: Translate,
) {
  const segments = [
    t("jobs.events.roundSummaryLine", {
      index,
      roundId: round.round_id || `round-${index}`,
      attempt: round.attempt || index,
      status: round.status || t("jobs.detail.none"),
      phase: round.current_phase || t("jobs.detail.none"),
    }),
  ]

  if (round.summary) {
    segments.push(round.summary)
  }
  if (round.phase_trace && round.phase_trace.length > 0) {
    segments.push(`${t("jobs.events.roundPhaseTrace")}: ${round.phase_trace.join(" -> ")}`)
  }
  if (round.target_paths && round.target_paths.length > 0) {
    segments.push(`${t("jobs.events.roundTargetPaths")}: ${round.target_paths.join(", ")}`)
  }
  if (round.file_facts && round.file_facts.length > 0) {
    segments.push(`${t("jobs.events.fileFacts")}: ${round.file_facts.map((item) => renderFileFact(item, t)).join(" | ")}`)
  }
  if (round.failed_checks && round.failed_checks.length > 0) {
    segments.push(`${t("jobs.events.failedChecks")}: ${round.failed_checks.join(", ")}`)
  }
  if (round.modified_paths && round.modified_paths.length > 0) {
    segments.push(`${t("jobs.events.roundModifiedPaths")}: ${round.modified_paths.join(", ")}`)
  }
  if (round.failure_signatures && round.failure_signatures.length > 0) {
    segments.push(`${t("jobs.events.failureSignatures")}: ${round.failure_signatures.join(", ")}`)
  }

  return segments.join(" | ")
}

function buildRoundActivityGroups(events: ExecutionEventItem[], t: Translate) {
  const groups = new Map<string, RoundActivityGroup>()
  for (const event of events) {
    const roundId = event.round_id?.trim()
    if (!roundId) {
      continue
    }
    if (!isRoundActivityEvent(event.type)) {
      continue
    }
    const attempt = event.attempt || 1
    const key = `${roundId}::${String(attempt)}`
    const existing = groups.get(key) || {
      key,
      roundId,
      attempt,
      status: event.status,
      currentPhase: event.current_phase,
      checkpointKey: event.checkpoint_key,
      phaseTrace: [],
      eventTypes: [],
      targetPaths: [],
      affectedPaths: [],
      latestAt: event.at,
      latestSummary: event.summary,
      timeline: [],
    }
    if (!existing.eventTypes.includes(event.type)) {
      existing.eventTypes.push(event.type)
    }
    existing.status = event.status || existing.status
    existing.currentPhase = event.current_phase || existing.currentPhase
    existing.checkpointKey = event.checkpoint_key || existing.checkpointKey
    existing.phaseTrace = mergeUniqueStrings(existing.phaseTrace, event.phase_trace)
    existing.targetPaths = mergeUniqueStrings(existing.targetPaths, event.target_paths)
    existing.affectedPaths = mergeUniqueStrings(existing.affectedPaths, event.affected_paths)
    if (!existing.latestAt || event.at > existing.latestAt) {
      existing.latestAt = event.at
      existing.latestSummary = event.summary || existing.latestSummary
    }
    existing.timeline.push({
      key: `${key}-${event.type}-${event.at || existing.timeline.length}`,
      at: event.at,
      type: event.type,
      summary: renderEventSummary(event, (value, options) => t(value, options)),
      checkpointKey: event.checkpoint_key,
      currentPhase: event.current_phase,
      targetPaths: event.target_paths || [],
      affectedPaths: event.affected_paths || [],
    })
    groups.set(key, existing)
  }
  return Array.from(groups.values())
    .map((group) => ({
      ...group,
      timeline: [...group.timeline].sort((left, right) => (left.at || "").localeCompare(right.at || "")),
    }))
    .sort((left, right) => (right.latestAt || "").localeCompare(left.latestAt || ""))
}

function isRoundActivityEvent(type: string) {
  switch (type) {
    case "run_patch_generation_started":
    case "run_round_started":
    case "run_patch_generated":
    case "run_patch_generation_failed":
    case "run_patch_applied":
    case "run_patch_apply_failed":
    case "run_heartbeat":
      return true
    default:
      return false
  }
}

function mergeUniqueStrings(current: string[], incoming?: string[]) {
  if (!incoming || incoming.length === 0) {
    return current
  }
  const merged = [...current]
  const seen = new Set(current)
  for (const item of incoming) {
    const normalized = item.trim()
    if (!normalized || seen.has(normalized)) {
      continue
    }
    seen.add(normalized)
    merged.push(normalized)
  }
  return merged
}

function renderFileFact(item: FileFactItem, t: Translate) {
  const parts = [item.path]
  const stateLabel = renderFileFactState(item.state, t)
  if (stateLabel) {
    parts.push(stateLabel)
  }
  const changeTypeLabel = renderFileFactChangeType(item.change_type, t)
  if (changeTypeLabel) {
    parts.push(changeTypeLabel)
  }
  return parts.join(" · ")
}

function renderFileFactState(state: string | undefined, t: Translate) {
  if (!state) {
    return ""
  }
  const key = `jobs.events.fileFactState.${state}`
  const translated = t(key)
  return translated === key ? state : translated
}

function renderFileFactChangeType(changeType: string | undefined, t: Translate) {
  if (!changeType) {
    return ""
  }
  const key = `jobs.events.fileFactChangeType.${changeType}`
  const translated = t(key)
  return translated === key ? changeType : translated
}

function RoundActivityCard({
  group,
  t,
  formatDate,
}: {
  group: RoundActivityGroup
  t: Translate
  formatDate: (value?: string) => string
}) {
  const tone = getRoundActivityTone(group)
  const phaseChain = group.timeline.map((item) => renderEventTypeLabel(item.type, t)).join(" -> ")

  return (
    <div
      className={cn(
        "rounded-xl border p-4",
        tone === "failed" && "border-rose-200 bg-rose-50/70",
        tone === "completed" && "border-emerald-200 bg-emerald-50/70",
        tone === "running" && "border-sky-200 bg-sky-50/70",
        tone === "idle" && "border-border/60 bg-muted/10",
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <div className="text-sm font-medium">
            {t("jobs.events.roundSummaryLine", {
              index: group.attempt,
              roundId: group.roundId,
              attempt: group.attempt,
              status: group.status || t("jobs.detail.none"),
              phase: group.currentPhase || t("jobs.detail.none"),
            })}
          </div>
          {group.latestSummary ? (
            <div className="text-muted-foreground text-xs leading-5">{group.latestSummary}</div>
          ) : null}
          {phaseChain ? <div className="text-muted-foreground text-xs leading-5">{phaseChain}</div> : null}
        </div>
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "rounded-md px-2 py-0.5 text-xs font-medium",
              tone === "failed" && "bg-rose-100 text-rose-800",
              tone === "completed" && "bg-emerald-100 text-emerald-800",
              tone === "running" && "bg-sky-100 text-sky-800",
              tone === "idle" && "bg-muted text-muted-foreground",
            )}
          >
            {tone === "idle" ? t("jobs.detail.none") : t(`jobs.progress.state.${tone}`)}
          </span>
          {group.latestAt ? <div className="text-muted-foreground text-xs">{formatDate(group.latestAt)}</div> : null}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        {group.eventTypes.map((type) => (
          <span key={`${group.key}-${type}`} className="rounded-md bg-muted px-2 py-0.5 text-xs font-medium">
            {renderEventTypeLabel(type, t)}
          </span>
        ))}
      </div>
      <div className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <KeyValue label={t("jobs.events.checkpointKey")} value={group.checkpointKey} />
        <KeyValue label={t("jobs.events.roundPhaseTrace")} value={group.phaseTrace.join(" -> ") || undefined} />
        <KeyValue label={t("jobs.events.targetPaths")} value={group.targetPaths.join(", ") || undefined} />
        <KeyValue label={t("jobs.events.affectedPaths")} value={group.affectedPaths.join(", ") || undefined} />
      </div>
      <details className="mt-3 rounded-lg border border-border/60 bg-background/80 px-3 py-2">
        <summary className="cursor-pointer list-none text-xs font-medium">{t("jobs.events.roundActivityDetails")}</summary>
        <div className="mt-3 space-y-2">
          {group.timeline.map((item) => (
            <div key={item.key} className="rounded-lg border border-border/60 bg-muted/10 px-3 py-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="rounded-md bg-muted px-2 py-0.5 text-[11px] font-medium">
                    {renderEventTypeLabel(item.type, t)}
                  </span>
                  {item.currentPhase ? <span className="text-muted-foreground text-[11px]">{item.currentPhase}</span> : null}
                  {item.checkpointKey ? <span className="text-muted-foreground text-[11px]">{item.checkpointKey}</span> : null}
                </div>
                {item.at ? <div className="text-muted-foreground text-[11px]">{formatDate(item.at)}</div> : null}
              </div>
              <div className="mt-2 text-xs leading-5">{item.summary}</div>
              {item.targetPaths.length > 0 ? (
                <div className="text-muted-foreground mt-1 text-[11px] leading-5">
                  {t("jobs.events.targetPaths")}: {item.targetPaths.join(", ")}
                </div>
              ) : null}
              {item.affectedPaths.length > 0 ? (
                <div className="text-muted-foreground mt-1 text-[11px] leading-5">
                  {t("jobs.events.affectedPaths")}: {item.affectedPaths.join(", ")}
                </div>
              ) : null}
            </div>
          ))}
        </div>
      </details>
    </div>
  )
}

function getRoundActivityTone(group: RoundActivityGroup): "running" | "completed" | "failed" | "idle" {
  if (group.eventTypes.includes("run_patch_apply_failed") || group.eventTypes.includes("run_patch_generation_failed")) {
    return "failed"
  }
  if (group.eventTypes.includes("run_patch_applied") || group.status === "completed") {
    return "completed"
  }
  if (
    group.eventTypes.includes("run_patch_generation_started") ||
    group.eventTypes.includes("run_patch_generated") ||
    group.eventTypes.includes("run_round_started") ||
    group.eventTypes.includes("run_heartbeat") ||
    group.status === "running"
  ) {
    return "running"
  }
  return "idle"
}

function ExecutionCheckpointRow({
  item,
  t,
  formatDate,
}: {
  item: ExecutionCheckpointItem
  t: Translate
  formatDate: (value?: string) => string
}) {
  return (
    <div
      className={cn(
        "rounded-xl border px-4 py-3",
        item.state === "running" && "border-sky-200 bg-sky-50 text-sky-950",
        item.state === "completed" && "border-emerald-200 bg-emerald-50 text-emerald-950",
        item.state === "failed" && "border-rose-200 bg-rose-50 text-rose-950",
        item.state === "pending" && "border-border/60 bg-muted/20 text-foreground",
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="space-y-1">
          <div className="text-xs font-medium opacity-80">{item.stageLabel}</div>
          <div className="text-sm font-medium">{item.stepLabel}</div>
        </div>
        <span
          className={cn(
            "rounded-md px-2 py-0.5 text-xs font-medium",
            item.state === "running" && "bg-sky-100 text-sky-800",
            item.state === "completed" && "bg-emerald-100 text-emerald-800",
            item.state === "failed" && "bg-rose-100 text-rose-800",
            item.state === "pending" && "bg-muted text-muted-foreground",
          )}
        >
          {t(`jobs.progress.state.${item.state}`)}
        </span>
      </div>
      {item.detail ? <div className="mt-2 text-xs/5 opacity-80">{item.detail}</div> : null}
      {item.at ? <div className="text-muted-foreground mt-2 text-xs">{formatDate(item.at)}</div> : null}
    </div>
  )
}