import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import type { ArtifactItem, PublicJobEvent, PublicJobRecord, PublicNotification } from "@/api/system"
import { JobsPage } from "@/components/jobs/jobs-page"

const {
  getAppConfigMock,
  getJobsMock,
  getJobMock,
  getJobEventsMock,
  getJobArtifactsMock,
  getNotificationsMock,
  getOrchestratorStatusMock,
  acknowledgeNotificationMock,
  compilePRDMock,
  prepareReviewMock,
  rebuildNotificationsMock,
  recordDeliveryMock,
  recordFollowUpMock,
  createJobMutationMock,
  registerBuilderMock,
  startJobMock,
  startOrchestratorWatchMock,
  stopOrchestratorWatchMock,
  submitPRDApprovalMock,
  toastSuccessMock,
  toastErrorMock,
  toastWarningMock,
  unlockOrchestratorWatchMock,
} = vi.hoisted(() => ({
  getAppConfigMock: vi.fn(),
  getJobsMock: vi.fn(),
  getJobMock: vi.fn(),
  getJobEventsMock: vi.fn(),
  getJobArtifactsMock: vi.fn(),
  getNotificationsMock: vi.fn(),
  getOrchestratorStatusMock: vi.fn(),
  acknowledgeNotificationMock: vi.fn(),
  compilePRDMock: vi.fn(),
  prepareReviewMock: vi.fn(),
  rebuildNotificationsMock: vi.fn(),
  recordDeliveryMock: vi.fn(),
  recordFollowUpMock: vi.fn(),
  createJobMutationMock: vi.fn(),
  registerBuilderMock: vi.fn(),
  startJobMock: vi.fn(),
  startOrchestratorWatchMock: vi.fn(),
  stopOrchestratorWatchMock: vi.fn(),
  submitPRDApprovalMock: vi.fn(),
  toastSuccessMock: vi.fn(),
  toastErrorMock: vi.fn(),
  toastWarningMock: vi.fn(),
  unlockOrchestratorWatchMock: vi.fn(),
}))

vi.mock("@/api/channels", () => ({
  getAppConfig: getAppConfigMock,
}))

vi.mock("@/api/system", async () => {
  const actual = await vi.importActual<typeof import("@/api/system")>("@/api/system")
  return {
    ...actual,
    acknowledgeNotification: acknowledgeNotificationMock,
    compilePrepareBundle: vi.fn(),
    compilePRD: compilePRDMock,
    createJob: createJobMutationMock,
    getJob: getJobMock,
    getJobArtifacts: getJobArtifactsMock,
    getJobEvents: getJobEventsMock,
    getJobs: getJobsMock,
    getNotifications: getNotificationsMock,
    getOrchestratorStatus: getOrchestratorStatusMock,
    prepareReview: prepareReviewMock,
    recordDelivery: recordDeliveryMock,
    recordDeliveryFollowUp: recordFollowUpMock,
    rebuildNotifications: rebuildNotificationsMock,
    registerBuilder: registerBuilderMock,
    startJob: startJobMock,
    startOrchestratorWatch: startOrchestratorWatchMock,
    stopOrchestratorWatch: stopOrchestratorWatchMock,
    submitPRDApproval: submitPRDApprovalMock,
    submitTemplateApproval: vi.fn(),
    unlockOrchestratorWatch: unlockOrchestratorWatchMock,
  }
})

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      if (options && typeof options.count === "number") {
        return `${key}:${String(options.count)}`
      }
      return key
    },
  }),
}))

vi.mock("sonner", () => ({
  toast: {
    success: toastSuccessMock,
    error: toastErrorMock,
    warning: toastWarningMock,
  },
}))

vi.mock("@/components/page-header", () => ({
  PageHeader: ({ title, children }: { title: string; children?: React.ReactNode }) => (
    <div>
      <div>{title}</div>
      <div>{children}</div>
    </div>
  ),
}))

function createJobRecord(overrides: Partial<PublicJobRecord> = {}): PublicJobRecord {
  return {
    schema_version: "v1",
    job_id: "job-001",
    title: "轻量记账 App",
    prd_id: "prd-001",
    prd_version: "1",
    template_id: "template-001",
    status: "failed",
    phase: "smoke",
    workspace_path: "workspace/jobs/job-001",
    artifact_dir: "workspace/jobs/job-001/artifacts",
    budgets: {
      iteration_budget: 10,
      token_budget: 1000,
    },
    human_approvals: [],
    created_at: "2026-04-03T10:00:00Z",
    updated_at: "2026-04-03T10:05:00Z",
    status_context: {
      summary: "Need manual follow-up",
      suggested_action: "inspect_execution_history",
      reason_code: "execution_failed",
    },
    failure_context: {
      retryable: false,
      failure_domain: "device",
      failure_category: "device_check_failed:launch",
      last_error_summary: "Build failed during smoke validation",
    },
    delivery_context: {
      delivery_record_path: "jobs/job-001/delivery.json",
      status: "released",
      summary: "Released to internal",
      suggested_action: "monitor_release_feedback",
      release_channel: "internal",
      rollout_percent: 100,
      reviewer_id: "reviewer-1",
      evidence_paths: ["jobs/job-001/artifacts/review-bundle.zip"],
      required_changes: [],
      signed_artifact_paths: ["jobs/job-001/artifacts/app-release.apk"],
      recorded_at: "2026-04-03T10:06:00Z",
      release_follow_up: {
        status: "monitoring",
        owner_id: "ops-1",
        summary: "Watching crash-free metrics",
        evidence_paths: ["jobs/job-001/artifacts/follow-up.txt"],
        updated_at: "2026-04-03T10:10:00Z",
        record_path: "jobs/job-001/follow-up.json",
        suggested_action: "monitor_release_feedback",
      },
    },
    artifacts: ["review-bundle", "app-release.apk"],
    ...overrides,
  }
}

