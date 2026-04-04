import type { ArtifactItem, PublicJobEvent, PublicJobRecord } from "@/api/system"

type Translate = (key: string, options?: Record<string, unknown>) => string

type ExecutionStepDefinition = {
  key: string
  stageKey: string
  stepKey?: string
  acceptanceCheckId?: string
}

export type JobExecutionStageSummary = {
  stageLabel: string
  stepLabel: string
  detail?: string
}

export type JobExecutionCheckpointState = "pending" | "running" | "completed" | "failed"

export type JobExecutionCheckpoint = {
  key: string
  state: JobExecutionCheckpointState
  stageLabel: string
  stepLabel: string
  detail?: string
  at?: string
}

export type JobExecutionEventStreamSummary = {
  totalCount: number
  recentItems: PublicJobEvent[]
}

export type JobExecutionCheckpointStateSummary = Record<JobExecutionCheckpointState, number>

type FailureGuidanceAction = {
  key: string
  targetId: string
}

export type FailureGuidanceCard = {
  key: string
  title: string
  description: string
  evidencePaths: string[]
  actions: FailureGuidanceAction[]
}

export type FailureDiagnosisSummary = {
  stageLabel: string
  impactLabel: string
  nextActionLabel: string
  summary: string
  evidencePaths: string[]
}

const RUNNING_JOB_STATUSES = new Set(["running", "running_builder"])
const DEVICE_EXECUTION_CHECK_IDS = [
  "check-adb-device-ready",
  "check-install-debug-apk",
  "check-launch-app-and-capture-logcat",
]

const DEFAULT_EXECUTION_STEPS: ExecutionStepDefinition[] = [
  {
    key: "orchestrator-dispatch",
    stageKey: "orchestrator",
    stepKey: "jobs.progress.step.orchestratorDispatch",
  },
  {
    key: "thin-prepare",
    stageKey: "thin-prepare",
    stepKey: "jobs.progress.step.thinPrepare",
  },
  {
    key: "check-flutter-pub-get",
    stageKey: "baseline",
    acceptanceCheckId: "check-flutter-pub-get",
  },
  {
    key: "check-counter-demo-removed",
    stageKey: "cheap",
    acceptanceCheckId: "check-counter-demo-removed",
  },
  {
    key: "check-entry-form-wiring",
    stageKey: "cheap",
    acceptanceCheckId: "check-entry-form-wiring",
  },
  {
    key: "check-local-persistence-wiring",
    stageKey: "cheap",
    acceptanceCheckId: "check-local-persistence-wiring",
  },
  {
    key: "check-flutter-analyze",
    stageKey: "cheap",
    acceptanceCheckId: "check-flutter-analyze",
  },
  {
    key: "check-flutter-test",
    stageKey: "cheap",
    acceptanceCheckId: "check-flutter-test",
  },
  {
    key: "check-flutter-build-apk",
    stageKey: "milestone",
    acceptanceCheckId: "check-flutter-build-apk",
  },
  {
    key: "check-adb-device-ready",
    stageKey: "device",
    acceptanceCheckId: "check-adb-device-ready",
  },
  {
    key: "check-install-debug-apk",
    stageKey: "device",
    acceptanceCheckId: "check-install-debug-apk",
  },
  {
    key: "check-launch-app-and-capture-logcat",
    stageKey: "smoke",
    acceptanceCheckId: "check-launch-app-and-capture-logcat",
  },
]

export function getLatestJobEvent(events: PublicJobEvent[]) {
  let latest: PublicJobEvent | undefined
  let latestAt = Number.NEGATIVE_INFINITY
  for (const event of events) {
    const at = Date.parse(event.at)
    if (Number.isNaN(at)) {
      continue
    }
    if (!latest || at > latestAt) {
      latest = event
      latestAt = at
    }
  }
  return latest
}

