import { IconLoader2 } from "@tabler/icons-react"

import { Button } from "@/components/ui/button"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobSuggestedActionButton({
  t,
  actionLabel,
  executable,
  onClick,
  disabled = false,
  pending = false,
  variant = "default",
  size = "sm",
  showManualHint = true,
}: {
  t: Translate
  actionLabel: string
  executable: boolean
  onClick?: () => void
  disabled?: boolean
  pending?: boolean
  variant?: "default" | "secondary" | "outline"
  size?: "sm" | "default"
  showManualHint?: boolean
}) {
  if (!executable) {
    return showManualHint ? (
      <div className="text-muted-foreground pt-2 text-xs">
        {t("jobs.detail.manualActionHint")}
      </div>
    ) : null
  }

  return (
    <Button variant={variant} size={size} disabled={disabled} onClick={onClick}>
      {pending ? <IconLoader2 className="size-4 animate-spin" /> : null}
      {t("jobs.detail.runSuggestedAction", {
        action: actionLabel,
      })}
    </Button>
  )
}