function createEvent(overrides: Partial<PublicJobEvent> = {}): PublicJobEvent {
  return {
    at: "2026-04-03T10:04:00Z",
    type: "run_failed",
    job_id: "job-001",
    run_id: "run-001",
    summary: "Build failed during smoke validation",
    round_summaries: [
      {
        round_id: "round-1",
        attempt: 1,
        status: "failed",
        summary: "flutter analyze failed and produced a repair patch",
        current_phase: "validate",
        phase_trace: ["inspect", "edit", "validate"],
        target_paths: ["lib/main.dart", "pubspec.yaml"],
        modified_paths: ["lib/main.dart"],
        file_facts: [
          { path: "lib/main.dart", state: "modified" },
          { path: "pubspec.yaml", state: "targeted" },
        ],
        failed_checks: ["flutter analyze"],
        failure_signatures: ["analyze_failed"],
      },
    ],
    stage: "smoke",
    status: "failed",
    target_paths: ["lib/main.dart", "pubspec.yaml"],
    affected_paths: ["lib/main.dart"],
    file_facts: [
      { path: "lib/main.dart", state: "finalized", change_type: "modified" },
    ],
    failure_domain: "device",
    failure_category: "device_check_failed:launch",
    snapshot_path: "jobs/job-001/artifacts/workspace-snapshot.tgz",
    ...overrides,
  }
}

function createArtifact(overrides: Partial<ArtifactItem> = {}): ArtifactItem {
  return {
    artifact_id: "review-bundle",
    path: "jobs/job-001/artifacts/review-bundle.zip",
    artifact_type: "zip",
    produced: true,
    label: "Review Bundle",
    ...overrides,
  }
}

function createNotification(overrides: Partial<PublicNotification> = {}): PublicNotification {
  return {
    notification_id: "notif-001",
    type: "job_failed",
    job_id: "job-001",
    summary: "Please inspect execution history",
    suggested_action: "inspect_execution_history",
    created_at: "2026-04-03T10:05:00Z",
    acknowledged: false,
    ...overrides,
  }
}

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
      mutations: {
        retry: false,
      },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <JobsPage />
    </QueryClientProvider>,
  )
}