export function summarizeExecutionStage(
  job: PublicJobRecord,
  latestEvent: PublicJobEvent | undefined,
  t: Translate,
): JobExecutionStageSummary {
  if (!latestEvent) {
    if (isRunningJobStatus(job.status)) {
      const plannedSteps = getPlannedExecutionSteps(job, new Map())
      const liveCheckpointKey = inferCheckpointKeyFromLiveRound(undefined, job.current_round, plannedSteps)
      const liveStep = plannedSteps.find((step) => step.key === liveCheckpointKey)
      if (liveStep?.acceptanceCheckId) {
        return {
          stageLabel: renderExecutionStageLabel(liveStep.stageKey, t),
          stepLabel: renderAcceptanceCheckLabel(liveStep.acceptanceCheckId, liveStep.acceptanceCheckId, t),
          detail: job.status_context?.summary || t("jobs.detail.none"),
        }
      }
    }
    if (job.status === "completed") {
      return {
        stageLabel: t("jobs.progress.stage.delivery"),
        stepLabel: t("jobs.progress.step.completed"),
      }
    }
    if (job.status === "failed") {
      return {
        stageLabel: t("jobs.progress.stage.failed"),
        stepLabel: job.failure_context?.last_error_summary || t("jobs.status.failed"),
      }
    }
    return {
      stageLabel: t("jobs.progress.stage.pending"),
      stepLabel: job.status_context?.summary || t("jobs.list.idleSummary"),
    }
  }

  const acceptanceCheck = parseAcceptanceCheckSummary(latestEvent.summary)
  if (acceptanceCheck) {
    return {
      stageLabel: renderExecutionStageLabel(latestEvent.stage || acceptanceCheck.stage, t),
      stepLabel: acceptanceCheck.label,
      detail: acceptanceCheck.checkId,
    }
  }

  if (latestEvent.stage === "thin-prepare") {
    return {
      stageLabel: renderExecutionStageLabel(latestEvent.stage, t),
      stepLabel: t("jobs.progress.step.thinPrepare"),
      detail: latestEvent.summary,
    }
  }

  const plannedSteps = getPlannedExecutionSteps(job, new Map())
  const liveCheckpointKey = inferCheckpointKeyFromLiveRound(latestEvent, job.current_round, plannedSteps)
  if (isRunningJobStatus(job.status) && liveCheckpointKey !== "") {
    const liveStep = plannedSteps.find((step) => step.key === liveCheckpointKey)
    return {
      stageLabel: renderExecutionStageLabel(latestEvent.stage || liveStep?.stageKey || job.phase, t),
      stepLabel: liveStep?.acceptanceCheckId
        ? renderAcceptanceCheckLabel(liveStep.acceptanceCheckId, liveStep.acceptanceCheckId, t)
        : liveStep?.stepKey
          ? t(liveStep.stepKey)
          : latestEvent.summary || latestEvent.type || job.status_context?.summary || t("jobs.detail.none"),
      detail: latestEvent.summary || latestEvent.type || t("jobs.detail.none"),
    }
  }

  const latestRoundSummary = getLatestRoundSummary(latestEvent)

  switch (latestEvent.type) {
    case "run_created":
    case "execution_enqueued":
    case "execution_attempt_claimed":
    case "execution_started":
      return {
        stageLabel: t("jobs.progress.stage.orchestrator"),
        stepLabel: t("jobs.progress.step.orchestratorDispatch"),
        detail: describeExecutionEvent(latestEvent, t),
      }
    case "run_completed":
      return {
        stageLabel: t("jobs.progress.stage.delivery"),
        stepLabel: t("jobs.progress.step.completed"),
        detail: getExecutionTerminalDetail(latestEvent, latestRoundSummary),
      }
    case "run_failed":
      return {
        stageLabel: renderExecutionStageLabel(latestEvent.stage, t),
        stepLabel: latestRoundSummary?.failed_checks?.[0] || t("jobs.progress.step.failed"),
        detail: getExecutionTerminalDetail(latestEvent, latestRoundSummary),
      }
    default:
      return {
        stageLabel: renderExecutionStageLabel(latestEvent.stage, t),
        stepLabel:
          latestEvent.summary || latestEvent.type || job.status_context?.summary || t("jobs.detail.none"),
      }
  }
}

