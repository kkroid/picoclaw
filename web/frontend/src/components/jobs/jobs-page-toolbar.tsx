import { IconLoader2, IconPlus, IconRefresh } from "@tabler/icons-react"

import { Button } from "@/components/ui/button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsPageToolbar({
  t,
  rebuildPending,
  onOpenCreate,
  onRefresh,
  onRebuildNotifications,
}: {
  t: Translate
  rebuildPending: boolean
  onOpenCreate: () => void
  onRefresh: () => void
  onRebuildNotifications: () => void
}) {
  return (
    <div className="flex items-center gap-2">
      <Button size="sm" onClick={onOpenCreate}>
        <IconPlus className="size-4" />
        {t("jobs.actions.createJob")}
      </Button>
      <Button variant="outline" size="sm" onClick={onRefresh}>
        <IconRefresh className="size-4" />
        {t("jobs.actions.refresh")}
      </Button>
      <Button
        variant="secondary"
        size="sm"
        disabled={rebuildPending}
        onClick={onRebuildNotifications}
      >
        {rebuildPending ? <IconLoader2 className="size-4 animate-spin" /> : null}
        {t("jobs.actions.rebuildNotifications")}
      </Button>
    </div>
  )
}