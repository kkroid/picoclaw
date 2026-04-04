import type { OrchestratorStatus } from "@/api/system"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function getOrchestratorToneClass(data: OrchestratorStatus | undefined) {
  if (!data) {
    return "bg-muted-foreground/40"
  }
  if (data.watch_runner_state === "running") {
    return "bg-emerald-500"
  }
  if (data.watch_runner_state === "stopping") {
    return "bg-amber-500"
  }
  if (data.watch_lock_state === "held_by_other") {
    return "bg-amber-500"
  }
  if (data.watch_lock_state === "stale") {
    return "bg-orange-500"
  }
  return "bg-slate-400"
}

export function getOrchestratorLabel(
  data: OrchestratorStatus | undefined,
  t: Translate,
) {
  if (!data) {
    return t("header.orchestrator.status.loading")
  }
  if (data.watch_runner_state === "running") {
    return t("header.orchestrator.status.running")
  }
  if (data.watch_runner_state === "stopping") {
    return t("header.orchestrator.status.stopping")
  }
  if (data.watch_lock_state === "held_by_other") {
    return t("header.orchestrator.status.locked")
  }
  if (data.watch_lock_state === "stale") {
    return t("header.orchestrator.status.stale")
  }
  return t("header.orchestrator.status.idle")
}

export function getOrchestratorDetail(
  data: OrchestratorStatus | undefined,
  t: Translate,
) {
  if (!data) {
    return t("header.orchestrator.menu.loading")
  }
  if (data.watch_runner_state === "running") {
    return t("header.orchestrator.menu.runningDetail", {
      interval: data.watch_runner_interval_seconds ?? 15,
    })
  }
  if (data.watch_runner_state === "stopping") {
    return t("header.orchestrator.menu.stoppingDetail")
  }
  if (data.watch_lock_state === "held_by_other") {
    return t("header.orchestrator.menu.lockedDetail")
  }
  if (data.watch_lock_state === "stale") {
    return t("header.orchestrator.menu.staleDetail")
  }
  return t("header.orchestrator.menu.idleDetail")
}