export function buildExecutionBreakdown(
  job: PublicJobRecord,
  events: PublicJobEvent[],
  t: Translate,
): JobExecutionCheckpoint[] {
  const latestEvent = getLatestJobEvent(events)
  const acceptanceEvents: Array<{
    event: PublicJobEvent
    acceptanceCheck: {
      checkId: string
      label: string
      stage: string
    }
  }> = []

  for (const event of events) {
    const acceptanceCheck = parseAcceptanceCheckSummary(event.summary)
    if (!acceptanceCheck) {
      continue
    }
    acceptanceEvents.push({ event, acceptanceCheck })
  }

  const latestAcceptanceEvents = new Map<string, (typeof acceptanceEvents)[number]>()
  for (const item of acceptanceEvents) {
    latestAcceptanceEvents.set(item.acceptanceCheck.checkId, item)
  }

  const queueEvents = events.filter((event) =>
    ["run_created", "execution_enqueued", "execution_attempt_claimed", "execution_started"].includes(event.type),
  )
  const latestQueueEvent = queueEvents.length > 0 ? queueEvents[queueEvents.length - 1] : undefined
  const thinPrepareEvent = findLatestMatchingEvent(events, (event) => event.stage === "thin-prepare")
  const plannedSteps = getPlannedExecutionSteps(job, latestAcceptanceEvents)
  const latestRoundSummary = getLatestRoundSummary(latestEvent)
  const latestRoundCheckpointKey = inferCheckpointKeyFromRoundSummary(
    latestRoundSummary,
    latestEvent?.stage,
    plannedSteps,
  )
  const latestCheckpointKey =
    mapEventToCheckpointKey(latestEvent, plannedSteps, job.current_round) ||
    inferCheckpointKeyFromLiveRound(latestEvent, job.current_round, plannedSteps)
  const latestCheckpointIndex = plannedSteps.findIndex((step) => step.key === latestCheckpointKey)
  const checkpoints = plannedSteps.map((step, index) => {
    const event = resolveExecutionStepEvent(step, latestQueueEvent, thinPrepareEvent, latestAcceptanceEvents)
    const acceptanceCheck = step.acceptanceCheckId
      ? latestAcceptanceEvents.get(step.acceptanceCheckId)?.acceptanceCheck
      : undefined
    const roundSummaryMatchesStep =
      step.acceptanceCheckId &&
      latestRoundCheckpointKey !== "" &&
      step.acceptanceCheckId === latestRoundCheckpointKey
    const state = event
      ? classifyCheckpointState(job, latestEvent, event)
      : classifySyntheticCheckpointState(job, latestCheckpointIndex, index)

    return {
      key: step.key,
      state,
      stageLabel: renderExecutionStageLabel(acceptanceCheck?.stage || step.stageKey, t),
      stepLabel: step.acceptanceCheckId
        ? renderAcceptanceCheckLabel(
            step.acceptanceCheckId,
            acceptanceCheck?.label || (roundSummaryMatchesStep ? latestRoundSummary?.failed_checks?.[0] || "" : ""),
            t,
          )
        : t(step.stepKey || "jobs.progress.step.failed"),
      detail: buildExecutionStepDetail(
        step,
        event,
        state,
        t,
        roundSummaryMatchesStep ? latestRoundSummary?.summary : undefined,
      ),
      at: event?.at,
    }
  })

  if (job.status === "completed") {
    checkpoints.push({
      key: "delivery-complete",
      state: "completed",
      stageLabel: t("jobs.progress.stage.delivery"),
      stepLabel: t("jobs.progress.step.completed"),
      detail: getExecutionTerminalDetail(latestEvent, latestRoundSummary),
      at: job.finished_at,
    })
  } else if (job.status === "failed" && latestCheckpointIndex < 0) {
    checkpoints.push({
      key: "delivery-failed",
      state: "failed",
      stageLabel: t("jobs.progress.stage.failed"),
      stepLabel: latestRoundSummary?.failed_checks?.[0] || t("jobs.progress.step.failed"),
      detail:
        getExecutionTerminalDetail(latestEvent, latestRoundSummary) ||
        job.failure_context?.last_error_summary ||
        latestEvent?.summary,
      at: job.updated_at,
    })
  }

  return compressCheckpoints(checkpoints)
}

