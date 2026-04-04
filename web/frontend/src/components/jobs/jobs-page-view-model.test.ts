import { describe, expect, it } from "vitest"

import type { ArtifactItem, PublicJobEvent, PublicJobRecord, PublicNotification } from "@/api/system"
import { buildJobsPageViewModel } from "@/components/jobs/jobs-page-view-model"

function createJob(overrides: Partial<PublicJobRecord> = {}): PublicJobRecord {
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
    created_at: "2026-04-04T00:00:00Z",
    updated_at: "2026-04-04T00:05:00Z",
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
      recorded_at: "2026-04-04T00:06:00Z",
    },
    ...overrides,
  }
}

function createEvent(overrides: Partial<PublicJobEvent> = {}): PublicJobEvent {
  return {
    at: "2026-04-04T00:04:00Z",
    type: "run_failed",
    job_id: "job-001",
    run_id: "run-001",
    summary: "Build failed during smoke validation",
    round_summaries: [
      {
        round_id: "round-1",
        status: "failed",
        summary: "flutter analyze failed and produced a repair patch",
        current_phase: "validate",
        modified_paths: ["lib/main.dart"],
        failed_checks: ["flutter analyze"],
        failure_signatures: ["analyze_failed"],
      },
    ],
    stage: "smoke",
    status: "failed",
    failure_domain: "device",
    failure_category: "device_check_failed:launch",
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

function createNotification(index: number, overrides: Partial<PublicNotification> = {}): PublicNotification {
  return {
    notification_id: `notif-00${index}`,
    type: "job_failed",
    job_id: "job-001",
    summary: `Need action ${index}`,
    suggested_action: "inspect_execution_history",
    created_at: `2026-04-04T00:0${index}:00Z`,
    acknowledged: false,
    ...overrides,
  }
}

describe("buildJobsPageViewModel", () => {
  it("会收敛任务列表、通知摘要和详情视图数据", () => {
    const selectedJob = createJob()
    const result = buildJobsPageViewModel({
      allJobs: [selectedJob],
      jobItems: [selectedJob],
      notifications: [
        createNotification(1),
        createNotification(2),
        createNotification(3),
        createNotification(4),
      ],
      selectedJob,
      effectiveSelectedJobId: selectedJob.job_id,
      jobEvents: [
        createEvent({
          at: "2026-04-04T00:02:00Z",
          type: "execution_started",
          stage: "orchestrator",
          status: "running",
          summary: "Execution started",
        }),
        createEvent(),
      ],
      artifactItems: [
        createArtifact(),
        createArtifact({
          artifact_id: "app-release.apk",
          path: "jobs/job-001/artifacts/app-release.apk",
          artifact_type: "apk",
          label: "Release APK",
        }),
      ],
      statusFilter: "all",
      jobSearchQuery: "job-001",
      deliveryForm: {
        status: "released",
        reviewerId: "reviewer-1",
        summary: "Released to internal",
        releaseChannel: "internal",
        rolloutPercent: "100",
        evidencePaths: "jobs/job-001/artifacts/review-bundle.zip",
        requiredChanges: "",
        signedArtifactPaths: "jobs/job-001/artifacts/app-release.apk",
        deviceVerificationStatus: "__none__",
        deviceVerificationSummary: "",
        deviceVerificationEvidencePaths: "",
      },
      t: (key) => key,
    })

    expect(result.jobListItems).toHaveLength(1)
    expect(result.jobListItems[0]?.job.job_id).toBe("job-001")
    expect(result.jobListItems[0]?.displayTitle).toBe("轻量记账 App")
    expect(result.latestRunId).toBe("run-001")
    expect(result.selectedJobNotifications).toHaveLength(3)
    expect(result.selectedJobActionCount).toBeGreaterThan(1)
    expect(result.selectedJobExecutionStage?.stageLabel).toBe("smoke")
    expect(result.selectedJobExecutionStage?.stepLabel).toBe("flutter analyze")
    expect(result.selectedJobExecutionStage?.detail).toBe("flutter analyze failed and produced a repair patch")
    const failedCheckpoint = result.selectedJobExecutionBreakdown.find((item) => item.key === "check-flutter-analyze")
    expect(failedCheckpoint?.state).toBe("failed")
    expect(failedCheckpoint?.stepLabel).toBe("flutter analyze")
    expect(failedCheckpoint?.detail).toBe("flutter analyze failed and produced a repair patch")
    expect(result.selectedJobExecutionBreakdown.some((item) => item.key === "delivery-failed")).toBe(false)
    expect(result.failureGuidance.length).toBeGreaterThan(0)
    expect(result.deliveryChecklistItems.length).toBeGreaterThan(0)
    expect(result.workspaceFocus.headline).toBeTruthy()
  })

  it("在缺少 failed_checks 时，也会根据 round phase 和 stage 回填失败 checkpoint", () => {
    const selectedJob = createJob()
    const result = buildJobsPageViewModel({
      allJobs: [selectedJob],
      jobItems: [selectedJob],
      notifications: [],
      selectedJob,
      effectiveSelectedJobId: selectedJob.job_id,
      jobEvents: [
        createEvent({
          summary: "Builder runtime failed during validation",
          stage: "cheap",
          round_summaries: [
            {
              round_id: "round-2",
              status: "failed",
              summary: "validation phase stopped before a concrete failed check was emitted",
              current_phase: "validate",
              modified_paths: ["lib/main.dart"],
              failed_checks: [],
              failure_signatures: ["validation_failed"],
            },
          ],
        }),
      ],
      artifactItems: [createArtifact()],
      statusFilter: "all",
      jobSearchQuery: "",
      deliveryForm: {
        status: "released",
        reviewerId: "reviewer-1",
        summary: "Released to internal",
        releaseChannel: "internal",
        rolloutPercent: "100",
        evidencePaths: "jobs/job-001/artifacts/review-bundle.zip",
        requiredChanges: "",
        signedArtifactPaths: "jobs/job-001/artifacts/app-release.apk",
        deviceVerificationStatus: "__none__",
        deviceVerificationSummary: "",
        deviceVerificationEvidencePaths: "",
      },
      t: (key) => key,
    })

    const failedCheckpoint = result.selectedJobExecutionBreakdown.find((item) => item.key === "check-flutter-test")
    expect(failedCheckpoint?.state).toBe("failed")
    expect(failedCheckpoint?.stepLabel).toBe("check-flutter-test")
    expect(failedCheckpoint?.detail).toBe("validation phase stopped before a concrete failed check was emitted")
    expect(result.selectedJobExecutionBreakdown.some((item) => item.key === "delivery-failed")).toBe(false)
  })

  it("运行中的 current_round 会驱动当前步骤与 running checkpoint", () => {
    const selectedJob = createJob({
      status: "running_builder",
      phase: "builder",
      failure_context: undefined,
      delivery_context: undefined,
      current_round: {
        round_id: "round-1",
        attempt: 1,
        checkpoint_key: "check-flutter-pub-get",
        current_phase: "validate",
        phase_trace: ["inspect", "edit", "validate"],
        target_paths: ["lib/main.dart", "pubspec.yaml"],
      },
      status_context: {
        summary: "Execution started",
        suggested_action: "inspect_execution_history",
      },
    })

    const result = buildJobsPageViewModel({
      allJobs: [selectedJob],
      jobItems: [selectedJob],
      notifications: [],
      selectedJob,
      effectiveSelectedJobId: selectedJob.job_id,
      jobEvents: [
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
          failure_domain: undefined,
          failure_category: undefined,
        }),
      ],
      artifactItems: [createArtifact()],
      statusFilter: "all",
      jobSearchQuery: "",
      deliveryForm: {
        status: "released",
        reviewerId: "reviewer-1",
        summary: "Released to internal",
        releaseChannel: "internal",
        rolloutPercent: "100",
        evidencePaths: "jobs/job-001/artifacts/review-bundle.zip",
        requiredChanges: "",
        signedArtifactPaths: "jobs/job-001/artifacts/app-release.apk",
        deviceVerificationStatus: "__none__",
        deviceVerificationSummary: "",
        deviceVerificationEvidencePaths: "",
      },
      t: (key) => key,
    })

    expect(result.selectedJobExecutionStage?.stageLabel).toBe("baseline")
    expect(result.selectedJobExecutionStage?.stepLabel).toBe("check-flutter-pub-get")
    expect(result.selectedJobExecutionStage?.detail).toBe("baseline passed")
    const runningCheckpoint = result.selectedJobExecutionBreakdown.find((item) => item.key === "check-flutter-pub-get")
    expect(runningCheckpoint?.state).toBe("running")
  })

  it("无事件时也会尽量用 current_round 回填列表摘要与运行中的 checkpoint", () => {
    const selectedJob = createJob({
      status: "running_builder",
      phase: "builder",
      failure_context: undefined,
      delivery_context: undefined,
      current_round: {
        round_id: "round-1",
        attempt: 1,
        checkpoint_key: "check-flutter-analyze",
        current_phase: "validate",
        phase_trace: ["inspect", "edit", "validate"],
        target_paths: ["lib/main.dart", "pubspec.yaml"],
      },
      status_context: {
        summary: "Execution started",
        suggested_action: "inspect_execution_history",
      },
    })

    const result = buildJobsPageViewModel({
      allJobs: [selectedJob],
      jobItems: [selectedJob],
      notifications: [],
      selectedJob,
      effectiveSelectedJobId: selectedJob.job_id,
      jobEvents: [],
      artifactItems: [createArtifact()],
      statusFilter: "all",
      jobSearchQuery: "",
      deliveryForm: {
        status: "released",
        reviewerId: "reviewer-1",
        summary: "Released to internal",
        releaseChannel: "internal",
        rolloutPercent: "100",
        evidencePaths: "jobs/job-001/artifacts/review-bundle.zip",
        requiredChanges: "",
        signedArtifactPaths: "jobs/job-001/artifacts/app-release.apk",
        deviceVerificationStatus: "__none__",
        deviceVerificationSummary: "",
        deviceVerificationEvidencePaths: "",
      },
      t: (key) => key,
    })

    expect(result.jobListItems[0]?.headline).toBe("check-flutter-analyze")
    expect(result.selectedJobExecutionStage?.stageLabel).toBe("cheap")
    expect(result.selectedJobExecutionStage?.stepLabel).toBe("check-flutter-analyze")
    const runningCheckpoint = result.selectedJobExecutionBreakdown.find((item) => item.key === "check-flutter-analyze")
    expect(runningCheckpoint?.state).toBe("running")
  })
})