describe("JobsPage regressions", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    HTMLElement.prototype.scrollIntoView = vi.fn()

    const selectedJob = createJobRecord()
    const jobs = { items: [selectedJob] }
    const events = {
      items: [
        createEvent({
          at: "2026-04-03T10:02:00Z",
          type: "run_heartbeat",
          stage: "baseline",
          status: "running",
          summary: "baseline passed",
          round_summaries: undefined,
          round_id: "round-1",
          attempt: 1,
          current_phase: "validate",
          phase_trace: ["inspect", "edit", "validate"],
          target_paths: ["lib/main.dart", "pubspec.yaml"],
        }),
        createEvent(),
      ],
    }
    const artifacts = {
      items: [
        createArtifact(),
        createArtifact({
          artifact_id: "app-release.apk",
          path: "jobs/job-001/artifacts/app-release.apk",
          artifact_type: "apk",
          label: "Release APK",
        }),
      ],
    }
    const notifications = { items: [createNotification()] }

    getAppConfigMock.mockResolvedValue({})
    getJobsMock.mockResolvedValue(jobs)
    getJobMock.mockResolvedValue(selectedJob)
    getJobEventsMock.mockResolvedValue(events)
    getJobArtifactsMock.mockResolvedValue(artifacts)
    getNotificationsMock.mockResolvedValue(notifications)
    getOrchestratorStatusMock.mockResolvedValue({
      schema_version: "v1",
      instance_id: "orch-1",
      state: "idle",
      updated_at: "2026-04-03T10:05:00Z",
      watch_runner_state: "idle",
      watch_lock_state: "free",
    })
    acknowledgeNotificationMock.mockResolvedValue({ ack: true })
    compilePRDMock.mockResolvedValue({
      job_id: "job-002",
      prd_id: "prd-002",
      template_id: "template-002",
      bundle_dir: "bundles/job-002",
    })
    createJobMutationMock.mockResolvedValue(
      createJobRecord({
        job_id: "job-002",
        prd_id: "prd-002",
        template_id: "template-002",
        status: "running",
        phase: "orchestrator",
        failure_context: undefined,
        delivery_context: undefined,
        status_context: {
          summary: "Execution started",
          suggested_action: "inspect_execution_history",
        },
      }),
    )
    registerBuilderMock.mockResolvedValue({ ack: true })
    startJobMock.mockResolvedValue(
      createJobRecord({
        job_id: "job-002",
        prd_id: "prd-002",
        template_id: "template-002",
        status: "running",
        phase: "orchestrator",
        failure_context: undefined,
        delivery_context: undefined,
        status_context: {
          summary: "Execution started",
          suggested_action: "inspect_execution_history",
        },
      }),
    )
  })

  afterEach(() => {
    cleanup()
  })

  it("默认概览会展示失败诊断和待处理动作", async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    expect(screen.getByText("Please inspect execution history")).toBeInTheDocument()
    expect(screen.getAllByText("Need manual follow-up").length).toBeGreaterThan(0)
  })

  it("任务列表默认不再展示模板和产物元信息尾栏", async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByText("job-001")).toBeInTheDocument()
    })

    expect(screen.queryByText(/jobs\.list\.template/)).not.toBeInTheDocument()
    expect(screen.queryByText(/jobs\.list\.artifacts/)).not.toBeInTheDocument()
  })

  it("任务列表会继续压缩中间摘要层，不再展示当前状态小标题", async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByText("轻量记账 App")).toBeInTheDocument()
    })

    expect(screen.queryByText("jobs.list.currentActivity")).not.toBeInTheDocument()
  })

  it("任务列表会优先展示应用名称，并支持按应用名称搜索", async () => {
    const user = userEvent.setup()

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("轻量记账 App")).toBeInTheDocument()
      expect(screen.getByText("job-001")).toBeInTheDocument()
    })

    await user.clear(screen.getByPlaceholderText("jobs.list.searchPlaceholder"))
    await user.type(screen.getByPlaceholderText("jobs.list.searchPlaceholder"), "轻量记账")

    expect(screen.getByText("轻量记账 App")).toBeInTheDocument()
    expect(screen.queryByText("jobs.list.empty")).not.toBeInTheDocument()
  })

  it("任务列表会对失败态和待处理态显示不同视觉层级", async () => {
    const user = userEvent.setup()
    const failedJob = createJobRecord({
      job_id: "job-failed",
      status: "failed",
      failure_context: {
        retryable: false,
        failure_domain: "device",
        failure_category: "device_check_failed:launch",
        last_error_summary: "Failed job",
      },
    })
    const awaitingJob = createJobRecord({
      job_id: "job-awaiting",
      status: "awaiting_prd_approval",
      failure_context: undefined,
      human_approvals: ["approval-1"],
      status_context: {
        summary: "Waiting for approval",
        suggested_action: "submit_prd_approval",
        reason_code: "approval_required",
      },
    })

    getJobsMock.mockResolvedValueOnce({ items: [failedJob, awaitingJob] })
    getJobMock.mockResolvedValueOnce(failedJob)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("job-failed")).toBeInTheDocument()
      expect(screen.getByText("job-awaiting")).toBeInTheDocument()
    })

    const awaitingCard = screen.getByRole("button", { name: /job-awaiting/i })
    await user.click(awaitingCard)

    const failedCard = screen.getByRole("button", { name: /job-failed/i })

    expect(failedCard.className).toContain("border-rose-200")
    expect(screen.getByRole("button", { name: /job-awaiting/i }).className).toContain("border-primary/40")
  })

  it("待处理筛选会聚合失败态和待审批任务", async () => {
    const user = userEvent.setup()
    const failedJob = createJobRecord({
      job_id: "job-failed",
      status: "failed",
      failure_context: {
        retryable: false,
        failure_domain: "device",
        failure_category: "device_check_failed:launch",
        last_error_summary: "Failed job",
      },
    })
    const awaitingJob = createJobRecord({
      job_id: "job-awaiting",
      status: "awaiting_prd_approval",
      failure_context: undefined,
      human_approvals: ["approval-1"],
      status_context: {
        summary: "Waiting for approval",
        suggested_action: "submit_prd_approval",
        reason_code: "approval_required",
      },
    })
    const completedJob = createJobRecord({
      job_id: "job-completed",
      status: "completed",
      failure_context: undefined,
      human_approvals: [],
      status_context: undefined,
    })

    getJobsMock
      .mockResolvedValueOnce({ items: [failedJob, awaitingJob, completedJob] })
      .mockResolvedValueOnce({ items: [failedJob, awaitingJob, completedJob] })
    getJobMock.mockResolvedValueOnce(failedJob)

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("job-failed")).toBeInTheDocument()
      expect(screen.getByText("job-awaiting")).toBeInTheDocument()
      expect(screen.getByText("job-completed")).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: /jobs\.filters\.attention/i }))

    await waitFor(() => {
      expect(screen.getByText("job-failed")).toBeInTheDocument()
      expect(screen.getByText("job-awaiting")).toBeInTheDocument()
    })
    expect(screen.queryByText("job-completed")).not.toBeInTheDocument()
  })

  it("切到执行标签后会展示执行快照和最近事件", async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.execution" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.detail.executionSnapshot")).toBeInTheDocument()
    })
    expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    expect(screen.getByText("jobs.events.recentTitle")).toBeInTheDocument()
    expect(screen.getAllByText("jobs.events.snapshotPath").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs/job-001/artifacts/workspace-snapshot.tgz").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.roundContext").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.roundSummaries").length).toBeGreaterThan(0)
    expect(screen.getAllByText(/jobs\.events\.roundSummaryLine/).length).toBeGreaterThan(0)
    expect(screen.getAllByText("baseline passed").length).toBeGreaterThan(0)
    expect(screen.getAllByText(/flutter analyze failed and produced a repair patch/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/jobs\.events\.roundPhaseTrace/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/inspect -> edit -> validate/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/jobs\.events\.roundTargetPaths/).length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.targetPaths").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.affectedPaths").length).toBeGreaterThan(0)
    expect(screen.getAllByText("lib/main.dart").length).toBeGreaterThan(0)
  })

  it("运行中任务会在概览和执行概览展示当前轮次", async () => {
    const user = userEvent.setup()
    const runningJob = createJobRecord({
      status: "running_builder",
      phase: "builder",
      updated_at: "2026-04-03T10:03:00Z",
      failure_context: undefined,
      delivery_context: undefined,
      status_context: {
        summary: "Execution started",
        suggested_action: "inspect_execution_history",
      },
      current_round: {
        round_id: "round-1",
        attempt: 1,
        checkpoint_key: "check-flutter-pub-get",
        current_phase: "validate",
        phase_trace: ["inspect", "edit", "validate"],
        target_paths: ["lib/main.dart", "pubspec.yaml"],
      },
    })

    getJobsMock.mockResolvedValueOnce({ items: [runningJob] })
    getJobMock.mockResolvedValueOnce(runningJob)
    getNotificationsMock.mockResolvedValueOnce({ items: [] })
    getJobEventsMock.mockResolvedValueOnce({
      items: [
        createEvent({
          type: "run_heartbeat",
          stage: "baseline",
          status: "running",
          summary: "baseline passed",
          round_summaries: undefined,
          round_id: "round-1",
          attempt: 1,
          checkpoint_key: "check-flutter-pub-get",
          current_phase: "validate",
          phase_trace: ["inspect", "edit", "validate"],
          target_paths: ["lib/main.dart", "pubspec.yaml"],
          affected_paths: undefined,
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
      ],
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Execution started").length).toBeGreaterThan(0)
    })

    expect(screen.getAllByText("jobs.detail.currentRound").length).toBeGreaterThan(0)
    expect(screen.getAllByText(/jobs\.events\.roundSummaryLine/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/jobs\.detail\.currentRoundTargets/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/inspect -> edit -> validate/).length).toBeGreaterThan(0)

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.execution" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.detail.executionSnapshot")).toBeInTheDocument()
    })
    expect(screen.getAllByText("jobs.detail.currentRound").length).toBeGreaterThan(0)
    expect(screen.getAllByText(/jobs\.detail\.currentRoundTargets/).length).toBeGreaterThan(0)
  })

  it("执行时间线会把轮次开始和补丁落地事件渲染成可读语义", async () => {
    const user = userEvent.setup()
    getJobEventsMock.mockResolvedValueOnce({
      items: [
        createEvent({
          at: "2026-04-03T10:01:00Z",
          type: "run_round_started",
          summary: undefined,
          status: "running",
          round_summaries: undefined,
          round_id: "round-2",
          attempt: 2,
          checkpoint_key: "check-flutter-analyze",
          current_phase: "edit",
          phase_trace: ["inspect", "edit"],
          target_paths: ["lib/main.dart"],
          file_facts: [{ path: "lib/main.dart", state: "targeted" }],
          affected_paths: undefined,
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
        createEvent({
          at: "2026-04-03T10:01:10Z",
          type: "run_patch_generation_started",
          summary: undefined,
          status: "running",
          round_summaries: undefined,
          round_id: "round-2",
          attempt: 2,
          checkpoint_key: "thin-prepare",
          current_phase: "edit",
          phase_trace: ["inspect", "edit"],
          target_paths: ["lib/main.dart", "pubspec.yaml"],
          file_facts: [
            { path: "lib/main.dart", state: "generation_started" },
            { path: "pubspec.yaml", state: "generation_started" },
          ],
          affected_paths: undefined,
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
        createEvent({
          at: "2026-04-03T10:01:30Z",
          type: "run_patch_generated",
          summary: undefined,
          status: "running",
          round_summaries: undefined,
          round_id: "round-2",
          attempt: 2,
          checkpoint_key: "thin-prepare",
          current_phase: "edit",
          phase_trace: ["inspect", "edit"],
          target_paths: ["lib/main.dart", "pubspec.yaml"],
          file_facts: [
            { path: "lib/main.dart", state: "generated" },
            { path: "pubspec.yaml", state: "generated" },
          ],
          affected_paths: undefined,
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
        createEvent({
          at: "2026-04-03T10:01:45Z",
          type: "run_patch_generation_failed",
          summary: undefined,
          status: "failed",
          round_summaries: undefined,
          round_id: "round-2",
          attempt: 2,
          checkpoint_key: "thin-prepare",
          current_phase: "edit",
          phase_trace: ["inspect", "edit"],
          target_paths: ["pubspec.yaml"],
          file_facts: [{ path: "pubspec.yaml", state: "generation_failed" }],
          affected_paths: undefined,
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
        createEvent({
          at: "2026-04-03T10:02:00Z",
          type: "run_patch_applied",
          summary: undefined,
          status: "completed",
          round_summaries: undefined,
          round_id: "round-2",
          attempt: 2,
          checkpoint_key: undefined,
          current_phase: "finalize",
          phase_trace: ["inspect", "edit", "finalize"],
          target_paths: ["lib/main.dart"],
          affected_paths: ["lib/main.dart", "pubspec.yaml"],
          file_facts: [
            { path: "lib/main.dart", state: "applied" },
            { path: "pubspec.yaml", state: "applied" },
          ],
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
        createEvent({
          at: "2026-04-03T10:02:20Z",
          type: "run_patch_apply_failed",
          summary: undefined,
          status: "failed",
          round_summaries: undefined,
          round_id: "round-2",
          attempt: 2,
          checkpoint_key: "thin-prepare",
          current_phase: "finalize",
          phase_trace: ["inspect", "edit", "finalize"],
          target_paths: ["pubspec.yaml"],
          file_facts: [{ path: "pubspec.yaml", state: "apply_failed" }],
          affected_paths: undefined,
          snapshot_path: undefined,
          failure_domain: undefined,
          failure_category: undefined,
        }),
      ],
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Need manual follow-up").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.execution" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.events.recentTitle")).toBeInTheDocument()
    })
    expect(screen.getByText("jobs.events.roundActivityTitle")).toBeInTheDocument()
    expect(screen.getByText("jobs.events.roundActivityDetails")).toBeInTheDocument()
    expect(screen.getAllByText("run_round_started").length).toBeGreaterThan(0)
    expect(screen.getAllByText("run_patch_generation_started").length).toBeGreaterThan(0)
    expect(screen.getAllByText("run_patch_generated").length).toBeGreaterThan(0)
    expect(screen.getAllByText("run_patch_generation_failed").length).toBeGreaterThan(0)
    expect(screen.getAllByText("run_patch_applied").length).toBeGreaterThan(0)
    expect(screen.getAllByText("run_patch_apply_failed").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.checkpointKey").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.fileFacts").length).toBeGreaterThan(0)
    expect(screen.getAllByText("check-flutter-analyze").length).toBeGreaterThan(0)
    expect(screen.getAllByText("jobs.events.affectedPaths").length).toBeGreaterThan(0)
    expect(screen.getAllByText(/lib\/main\.dart · targeted|lib\/main\.dart · Targeted/).length).toBeGreaterThan(0)
    expect(screen.getAllByText(/pubspec\.yaml · apply_failed|pubspec\.yaml · Apply failed/).length).toBeGreaterThan(0)
    expect(screen.getAllByText("pubspec.yaml").length).toBeGreaterThan(0)
  })

  it("执行事件会展示交付记录和恢复建议信息", async () => {
    const user = userEvent.setup()
    getJobEventsMock.mockResolvedValueOnce({
      items: [
        createEvent({
          type: "resume_ready",
          summary: "failed job can be resumed",
          delivery_record_path: "jobs/job-001/delivery.json",
          recommended_resume_mode: "resume_from_failure",
        }),
      ],
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.execution" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.events.deliveryRecordPath")).toBeInTheDocument()
    })
    expect(screen.getByText("jobs/job-001/delivery.json")).toBeInTheDocument()
    expect(screen.getByText("jobs.events.recommendedResumeMode")).toBeInTheDocument()
    expect(screen.getByText("resume_from_failure")).toBeInTheDocument()
  })

  it("切到交付标签后会展示交付上下文并允许展开 follow-up 表单", async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))

    await waitFor(() => {
      expect(screen.getByText("Released to internal")).toBeInTheDocument()
    })

    expect(screen.getByText("reviewer-1")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "jobs.delivery.recordFollowUp" }))
    expect(screen.getByDisplayValue("ops-1")).toBeInTheDocument()
    expect(screen.getByDisplayValue("Watching crash-free metrics")).toBeInTheDocument()
  })

  it("切到产物标签后会展示最终交付物和产物行", async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.artifacts" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.artifacts.title")).toBeInTheDocument()
    })

    expect(screen.getAllByText("Release APK").length).toBeGreaterThan(0)
    expect(screen.getAllByText("Review Bundle").length).toBeGreaterThan(0)
  })

  it("创建任务在编译失败时会展示错误并停止后续流程", async () => {
    const user = userEvent.setup()
    compilePRDMock.mockRejectedValueOnce(new Error("compile failed"))

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.actions.createJob" }))
    await waitFor(() => {
      expect(screen.getByText("jobs.create.title")).toBeInTheDocument()
    })

    await user.type(screen.getByPlaceholderText("jobs.create.placeholders.title"), "Android Demo")
    await user.type(
      screen.getByPlaceholderText("jobs.create.placeholders.requirementText"),
      "Build an Android release candidate",
    )

    const submitButtons = screen.getAllByRole("button", { name: "jobs.actions.createJob" })
    await user.click(submitButtons[submitButtons.length - 1])

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("compile failed")
    })
    expect(createJobMutationMock).not.toHaveBeenCalled()
    expect(registerBuilderMock).not.toHaveBeenCalled()
    expect(startJobMock).not.toHaveBeenCalled()
  })

  it("创建任务在启动失败时会保留已创建任务并弹出警告", async () => {
    const user = userEvent.setup()
    startJobMock.mockRejectedValueOnce(new Error("builder unavailable"))

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.actions.createJob" }))
    await waitFor(() => {
      expect(screen.getByText("jobs.create.title")).toBeInTheDocument()
    })

    await user.type(screen.getByPlaceholderText("jobs.create.placeholders.title"), "Android Demo")
    await user.type(
      screen.getByPlaceholderText("jobs.create.placeholders.requirementText"),
      "Build an Android release candidate",
    )

    const submitButtons = screen.getAllByRole("button", { name: "jobs.actions.createJob" })
    await user.click(submitButtons[submitButtons.length - 1])

    await waitFor(() => {
      expect(toastWarningMock).toHaveBeenCalledWith("jobs.toasts.jobCreatedButStartFailed")
    })
    expect(createJobMutationMock).toHaveBeenCalled()
    expect(registerBuilderMock).toHaveBeenCalled()
    expect(startJobMock).toHaveBeenCalled()
  })

  it("创建弹窗会把高级选项默认折叠，并允许关闭真实检查和自动启动", async () => {
    const user = userEvent.setup()

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.actions.createJob" }))
    await waitFor(() => {
      expect(screen.getByText("jobs.create.title")).toBeInTheDocument()
    })

    expect(screen.queryByRole("switch", { name: "jobs.create.advanced.realChecks.label" })).not.toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: /jobs\.create\.advanced\.trigger/i }))

    const realChecksSwitch = await screen.findByRole("switch", {
      name: "jobs.create.advanced.realChecks.label",
    })
    const autoStartBuilderSwitch = screen.getByRole("switch", {
      name: "jobs.create.advanced.autoStartBuilder.label",
    })

    expect(realChecksSwitch).toHaveAttribute("data-state", "checked")
    expect(autoStartBuilderSwitch).toHaveAttribute("data-state", "checked")

    await user.click(realChecksSwitch)
    await user.click(autoStartBuilderSwitch)
    await user.type(screen.getByPlaceholderText("jobs.create.placeholders.title"), "Android Demo")
    await user.type(
      screen.getByPlaceholderText("jobs.create.placeholders.requirementText"),
      "Build an Android release candidate",
    )

    const submitButtons = screen.getAllByRole("button", { name: "jobs.actions.createJob" })
    await user.click(submitButtons[submitButtons.length - 1])

    await waitFor(() => {
      expect(compilePRDMock).toHaveBeenCalledWith(
        expect.objectContaining({
          real_checks: false,
        }),
      )
    })
    expect(createJobMutationMock).toHaveBeenCalled()
    expect(registerBuilderMock).not.toHaveBeenCalled()
    expect(startJobMock).not.toHaveBeenCalled()
    expect(toastSuccessMock).toHaveBeenCalledWith("jobs.toasts.jobCreated")
  })

  it("确认通知后会调用 acknowledge 并反馈成功提示", async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "jobs.actions.acknowledge" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "jobs.actions.acknowledge" }))

    await waitFor(() => {
      expect(acknowledgeNotificationMock).toHaveBeenCalled()
    })
    expect(acknowledgeNotificationMock.mock.calls[0]?.[0]).toBe("notif-001")
    expect(toastSuccessMock).toHaveBeenCalledWith("jobs.toasts.notificationAcked")
  })

  it("重建通知失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    rebuildNotificationsMock.mockRejectedValueOnce(new Error("rebuild failed"))

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.actions.rebuildNotifications" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("rebuild failed")
    })
    expect(rebuildNotificationsMock).toHaveBeenCalled()
  })

  it("重建通知成功时会展示成功提示", async () => {
    const user = userEvent.setup()
    rebuildNotificationsMock.mockResolvedValueOnce({ ack: true })

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.actions.rebuildNotifications" }))

    await waitFor(() => {
      expect(toastSuccessMock).toHaveBeenCalledWith("jobs.toasts.notificationsRebuilt")
    })
    expect(rebuildNotificationsMock).toHaveBeenCalled()
  })

  it("概览建议动作执行失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    const actionableJob = createJobRecord({
      status: "awaiting_prd_approval",
      status_context: {
        summary: "Need PRD approval",
        suggested_action: "submit_prd_approval",
        reason_code: "approval_required",
      },
      failure_context: undefined,
      delivery_context: undefined,
    })
    getJobsMock.mockResolvedValueOnce({ items: [actionableJob] })
    getJobMock.mockResolvedValueOnce(actionableJob)
    getNotificationsMock.mockResolvedValueOnce({ items: [] })
    submitPRDApprovalMock.mockRejectedValueOnce(new Error("approval failed"))

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Need PRD approval").length).toBeGreaterThan(0)
    })

    const actionButtons = screen.getAllByRole("button", { name: "jobs.detail.runSuggestedAction" })
    await user.click(actionButtons[0])

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("approval failed")
    })
    expect(submitPRDApprovalMock).toHaveBeenCalledWith("prd-001", "job-001")
  })

  it("执行标签在没有事件时会展示空态", async () => {
    const user = userEvent.setup()
    getJobEventsMock.mockResolvedValueOnce({ items: [] })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.execution" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.events.empty")).toBeInTheDocument()
    })
  })

  it("通知建议动作会切到执行标签并聚焦事件区", async () => {
    const user = userEvent.setup()

    renderPage()

    await waitFor(() => {
      expect(screen.getByText("Please inspect execution history")).toBeInTheDocument()
    })

    const actionButtons = screen.getAllByRole("button", { name: "jobs.detail.runSuggestedAction" })
    await user.click(actionButtons[0])

    await waitFor(() => {
      expect(screen.getByText("jobs.detail.executionSnapshot")).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(document.activeElement?.id).toBe("jobs-events")
    })
  })

  it("failure guidance 的 inspectEvents 会切到执行标签并聚焦事件区", async () => {
    const user = userEvent.setup()
    const builderRuntimeFailureJob = createJobRecord({
      failure_context: {
        retryable: false,
        failure_domain: "builder_runtime",
        failure_category: "builder_runtime_patch_parse_failed",
        failure_signature: "builder_runtime_patch_parse_failed",
        last_error_summary: "Patch parse failed before apply",
      },
      status_context: undefined,
    })
    getJobsMock.mockResolvedValueOnce({ items: [builderRuntimeFailureJob] })
    getJobMock.mockResolvedValueOnce(builderRuntimeFailureJob)
    getNotificationsMock.mockResolvedValueOnce({ items: [] })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Patch parse failed before apply").length).toBeGreaterThan(0)
    })

    const inspectEventButtons = screen.getAllByRole("button", { name: "inspectEvents" })
    expect(inspectEventButtons.length).toBeGreaterThan(0)
    await user.click(inspectEventButtons[0])

    await waitFor(() => {
      expect(screen.getByText("jobs.detail.executionSnapshot")).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(document.activeElement?.id).toBe("jobs-events")
    })
  })

  it("failure guidance 的 inspectArtifacts 会切到产物标签并聚焦产物区", async () => {
    const user = userEvent.setup()
    const builderRuntimeFailureJob = createJobRecord({
      failure_context: {
        retryable: false,
        failure_domain: "builder_runtime",
        failure_category: "builder_runtime_patch_parse_failed",
        failure_signature: "builder_runtime_patch_parse_failed",
        last_error_summary: "Patch parse failed before apply",
      },
      status_context: undefined,
    })
    getJobsMock.mockResolvedValueOnce({ items: [builderRuntimeFailureJob] })
    getJobMock.mockResolvedValueOnce(builderRuntimeFailureJob)
    getNotificationsMock.mockResolvedValueOnce({ items: [] })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Patch parse failed before apply").length).toBeGreaterThan(0)
    })

    const inspectArtifactButtons = screen.getAllByRole("button", { name: "inspectArtifacts" })
    expect(inspectArtifactButtons.length).toBeGreaterThan(0)
    await user.click(inspectArtifactButtons[0])

    await waitFor(() => {
      expect(screen.getByText("jobs.artifacts.title")).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(document.activeElement?.id).toBe("jobs-artifacts")
    })
  })

  it("交付建议动作会聚焦 follow-up 区块", async () => {
    const user = userEvent.setup()

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))
    await waitFor(() => {
      expect(screen.getByText("Released to internal")).toBeInTheDocument()
    })

    const actionButtons = screen.getAllByRole("button", { name: "jobs.detail.runSuggestedAction" })
    await user.click(actionButtons[actionButtons.length - 1])

    await waitFor(() => {
      expect(document.activeElement?.id).toBe("jobs-delivery-follow-up")
    })
  })

  it("准备 review 成功后会切到产物标签", async () => {
    const user = userEvent.setup()

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "jobs.delivery.prepareReview" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "jobs.delivery.prepareReview" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.artifacts.title")).toBeInTheDocument()
    })
    await waitFor(() => {
      expect(document.activeElement?.id).toBe("jobs-artifacts")
    })
  })

  it("编排器启动失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    startOrchestratorWatchMock.mockRejectedValueOnce(new Error("watch start failed"))

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.workspace.showSystemStatus" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "header.orchestrator.action.startWatch" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "header.orchestrator.action.startWatch" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("watch start failed")
    })
    expect(startOrchestratorWatchMock).toHaveBeenCalledWith(15)
  })

  it("编排器启动成功时会展示成功提示", async () => {
    const user = userEvent.setup()
    startOrchestratorWatchMock.mockResolvedValueOnce({ ack: true })

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.workspace.showSystemStatus" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "header.orchestrator.action.startWatch" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "header.orchestrator.action.startWatch" }))

    await waitFor(() => {
      expect(toastSuccessMock).toHaveBeenCalledWith("header.orchestrator.toast.started")
    })
    expect(startOrchestratorWatchMock).toHaveBeenCalledWith(15)
  })

  it("编排器停止失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    getOrchestratorStatusMock.mockResolvedValueOnce({
      schema_version: "v1",
      instance_id: "orch-1",
      state: "running",
      updated_at: "2026-04-03T10:05:00Z",
      watch_runner_state: "running",
      watch_lock_state: "free",
    })
    stopOrchestratorWatchMock.mockRejectedValueOnce(new Error("watch stop failed"))

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.workspace.showSystemStatus" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "header.orchestrator.action.stopWatch" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "header.orchestrator.action.stopWatch" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("watch stop failed")
    })
    expect(stopOrchestratorWatchMock).toHaveBeenCalled()
  })

  it("编排器停止成功时会展示成功提示", async () => {
    const user = userEvent.setup()
    getOrchestratorStatusMock.mockResolvedValueOnce({
      schema_version: "v1",
      instance_id: "orch-1",
      state: "running",
      updated_at: "2026-04-03T10:05:00Z",
      watch_runner_state: "running",
      watch_lock_state: "free",
    })
    stopOrchestratorWatchMock.mockResolvedValueOnce({ ack: true })

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.workspace.showSystemStatus" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "header.orchestrator.action.stopWatch" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "header.orchestrator.action.stopWatch" }))

    await waitFor(() => {
      expect(toastSuccessMock).toHaveBeenCalledWith("header.orchestrator.toast.stopped")
    })
    expect(stopOrchestratorWatchMock).toHaveBeenCalled()
  })

  it("编排器强制解锁失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    getOrchestratorStatusMock.mockResolvedValueOnce({
      schema_version: "v1",
      instance_id: "orch-1",
      state: "idle",
      updated_at: "2026-04-03T10:05:00Z",
      watch_runner_state: "idle",
      watch_lock_state: "stale",
      watch_lock_owner_id: "worker-2",
    })
    unlockOrchestratorWatchMock.mockRejectedValueOnce(new Error("unlock failed"))

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.workspace.showSystemStatus" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "header.orchestrator.action.forceUnlock" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "header.orchestrator.action.forceUnlock" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("unlock failed")
    })
    expect(unlockOrchestratorWatchMock).toHaveBeenCalledWith(true)
  })

  it("编排器强制解锁成功时会展示成功提示", async () => {
    const user = userEvent.setup()
    getOrchestratorStatusMock.mockResolvedValueOnce({
      schema_version: "v1",
      instance_id: "orch-1",
      state: "idle",
      updated_at: "2026-04-03T10:05:00Z",
      watch_runner_state: "idle",
      watch_lock_state: "stale",
      watch_lock_owner_id: "worker-2",
    })
    unlockOrchestratorWatchMock.mockResolvedValueOnce({ ack: true })

    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.workspace.showSystemStatus" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "header.orchestrator.action.forceUnlock" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "header.orchestrator.action.forceUnlock" }))

    await waitFor(() => {
      expect(toastSuccessMock).toHaveBeenCalledWith("header.orchestrator.toast.unlocked")
    })
    expect(unlockOrchestratorWatchMock).toHaveBeenCalledWith(true)
  })

  it("产物标签在没有产物时会展示空态", async () => {
    const user = userEvent.setup()
    getJobArtifactsMock.mockResolvedValueOnce({ items: [] })

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.artifacts" }))

    await waitFor(() => {
      expect(screen.getByText("jobs.artifacts.empty")).toBeInTheDocument()
    })
  })

  it("交付表单缺少 reviewer 时会先拦截校验而不提交", async () => {
    const user = userEvent.setup()
    const jobWithoutDelivery = createJobRecord({
      delivery_context: undefined,
    })
    getJobsMock.mockResolvedValueOnce({ items: [jobWithoutDelivery] })
    getJobMock.mockResolvedValueOnce(jobWithoutDelivery)

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "jobs.delivery.recordDecision" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "jobs.delivery.recordDecision" }))
    await user.click(screen.getByRole("button", { name: "jobs.delivery.submitRecord" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("jobs.delivery.validation.reviewerId")
    })
    expect(recordDeliveryMock).not.toHaveBeenCalled()
  })

  it("准备 review 失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    prepareReviewMock.mockRejectedValueOnce(new Error("review failed"))

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "jobs.delivery.prepareReview" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "jobs.delivery.prepareReview" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("review failed")
    })
    expect(prepareReviewMock).toHaveBeenCalledWith("run-001")
  })

  it("交付记录提交失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    const deliveryReadyJob = createJobRecord({
      delivery_context: {
        delivery_record_path: "jobs/job-001/delivery.json",
        status: "released",
        summary: "Released to internal",
        suggested_action: "monitor_release_feedback",
        release_channel: "internal",
        rollout_percent: 100,
        reviewer_id: "reviewer-1",
        evidence_paths: ["jobs/job-001/artifacts/review-bundle.zip"],
        required_changes: [],
        signed_artifact_paths: ["jobs/job-001/artifacts/app-release.apk"],
        recorded_at: "2026-04-03T10:06:00Z",
        device_verification: {
          status: "passed",
          summary: "Verified on device",
          evidence_paths: ["jobs/job-001/artifacts/device-logcat.txt"],
        },
        release_follow_up: {
          status: "monitoring",
          owner_id: "ops-1",
          summary: "Watching crash-free metrics",
          evidence_paths: ["jobs/job-001/artifacts/follow-up.txt"],
          updated_at: "2026-04-03T10:10:00Z",
          record_path: "jobs/job-001/follow-up.json",
          suggested_action: "monitor_release_feedback",
        },
      },
    })
    getJobsMock.mockResolvedValueOnce({ items: [deliveryReadyJob] })
    getJobMock.mockResolvedValueOnce(deliveryReadyJob)
    recordDeliveryMock.mockRejectedValueOnce(new Error("delivery failed"))

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "jobs.delivery.recordDecision" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "jobs.delivery.recordDecision" }))
    await user.click(screen.getByRole("button", { name: "jobs.delivery.submitRecord" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("delivery failed")
    })
    expect(recordDeliveryMock).toHaveBeenCalled()
  })

  it("follow-up 提交失败时会展示错误提示", async () => {
    const user = userEvent.setup()
    recordFollowUpMock.mockRejectedValueOnce(new Error("follow-up failed"))

    renderPage()

    await waitFor(() => {
      expect(screen.getAllByText("Build failed during smoke validation").length).toBeGreaterThan(0)
    })

    await user.click(screen.getByRole("button", { name: "jobs.workspace.tabs.delivery" }))
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "jobs.delivery.recordFollowUp" })).toBeInTheDocument()
    })

    await user.click(screen.getByRole("button", { name: "jobs.delivery.recordFollowUp" }))
    await user.click(screen.getByRole("button", { name: "jobs.delivery.submitFollowUp" }))

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("follow-up failed")
    })
    expect(recordFollowUpMock).toHaveBeenCalled()
  })

  it("创建任务时会先做必填校验，再能成功提交", async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole("button", { name: "jobs.actions.createJob" }))
    await waitFor(() => {
      expect(screen.getByText("jobs.create.title")).toBeInTheDocument()
    })

    const submitButtons = screen.getAllByRole("button", { name: "jobs.actions.createJob" })
    await user.click(submitButtons[submitButtons.length - 1])
    expect(toastErrorMock).toHaveBeenCalledWith("jobs.create.validation.title")

    await user.type(screen.getByPlaceholderText("jobs.create.placeholders.title"), "Android Demo")
    await user.click(submitButtons[submitButtons.length - 1])
    expect(toastErrorMock).toHaveBeenCalledWith("jobs.create.validation.requirementText")

    await user.type(
      screen.getByPlaceholderText("jobs.create.placeholders.requirementText"),
      "Build an Android release candidate",
    )
    await user.click(submitButtons[submitButtons.length - 1])

    await waitFor(() => {
      expect(compilePRDMock).toHaveBeenCalled()
    })
    expect(createJobMutationMock).toHaveBeenCalled()
    expect(registerBuilderMock).toHaveBeenCalled()
    expect(startJobMock).toHaveBeenCalled()
    expect(toastSuccessMock).toHaveBeenCalledWith("jobs.toasts.jobCreatedAndStarted")
  })
})