export function buildExecutionEventStreamSummary(events: PublicJobEvent[]): JobExecutionEventStreamSummary {
  return {
    totalCount: events.length,
    recentItems: events.slice(-6).reverse(),
  }
}

export function summarizeExecutionCheckpointStates(
  items: JobExecutionCheckpoint[],
): JobExecutionCheckpointStateSummary {
  const summary: JobExecutionCheckpointStateSummary = {
    pending: 0,
    running: 0,
    completed: 0,
    failed: 0,
  }

  for (const item of items) {
    summary[item.state] += 1
  }

  return summary
}

export function renderExecutionStageLabel(stage: string | undefined, t: Translate) {
  if (!stage) {
    return t("jobs.progress.stage.pending")
  }
  const key = `jobs.progress.stage.${stage}`
  const translated = t(key)
  return translated === key ? stage : translated
}

export function describeExecutionEvent(event: PublicJobEvent, t: Translate) {
  switch (event.type) {
    case "run_created":
      return t("jobs.progress.step.runCreated")
    case "execution_enqueued":
      return t("jobs.progress.step.executionEnqueued")
    case "execution_attempt_claimed":
      return t("jobs.progress.step.executionClaimed")
    case "execution_started":
      return t("jobs.progress.step.executionStarted")
    default: {
      if (event.summary?.trim()) {
        return event.summary
      }
      const typeKey = `jobs.events.type.${event.type}`
      const translated = t(typeKey)
      return translated === typeKey ? event.type : translated
    }
  }
}

export function renderAcceptanceCheckLabel(checkId: string, fallbackLabel: string, t: Translate) {
  const key = `jobs.progress.check.${checkId}`
  const translated = t(key)
  if (translated !== key) {
    return translated
  }
  return fallbackLabel || checkId
}

export function parseAcceptanceCheckSummary(summary: string | undefined) {
  if (!summary) {
    return undefined
  }
  const trimmed = summary.trim()
  if (!trimmed.startsWith("acceptance check:")) {
    return undefined
  }
  const segments = trimmed.split("|").map((segment) => segment.trim()).filter(Boolean)
  const checkId = segments[0]?.replace(/^acceptance check:\s*/, "") || ""
  const label = segments[1] || checkId
  const stageSegment = segments.find((segment) => segment.startsWith("stage="))
  return {
    checkId,
    label,
    stage: stageSegment?.slice("stage=".length) || "",
  }
}

export function renderRunningStageDetail(stage: string | undefined, at: string | undefined, t: Translate) {
  const stageKey = stage && stage.trim() !== "" ? stage : "generic"
  const translationKey = `jobs.progress.runningHint.${stageKey}`
  const translated = t(translationKey)
  const base = translated === translationKey ? t("jobs.progress.runningHint.generic") : translated
  const elapsed = formatExecutionElapsed(at, t)
  return elapsed ? `${base} · ${elapsed}` : base
}

export function collectProducedArtifactPaths(artifacts: ArtifactItem[], markers: string[]) {
  return [...new Set(
    artifacts
      .filter((item) => item.produced)
      .map((item) => item.path)
      .filter((path) => markers.some((marker) => path.includes(marker))),
  )]
}

export function buildFailureDiagnosisSummary(
  job: PublicJobRecord,
  artifacts: ArtifactItem[],
  guidance: FailureGuidanceCard[],
  t: Translate,
  options: {
    renderSuggestedActionLabel: (action: string | undefined) => string
    renderFailureGuidanceActionLabel: (action: string) => string
  },
): FailureDiagnosisSummary | undefined {
  const failure = job.failure_context
  if (!failure) {
    return undefined
  }

  const guidanceEvidence = guidance.flatMap((item) => item.evidencePaths)
  const fallbackEvidence = collectFailureDiagnosisEvidencePaths(job, artifacts)
  const nextAction = job.status_context?.suggested_action
    ? options.renderSuggestedActionLabel(job.status_context.suggested_action)
    : guidance[0]?.actions[0]
      ? options.renderFailureGuidanceActionLabel(guidance[0].actions[0].key)
      : t("jobs.detail.failureInspectNow")

  return {
    stageLabel: renderExecutionStageLabel(job.phase, t),
    impactLabel: renderFailureImpactLabel(failure.failure_category, failure.failure_domain, t),
    nextActionLabel: nextAction,
    summary:
      failure.last_error_summary ||
      guidance[0]?.description ||
      t("jobs.detail.failedSummaryFallback"),
    evidencePaths: collectUniquePaths([...guidanceEvidence, ...fallbackEvidence]).slice(0, 5),
  }
}

