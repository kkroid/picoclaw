import { act, renderHook } from "@testing-library/react"
import { describe, expect, it } from "vitest"

import type { PublicJobRecord } from "@/api/system"
import { useJobsPageUiState } from "@/components/jobs/jobs-page-ui-state"

function createJob(jobId: string): PublicJobRecord {
  return {
    schema_version: "v1",
    job_id: jobId,
    prd_id: `prd-${jobId}`,
    prd_version: "1",
    template_id: `template-${jobId}`,
    status: "running",
    phase: "orchestrator",
    workspace_path: `workspace/jobs/${jobId}`,
    artifact_dir: `workspace/jobs/${jobId}/artifacts`,
    budgets: {
      iteration_budget: 10,
      token_budget: 1000,
    },
    human_approvals: [],
    created_at: "2026-04-04T00:00:00Z",
    updated_at: "2026-04-04T00:01:00Z",
  }
}

describe("useJobsPageUiState", () => {
  it("会在选中任务失效时回退到列表首项", () => {
    const firstJob = createJob("job-001")
    const secondJob = createJob("job-002")

    const { result, rerender } = renderHook(
      ({ jobItems }) =>
        useJobsPageUiState({
          jobItems,
        }),
      {
        initialProps: {
          jobItems: [firstJob, secondJob],
        },
      },
    )

    act(() => {
      result.current.setSelectedJobId("job-002")
    })
    expect(result.current.effectiveSelectedJobId).toBe("job-002")

    rerender({
      jobItems: [firstJob],
    })

    expect(result.current.effectiveSelectedJobId).toBe("job-001")
  })

  it("重置创建表单时会关闭弹窗并恢复默认值", () => {
    const { result } = renderHook(() =>
      useJobsPageUiState({
        jobItems: [],
      }),
    )

    act(() => {
      result.current.setShowCreateJobSheet(true)
      result.current.setCreateJobForm({
        title: "Android Demo",
        requirementText: "Build a release candidate",
        advancedOpen: true,
        realChecks: false,
        autoStartBuilder: false,
      })
    })

    act(() => {
      result.current.resetCreateJobForm()
    })

    expect(result.current.showCreateJobSheet).toBe(false)
    expect(result.current.createJobForm).toEqual({
      title: "",
      requirementText: "",
      advancedOpen: false,
      realChecks: true,
      autoStartBuilder: true,
    })
  })
})