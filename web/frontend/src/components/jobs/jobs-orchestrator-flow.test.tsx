import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { act, renderHook, waitFor } from "@testing-library/react"
import { describe, expect, it, vi, beforeEach } from "vitest"

import { useJobsOrchestratorFlow } from "@/components/jobs/jobs-orchestrator-flow"

const {
  startOrchestratorWatchMock,
  stopOrchestratorWatchMock,
  unlockOrchestratorWatchMock,
  toastSuccessMock,
  toastErrorMock,
} = vi.hoisted(() => ({
  startOrchestratorWatchMock: vi.fn(),
  stopOrchestratorWatchMock: vi.fn(),
  unlockOrchestratorWatchMock: vi.fn(),
  toastSuccessMock: vi.fn(),
  toastErrorMock: vi.fn(),
}))

vi.mock("@/api/system", async () => {
  const actual = await vi.importActual<typeof import("@/api/system")>("@/api/system")
  return {
    ...actual,
    startOrchestratorWatch: startOrchestratorWatchMock,
    stopOrchestratorWatch: stopOrchestratorWatchMock,
    unlockOrchestratorWatch: unlockOrchestratorWatchMock,
  }
})

vi.mock("sonner", () => ({
  toast: {
    success: toastSuccessMock,
    error: toastErrorMock,
  },
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe("useJobsOrchestratorFlow", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("会根据编排器状态导出运行态和锁异常", () => {
    const { result } = renderHook(
      () =>
        useJobsOrchestratorFlow({
          t: (key) => key,
          data: {
            schema_version: "v1",
            instance_id: "orch-1",
            state: "running",
            updated_at: "2026-04-04T00:00:00Z",
            watch_runner_state: "running",
            watch_lock_state: "stale",
          },
          loading: false,
          refreshOrchestrator: vi.fn().mockResolvedValue(undefined),
        }),
      { wrapper: createWrapper() },
    )

    expect(result.current.watcherRunning).toBe(true)
    expect(result.current.watcherStopping).toBe(false)
    expect(result.current.lockIssue).toBe(true)
    expect(result.current.orchestratorBusy).toBe(false)
  })

  it("启动 watch 成功后会提示成功并刷新状态", async () => {
    const refreshOrchestrator = vi.fn().mockResolvedValue(undefined)
    startOrchestratorWatchMock.mockResolvedValueOnce({ ack: true })

    const { result } = renderHook(
      () =>
        useJobsOrchestratorFlow({
          t: (key) => key,
          data: undefined,
          loading: false,
          refreshOrchestrator,
        }),
      { wrapper: createWrapper() },
    )

    act(() => {
      result.current.startWatchMutation.mutate()
    })

    await waitFor(() => {
      expect(startOrchestratorWatchMock).toHaveBeenCalledWith(15)
    })
    await waitFor(() => {
      expect(toastSuccessMock).toHaveBeenCalledWith("header.orchestrator.toast.started")
    })
    expect(refreshOrchestrator).toHaveBeenCalled()
  })

  it("停止 watch 失败时会提示错误", async () => {
    const refreshOrchestrator = vi.fn().mockResolvedValue(undefined)
    stopOrchestratorWatchMock.mockRejectedValueOnce(new Error("stop failed"))

    const { result } = renderHook(
      () =>
        useJobsOrchestratorFlow({
          t: (key) => key,
          data: undefined,
          loading: false,
          refreshOrchestrator,
        }),
      { wrapper: createWrapper() },
    )

    act(() => {
      result.current.stopWatchMutation.mutate()
    })

    await waitFor(() => {
      expect(toastErrorMock).toHaveBeenCalledWith("stop failed")
    })
    expect(refreshOrchestrator).not.toHaveBeenCalled()
  })

  it("强制解锁成功时会调用 unlock 并刷新状态", async () => {
    const refreshOrchestrator = vi.fn().mockResolvedValue(undefined)
    unlockOrchestratorWatchMock.mockResolvedValueOnce({ ack: true })

    const { result } = renderHook(
      () =>
        useJobsOrchestratorFlow({
          t: (key) => key,
          data: undefined,
          loading: false,
          refreshOrchestrator,
        }),
      { wrapper: createWrapper() },
    )

    act(() => {
      result.current.unlockWatchMutation.mutate()
    })

    await waitFor(() => {
      expect(unlockOrchestratorWatchMock).toHaveBeenCalledWith(true)
    })
    await waitFor(() => {
      expect(toastSuccessMock).toHaveBeenCalledWith("header.orchestrator.toast.unlocked")
    })
    expect(refreshOrchestrator).toHaveBeenCalled()
  })
})