export function buildFailureGuidance(
  job: PublicJobRecord,
  artifacts: ArtifactItem[],
  t: Translate,
  options: {
    renderFailureCategoryLabel: (category: string | undefined) => string
  },
): FailureGuidanceCard[] {
  const failureSignature = job.failure_context?.failure_signature?.trim() ?? ""
  const failureCategory = job.failure_context?.failure_category?.trim() ?? failureSignature
  const runtimeEvidence = collectUniquePaths([
    job.builder_output_path,
    job.logs?.summary_path,
    job.logs?.event_log_path,
  ])
  const validationEvidence = collectUniquePaths([
    ...collectProducedArtifactPaths(artifacts, [
      "build-report",
      "smoke-test-report",
      "app-debug.apk",
    ]),
    ...runtimeEvidence,
  ])
  const deviceEvidence = collectUniquePaths([
    ...collectDeviceEvidencePaths(artifacts),
    ...runtimeEvidence,
  ])
  const failureCategoryLabel = options.renderFailureCategoryLabel(failureCategory)

  if (failureSignature === "builder_runtime_patch_parse_failed") {
    return [{
      key: "builder-runtime-patch-parse",
      title: t("jobs.failureHelp.builderRuntimePatchParse.title"),
      description: t("jobs.failureHelp.builderRuntimePatchParse.description"),
      evidencePaths: runtimeEvidence,
      actions: [
        { key: "inspectEvents", targetId: "jobs-events" },
        { key: "inspectArtifacts", targetId: "jobs-artifacts" },
      ],
    }]
  }

  if (failureSignature === "workspace_patch_apply_failed") {
    return [{
      key: "workspace-patch-apply",
      title: t("jobs.failureHelp.workspacePatchApply.title"),
      description: t("jobs.failureHelp.workspacePatchApply.description"),
      evidencePaths: runtimeEvidence,
      actions: [
        { key: "inspectEvents", targetId: "jobs-events" },
        { key: "inspectArtifacts", targetId: "jobs-artifacts" },
      ],
    }]
  }

  if (failureCategory.startsWith("environment_check_failed:")) {
    return [{
      key: "environment-check-failed",
      title: t("jobs.failureHelp.environmentCheck.title"),
      description: t("jobs.failureHelp.environmentCheck.description", { category: failureCategoryLabel }),
      evidencePaths: validationEvidence,
      actions: [
        { key: "inspectEvents", targetId: "jobs-events" },
        { key: "inspectArtifacts", targetId: "jobs-artifacts" },
      ],
    }]
  }

  if (failureCategory.startsWith("device_check_failed:")) {
    return [{
      key: "device-check-failed",
      title: t("jobs.failureHelp.deviceCheck.title"),
      description: t("jobs.failureHelp.deviceCheck.description", { category: failureCategoryLabel }),
      evidencePaths: deviceEvidence,
      actions: [
        { key: "inspectArtifacts", targetId: "jobs-artifacts" },
        { key: "inspectEvents", targetId: "jobs-events" },
        { key: "inspectOrchestrator", targetId: "jobs-orchestrator-card" },
      ],
    }]
  }

  if (failureCategory.startsWith("profile_check_failed:")) {
    return [{
      key: "profile-check-failed",
      title: t("jobs.failureHelp.profileCheck.title"),
      description: t("jobs.failureHelp.profileCheck.description", { category: failureCategoryLabel }),
      evidencePaths: validationEvidence,
      actions: [
        { key: "inspectEvents", targetId: "jobs-events" },
        { key: "inspectArtifacts", targetId: "jobs-artifacts" },
      ],
    }]
  }

  return [{
    key: "generic-failure",
    title: t("jobs.failureHelp.generic.title"),
    description: t("jobs.failureHelp.generic.description"),
    evidencePaths: validationEvidence,
    actions: [
      { key: "inspectEvents", targetId: "jobs-events" },
      { key: "inspectArtifacts", targetId: "jobs-artifacts" },
    ],
  }]
}

