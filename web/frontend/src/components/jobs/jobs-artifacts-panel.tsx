import type { ArtifactItem, PublicJobRecord } from "@/api/system"
import {
  EmptyBlock,
  ErrorBlock,
  LoadingBlock,
} from "@/components/jobs/jobs-page-primitives"
import { JobArtifactRow } from "@/components/jobs/jobs-detail-rows"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

type Translate = (key: string, options?: Record<string, unknown>) => string

type JobDeliverableSummary = {
  key: string
  label: string
  artifactType: string
  localPath: string
  relativePath?: string
}

export function JobsArtifactsPanel({
  t,
  job,
  artifactItems,
  loading,
  errorMessage,
}: {
  t: Translate
  job?: PublicJobRecord
  artifactItems: ArtifactItem[]
  loading: boolean
  errorMessage?: string
}) {
  if (!job) {
    return (
      <Card id="jobs-artifacts" className="border-border/60 bg-card/80" size="sm">
        <CardHeader>
          <CardTitle>{t("jobs.artifacts.title")}</CardTitle>
          <CardDescription>{t("jobs.artifacts.description")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <EmptyBlock message={t("jobs.detail.empty")} />
        </CardContent>
      </Card>
    )
  }

  const appFactoryRoot = deriveAppFactoryRoot(job, artifactItems)
  const finalDeliverables = collectFinalDeliverables(job, artifactItems, appFactoryRoot)

  return (
    <Card id="jobs-artifacts" className="border-border/60 bg-card/80" size="sm">
      <CardHeader>
        <CardTitle>{t("jobs.artifacts.title")}</CardTitle>
        <CardDescription>{t("jobs.artifacts.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? (
          <LoadingBlock label={t("labels.loading")} />
        ) : errorMessage ? (
          <ErrorBlock message={errorMessage} />
        ) : (
          <>
            <div className="rounded-xl border border-amber-200 bg-amber-50/70 p-4">
              <div className="text-sm font-semibold text-amber-950">{t("jobs.artifacts.deliveryTitle")}</div>
              <div className="mt-1 text-sm text-amber-950/85">
                {t("jobs.artifacts.deliveryDescription")}
              </div>
              {finalDeliverables.length > 0 ? (
                <div className="mt-3 space-y-3">
                  {finalDeliverables.map((item) => (
                    <div
                      key={item.key}
                      className="rounded-lg border border-amber-200 bg-background/80 p-3"
                    >
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{item.label}</span>
                        <span className="rounded-md bg-muted px-2 py-0.5 text-xs">
                          {item.artifactType}
                        </span>
                      </div>
                      <div className="mt-2 space-y-1 text-xs">
                        <div className="text-muted-foreground">
                          {t("jobs.artifacts.localPickupPath")}
                        </div>
                        <div className="font-mono break-all">{item.localPath}</div>
                        {item.relativePath ? (
                          <div className="text-muted-foreground font-mono break-all">
                            {item.relativePath}
                          </div>
                        ) : null}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="mt-3 text-sm text-amber-950/85">
                  {t("jobs.artifacts.noFinalDeliverable")}
                </div>
              )}
              <div className="mt-3 space-y-1 text-xs text-amber-950/80">
                <div>{t("jobs.artifacts.downloadUnavailable")}</div>
                {appFactoryRoot ? (
                  <div className="font-mono break-all">
                    {t("jobs.artifacts.workspaceRoot", { path: appFactoryRoot })}
                  </div>
                ) : null}
              </div>
            </div>

            {artifactItems.length === 0 ? (
              <EmptyBlock message={t("jobs.artifacts.empty")} />
            ) : (
              artifactItems.map((item) => (
                <JobArtifactRow
                  key={item.artifact_id}
                  item={item}
                  localPath={resolveArtifactLocalPath(job, item.path, appFactoryRoot)}
                  t={t}
                />
              ))
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}

function deriveAppFactoryRoot(job: PublicJobRecord, artifacts: ArtifactItem[]) {
  const relativeCandidates = [
    job.logs?.summary_path,
    job.logs?.event_log_path,
    job.builder_output_path,
    job.workspace_path,
    job.artifact_dir,
  ].filter((value): value is string => Boolean(value && value.trim() !== ""))

  for (const artifact of artifacts) {
    const absolutePath = artifact.path.trim()
    if (!absolutePath.startsWith("/")) {
      continue
    }
    for (const relativePath of relativeCandidates) {
      if (absolutePath.endsWith(relativePath)) {
        return absolutePath.slice(0, absolutePath.length - relativePath.length).replace(/\/$/, "")
      }
    }
  }
  return ""
}

function resolveArtifactLocalPath(
  job: PublicJobRecord,
  artifactPath: string,
  appFactoryRoot: string,
) {
  const normalized = artifactPath.trim()
  if (normalized === "") {
    return ""
  }
  if (normalized.startsWith("/")) {
    return normalized
  }
  const fullRelativePath = normalized.startsWith("jobs/")
    ? normalized
    : `jobs/${job.job_id}/${normalized}`
  if (!appFactoryRoot) {
    return fullRelativePath
  }
  return `${appFactoryRoot}/${fullRelativePath}`
}

function collectFinalDeliverables(
  job: PublicJobRecord,
  artifacts: ArtifactItem[],
  appFactoryRoot: string,
): JobDeliverableSummary[] {
  const selected = artifacts.filter((item) => {
    if (!item.produced) {
      return false
    }
    if (["apk", "aab", "ipa"].includes(item.artifact_type)) {
      return true
    }
    return /(apk|bundle|signed)/i.test(item.artifact_id)
  })
  return selected.map((item) => ({
    key: item.artifact_id,
    label: item.label || item.artifact_id,
    artifactType: item.artifact_type,
    localPath: resolveArtifactLocalPath(job, item.path, appFactoryRoot),
    relativePath: item.path,
  }))
}