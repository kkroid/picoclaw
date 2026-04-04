import { describe, expect, it, vi, beforeEach, afterEach } from "vitest"

import { createJobsWorkspaceController } from "@/components/jobs/jobs-workspace-controller"

describe("createJobsWorkspaceController", () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it("会在打开执行区时同步选中任务、切换标签并刷新 job/event 查询", async () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined)
    const setSelectedJobId = vi.fn()
    const setDetailTab = vi.fn()
    const focusElementById = vi.fn()

    const controller = createJobsWorkspaceController({
      queryClient: { invalidateQueries } as never,
      setSelectedJobId,
      setDetailTab,
      focusElementById,
    })

    controller.openJobExecution("job-001")

    expect(setSelectedJobId).toHaveBeenCalledWith("job-001")
    expect(setDetailTab).toHaveBeenCalledWith("execution")
    expect(focusElementById).toHaveBeenCalledWith("jobs-events")
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["appfactory", "job", "job-001"] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["appfactory", "job-events", "job-001"] })

    await vi.runAllTimersAsync()

    expect(focusElementById).toHaveBeenCalledTimes(2)
  })

  it("会在打开交付 follow-up 区时刷新 job/artifact 查询并切换到 delivery 标签", () => {
    const invalidateQueries = vi.fn().mockResolvedValue(undefined)
    const controller = createJobsWorkspaceController({
      queryClient: { invalidateQueries } as never,
      setSelectedJobId: vi.fn(),
      setDetailTab: vi.fn(),
      focusElementById: vi.fn(),
    })

    controller.openJobDelivery("job-002", "jobs-delivery-follow-up")

    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["appfactory", "job", "job-002"] })
    expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ["appfactory", "job-artifacts", "job-002"] })
  })
})