function isRunningJobStatus(status: string) {
  return RUNNING_JOB_STATUSES.has(status)
}

function compressCheckpoints(items: JobExecutionCheckpoint[]) {
  const deduped = new Map<string, JobExecutionCheckpoint>()
  for (const item of items) {
    deduped.set(item.key, item)
  }
  return [...deduped.values()]
}

function findLatestMatchingEvent(
  events: PublicJobEvent[],
  predicate: (event: PublicJobEvent) => boolean,
) {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    if (predicate(events[index])) {
      return events[index]
    }
  }
  return undefined
}

function classifyCheckpointState(
  job: PublicJobRecord,
  latestEvent: PublicJobEvent | undefined,
  checkpointEvent: PublicJobEvent,
): JobExecutionCheckpointState {
  if (job.status === "failed" && latestEvent?.at === checkpointEvent.at) {
    return "failed"
  }
  if (latestEvent?.at === checkpointEvent.at) {
    return isRunningJobStatus(job.status) ? "running" : job.status === "failed" ? "failed" : "completed"
  }
  if (job.status === "failed" && Date.parse(checkpointEvent.at) >= Date.parse(latestEvent?.at || checkpointEvent.at)) {
    return "failed"
  }
  return "completed"
}

function classifySyntheticCheckpointState(
  job: PublicJobRecord,
  latestCheckpointIndex: number,
  stepIndex: number,
): JobExecutionCheckpointState {
  if (latestCheckpointIndex < 0) {
    return "pending"
  }
  if (stepIndex < latestCheckpointIndex) {
    return "completed"
  }
  if (stepIndex > latestCheckpointIndex) {
    return "pending"
  }
  if (job.status === "failed") {
    return "failed"
  }
  if (job.status === "completed") {
    return "completed"
  }
  return isRunningJobStatus(job.status) ? "running" : "pending"
}

function getPlannedExecutionSteps(
  job: PublicJobRecord,
  latestAcceptanceEvents: Map<
    string,
    {
      event: PublicJobEvent
      acceptanceCheck: {
        checkId: string
        label: string
        stage: string
      }
    }
  >,
) {
  const shouldIncludeDeviceSteps =
    DEVICE_EXECUTION_CHECK_IDS.some((checkId) => latestAcceptanceEvents.has(checkId)) ||
    Boolean(job.delivery_context?.device_verification)
  return DEFAULT_EXECUTION_STEPS.filter(
    (step) => shouldIncludeDeviceSteps || !DEVICE_EXECUTION_CHECK_IDS.includes(step.key),
  )
}

function resolveExecutionStepEvent(
  step: ExecutionStepDefinition,
  latestQueueEvent: PublicJobEvent | undefined,
  thinPrepareEvent: PublicJobEvent | undefined,
  latestAcceptanceEvents: Map<
    string,
    {
      event: PublicJobEvent
      acceptanceCheck: {
        checkId: string
        label: string
        stage: string
      }
    }
  >,
) {
  if (step.key === "orchestrator-dispatch") {
    return latestQueueEvent
  }
  if (step.key === "thin-prepare") {
    return thinPrepareEvent
  }
  if (step.acceptanceCheckId) {
    return latestAcceptanceEvents.get(step.acceptanceCheckId)?.event
  }
  return undefined
}

function buildExecutionStepDetail(
  step: ExecutionStepDefinition,
  event: PublicJobEvent | undefined,
  state: JobExecutionCheckpointState,
  t: Translate,
  fallbackDetail?: string,
) {
  if (state === "running") {
    return renderRunningStageDetail(event?.stage || step.stageKey, event?.at, t)
  }
  if (!event) {
    return fallbackDetail
  }
  if (step.key === "orchestrator-dispatch") {
    return describeExecutionEvent(event, t)
  }
  return event.summary || undefined
}

