import { isRunningJobStatus, renderStatusLabel } from "@/components/jobs/jobs-page-state"
import { cn } from "@/lib/utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsStatusBadge({
  status,
  t,
}: {
  status: string
  t: Translate
}) {
  return (
    <span
      className={cn(
        "rounded-md px-2 py-0.5 text-xs font-medium",
        status === "completed" && "bg-emerald-100 text-emerald-800",
        isRunningJobStatus(status) && "bg-sky-100 text-sky-800",
        status === "failed" && "bg-rose-100 text-rose-800",
        status === "cancelled" && "bg-slate-200 text-slate-700",
        !["completed", "failed", "cancelled"].includes(status) &&
          !isRunningJobStatus(status) &&
          "bg-amber-100 text-amber-800",
      )}
    >
      {renderStatusLabel(status, t)}
    </span>
  )
}