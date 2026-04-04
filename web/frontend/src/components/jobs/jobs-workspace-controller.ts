import type { QueryClient } from "@tanstack/react-query"

export type JobWorkspaceTab = "overview" | "execution" | "artifacts" | "delivery"

type FocusElement = (id: string) => void

function resolveWorkspaceTabForTarget(targetId: string): JobWorkspaceTab | undefined {
  switch (targetId) {
    case "jobs-events":
      return "execution"
    case "jobs-artifacts":
      return "artifacts"
    case "jobs-delivery-context":
    case "jobs-delivery-follow-up":
      return "delivery"
    default:
      return undefined
  }
}

export function focusJobWorkspaceTarget({
  targetId,
  setDetailTab,
  focusElementById,
}: {
  targetId: string
  setDetailTab: (tab: JobWorkspaceTab) => void
  focusElementById: FocusElement
}) {
  const detailTab = resolveWorkspaceTabForTarget(targetId)
  if (detailTab) {
    setDetailTab(detailTab)
    setTimeout(() => {
      focusElementById(targetId)
    }, 0)
  }
  focusElementById(targetId)
}

export function createJobsWorkspaceController({
  queryClient,
  setSelectedJobId,
  setDetailTab,
  focusElementById,
}: {
  queryClient: QueryClient
  setSelectedJobId: (jobId: string) => void
  setDetailTab: (tab: JobWorkspaceTab) => void
  focusElementById: FocusElement
}) {
  const focusTarget = (targetId: string) => {
    focusJobWorkspaceTarget({
      targetId,
      setDetailTab,
      focusElementById,
    })
  }

  const invalidateJobTarget = async ({
    jobId,
    includeEvents = false,
    includeArtifacts = false,
  }: {
    jobId: string
    includeEvents?: boolean
    includeArtifacts?: boolean
  }) => {
    const invalidations = [
      queryClient.invalidateQueries({ queryKey: ["appfactory", "job", jobId] }),
    ]
    if (includeEvents) {
      invalidations.push(
        queryClient.invalidateQueries({ queryKey: ["appfactory", "job-events", jobId] }),
      )
    }
    if (includeArtifacts) {
      invalidations.push(
        queryClient.invalidateQueries({ queryKey: ["appfactory", "job-artifacts", jobId] }),
      )
    }
    await Promise.all(invalidations)
  }

  const openJobTarget = ({
    jobId,
    targetId,
    includeEvents = false,
    includeArtifacts = false,
  }: {
    jobId: string
    targetId: string
    includeEvents?: boolean
    includeArtifacts?: boolean
  }) => {
    setSelectedJobId(jobId)
    void invalidateJobTarget({
      jobId,
      includeEvents,
      includeArtifacts,
    })
    focusTarget(targetId)
  }

  return {
    openOverview() {
      setDetailTab("overview")
    },
    focusTarget,
    focusOrchestrator() {
      focusElementById("jobs-orchestrator-card")
    },
    openJobExecution(jobId: string) {
      openJobTarget({
        jobId,
        targetId: "jobs-events",
        includeEvents: true,
      })
    },
    openJobArtifacts(jobId: string) {
      openJobTarget({
        jobId,
        targetId: "jobs-artifacts",
        includeArtifacts: true,
      })
    },
    openJobDelivery(jobId: string, targetId: "jobs-delivery-context" | "jobs-delivery-follow-up") {
      openJobTarget({
        jobId,
        targetId,
        includeArtifacts: true,
      })
    },
  }
}

export type JobsWorkspaceController = ReturnType<typeof createJobsWorkspaceController>