function getLatestRoundSummary(event: PublicJobEvent | undefined) {
  if (!event?.round_summaries || event.round_summaries.length === 0) {
    return undefined
  }
  return event.round_summaries[event.round_summaries.length - 1]
}

function getExecutionTerminalDetail(
  event: PublicJobEvent | undefined,
  roundSummary:
    | NonNullable<PublicJobEvent["round_summaries"]>[number]
    | undefined,
) {
  if (roundSummary?.summary && roundSummary.summary.trim() !== "") {
    return roundSummary.summary
  }
  return event?.summary || undefined
}

function formatExecutionElapsed(at: string | undefined, t: Translate) {
  if (!at) {
    return ""
  }
  const startedAt = Date.parse(at)
  if (Number.isNaN(startedAt)) {
    return ""
  }
  const elapsedMinutes = Math.max(0, Math.floor((Date.now() - startedAt) / 60000))
  if (elapsedMinutes <= 0) {
    return t("jobs.progress.elapsed.justNow")
  }
  if (elapsedMinutes < 60) {
    return t("jobs.progress.elapsed.minutes", { minutes: elapsedMinutes })
  }
  return t("jobs.progress.elapsed.hoursMinutes", {
    hours: Math.floor(elapsedMinutes / 60),
    minutes: elapsedMinutes % 60,
  })
}

function mapEventToCheckpointKey(
  event: PublicJobEvent | undefined,
  plannedSteps: ExecutionStepDefinition[] = DEFAULT_EXECUTION_STEPS,
  currentRound?: PublicJobRecord["current_round"],
) {
  if (!event) {
    return ""
  }
  const acceptanceCheck = parseAcceptanceCheckSummary(event.summary)
  if (acceptanceCheck?.checkId) {
    return acceptanceCheck.checkId
  }
  const roundCheckpointKey = inferCheckpointKeyFromRoundSummary(
    getLatestRoundSummary(event),
    event.stage,
    plannedSteps,
  )
  if (roundCheckpointKey) {
    return roundCheckpointKey
  }
  const liveRoundCheckpointKey = inferCheckpointKeyFromLiveRound(event, currentRound, plannedSteps)
  if (liveRoundCheckpointKey) {
    return liveRoundCheckpointKey
  }
  if (["run_created", "execution_enqueued", "execution_attempt_claimed", "execution_started"].includes(event.type)) {
    return "orchestrator-dispatch"
  }
  if (event.stage === "thin-prepare") {
    return "thin-prepare"
  }
  return ""
}

function inferCheckpointKeyFromLiveRound(
  event: PublicJobEvent | undefined,
  currentRound: PublicJobRecord["current_round"] | undefined,
  plannedSteps: ExecutionStepDefinition[],
) {
  const explicitCheckpointKey = event?.checkpoint_key?.trim() || currentRound?.checkpoint_key?.trim()
  if (explicitCheckpointKey) {
    return explicitCheckpointKey
  }
  const currentPhase = event?.current_phase?.trim() || currentRound?.current_phase?.trim()
  const fallbackStage = event?.stage?.trim() || ""
  if (!currentPhase && fallbackStage === "") {
    return ""
  }
  if (fallbackStage === "" && currentPhase !== "validate" && currentPhase !== "finalize") {
    return ""
  }
  return inferCheckpointKeyFromRoundPhase(currentPhase, fallbackStage || undefined, plannedSteps)
}

function inferCheckpointKeyFromRoundSummary(
  roundSummary:
    | NonNullable<PublicJobEvent["round_summaries"]>[number]
    | undefined,
  fallbackStage: string | undefined,
  plannedSteps: ExecutionStepDefinition[],
) {
  const failedCheck = roundSummary?.failed_checks?.[0]
  if (!failedCheck) {
    return inferCheckpointKeyFromRoundPhase(roundSummary?.current_phase, fallbackStage, plannedSteps)
  }
  return normalizeFailedCheckToCheckpointKey(failedCheck)
}

function inferCheckpointKeyFromRoundPhase(
  currentPhase: string | undefined,
  fallbackStage: string | undefined,
  plannedSteps: ExecutionStepDefinition[],
) {
  const preferredStage = normalizeRoundPhaseStage(currentPhase, fallbackStage)
  if (!preferredStage) {
    return ""
  }

  const candidates = plannedSteps.filter((step) => step.stageKey === preferredStage)
  return candidates.at(-1)?.key || ""
}

