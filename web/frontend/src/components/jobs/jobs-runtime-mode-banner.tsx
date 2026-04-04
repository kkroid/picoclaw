import type { BuilderRuntimeMode } from "@/components/jobs/jobs-page-utils"
import { cn } from "@/lib/utils"

type Translate = (key: string, options?: Record<string, unknown>) => string

export function JobsRuntimeModeBanner({
  t,
  builderRuntimeMode,
}: {
  t: Translate
  builderRuntimeMode: BuilderRuntimeMode
}) {
  return (
    <div
      className={cn(
        "rounded-xl border px-4 py-3 text-sm",
        builderRuntimeMode.enabled
          ? "border-emerald-200 bg-emerald-50 text-emerald-950"
          : "border-amber-200 bg-amber-50 text-amber-950",
      )}
    >
      <div className="font-medium">{t("jobs.runtimeMode.title")}</div>
      <div className="mt-1 text-sm/6">
        {builderRuntimeMode.enabled
          ? t("jobs.runtimeMode.enabled", {
              defaultModel: builderRuntimeMode.defaultModel,
              upgradeModel: builderRuntimeMode.upgradeModel,
            })
          : t("jobs.runtimeMode.disabled")}
      </div>
    </div>
  )
}