function normalizeRoundPhaseStage(currentPhase: string | undefined, fallbackStage: string | undefined) {
  const normalizedStage = fallbackStage?.trim()
  if (normalizedStage) {
    return normalizedStage
  }

  switch (currentPhase?.trim()) {
    case "inspect":
    case "edit":
    case "repair":
      return "cheap"
    case "validate":
      return "cheap"
    case "finalize":
      return "milestone"
    default:
      return ""
  }
}

function normalizeFailedCheckToCheckpointKey(label: string) {
  const normalized = label.trim().toLowerCase()
  if (normalized === "") {
    return ""
  }
  if (normalized.startsWith("check-")) {
    return normalized
  }

  const mappings: Array<[string, string]> = [
    ["flutter pub get", "check-flutter-pub-get"],
    ["pub get", "check-flutter-pub-get"],
    ["counter", "check-counter-demo-removed"],
    ["entry form", "check-entry-form-wiring"],
    ["form wiring", "check-entry-form-wiring"],
    ["local persistence", "check-local-persistence-wiring"],
    ["flutter analyze", "check-flutter-analyze"],
    ["flutter test", "check-flutter-test"],
    ["flutter build apk", "check-flutter-build-apk"],
    ["build apk", "check-flutter-build-apk"],
    ["install debug apk", "check-install-debug-apk"],
    ["install apk", "check-install-debug-apk"],
    ["logcat", "check-launch-app-and-capture-logcat"],
    ["launch app", "check-launch-app-and-capture-logcat"],
    ["adb", "check-adb-device-ready"],
  ]

  for (const [needle, checkId] of mappings) {
    if (normalized.includes(needle)) {
      return checkId
    }
  }

  return ""
}

function collectUniquePaths(items: Array<string | undefined>) {
  return [...new Set(
    items
      .filter((item): item is string => typeof item === "string")
      .map((item) => item.trim())
      .filter((item) => item !== ""),
  )]
}

function collectDeviceEvidencePaths(artifacts: ArtifactItem[]) {
  return collectProducedArtifactPaths(artifacts, [
    "device-logcat",
    "device-screenshot",
    "smoke-test-report",
    "build-report",
    "app-debug.apk",
  ])
}

function collectSuggestedEvidencePaths(artifacts: ArtifactItem[]) {
  return collectProducedArtifactPaths(artifacts, [
    "review-bundle",
    "handoff-checklist",
    "smoke-test-report",
    "build-report",
  ])
}

function collectFailureDiagnosisEvidencePaths(job: PublicJobRecord, artifacts: ArtifactItem[]) {
  const failureCategory = job.failure_context?.failure_category?.trim() ?? ""
  const runtimeEvidence = [
    job.builder_output_path,
    job.logs?.summary_path,
    job.logs?.event_log_path,
  ]

  if (failureCategory.startsWith("device_check_failed:")) {
    return collectUniquePaths([...collectDeviceEvidencePaths(artifacts), ...runtimeEvidence])
  }

  return collectUniquePaths([
    ...collectSuggestedEvidencePaths(artifacts),
    ...collectProducedArtifactPaths(artifacts, ["app-debug.apk"]),
    ...runtimeEvidence,
  ])
}

function renderFailureImpactLabel(category: string | undefined, domain: string | undefined, t: Translate) {
  if (category?.startsWith("device_check_failed:")) {
    return t("jobs.detail.failureImpactValue.device")
  }
  if (category?.startsWith("environment_check_failed:")) {
    return t("jobs.detail.failureImpactValue.environment")
  }
  if (category?.startsWith("profile_check_failed:")) {
    return t("jobs.detail.failureImpactValue.profile")
  }
  if (
    category === "builder_runtime_patch_parse_failed" ||
    category === "workspace_patch_apply_failed" ||
    domain === "builder_runtime"
  ) {
    return t("jobs.detail.failureImpactValue.builderRuntime")
  }
  return t("jobs.detail.failureImpactValue.generic")
}