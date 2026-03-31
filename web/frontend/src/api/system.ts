export interface AutoStartStatus {
  enabled: boolean
  supported: boolean
  platform: string
  message?: string
}

export interface LauncherConfig {
  port: number
  public: boolean
  allowed_cidrs: string[]
}

export interface OrchestratorStatus {
  schema_version: string
  instance_id: string
  state: string
  watch_runner_state?: string
  watch_runner_interval_seconds?: number
  watch_runner_started_at?: string
  watch_runner_stopped_at?: string
  watch_runner_last_error?: string
  watch_lock_state?: string
  watch_lock_owner_id?: string
  watch_lock_mode?: string
  watch_lock_acquired_at?: string
  watch_lock_updated_at?: string
  watch_lock_expires_at?: string
  last_trigger?: string
  last_pass_started_at?: string
  last_pass_finished_at?: string
  queued_recoveries?: number
  running_recoveries?: number
  foreign_live_lease_skips?: number
  last_error?: string
  updated_at: string
}

export interface PublicJobBudgets {
  iteration_budget: number
  token_budget: number
  elapsed_iterations?: number
  consumed_tokens?: number
}

export interface PublicJobRuntime {
  builder?: string
  worker_pool?: string
  network_policy?: string
}

export interface PublicJobLogs {
  summary_path?: string
  event_log_path?: string
}

export interface PublicJobFailureContext {
  failure_signature?: string
  last_error_summary?: string
  retryable: boolean
}

export interface PublicJobResumeContext {
  resume_allowed: boolean
  failure_domain?: string
  failure_category?: string
  recommended_resume_mode?: string
  requires_human_confirmation: boolean
  requires_preserved_workspace: boolean
  preserved_workspace_path?: string
  next_action?: string
}

export interface PublicJobRecord {
  schema_version: string
  job_id: string
  prd_id: string
  prd_version: string
  template_id: string
  status: string
  phase: string
  workspace_path: string
  artifact_dir: string
  builder_input_path?: string
  builder_output_path?: string
  budgets: PublicJobBudgets
  runtime?: PublicJobRuntime
  logs?: PublicJobLogs
  artifacts?: string[]
  human_approvals: string[]
  failure_context?: PublicJobFailureContext
  resume_context?: PublicJobResumeContext
  created_at: string
  updated_at: string
  started_at?: string
  finished_at?: string
}

export interface PublicJobEvent {
  at: string
  type: string
  job_id: string
  run_id?: string
  summary?: string
  snapshot_path?: string
  stage?: string
  status?: string
  failure_domain?: string
  failure_category?: string
  recommended_resume_mode?: string
  requires_human_confirmation?: boolean
  requires_preserved_workspace?: boolean
}

export interface PublicNotification {
  notification_id: string
  type: string
  job_id?: string
  approval_id?: string
  status?: string
  summary?: string
  suggested_action?: string
  links?: string[]
  created_at: string
  failure_domain?: string
  failure_category?: string
  recommended_resume_mode?: string
  requires_human_confirmation?: boolean
  requires_preserved_workspace?: boolean
  acknowledged?: boolean
  acknowledged_at?: string
}

export interface ArtifactItem {
  artifact_id: string
  path: string
  artifact_type: string
  produced: boolean
  required?: boolean
  label?: string
  description?: string
  size_bytes?: number
  sha256?: string
  content_type?: string
}

export interface ArtifactManifest {
  schema_version: string
  job_id: string
  generated_at: string
  items: ArtifactItem[]
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(path, options)
  if (!res.ok) {
    let message = `API error: ${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as {
        error?: string
        errors?: string[]
        message?: string
        error_code?: string
      }
      if (Array.isArray(body.errors) && body.errors.length > 0) {
        message = body.errors.join("; ")
      } else if (typeof body.message === "string" && body.message.trim() !== "") {
        message = body.message
      } else if (typeof body.error === "string" && body.error.trim() !== "") {
        message = body.error
      } else if (
        typeof body.error_code === "string" &&
        body.error_code.trim() !== ""
      ) {
        message = body.error_code
      }
    } catch {
      // Keep fallback error message when response body is not JSON.
    }
    throw new Error(message)
  }
  return res.json() as Promise<T>
}

export async function getAutoStartStatus(): Promise<AutoStartStatus> {
  return request<AutoStartStatus>("/api/system/autostart")
}

export async function setAutoStartEnabled(
  enabled: boolean,
): Promise<AutoStartStatus> {
  return request<AutoStartStatus>("/api/system/autostart", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  })
}

export async function getLauncherConfig(): Promise<LauncherConfig> {
  return request<LauncherConfig>("/api/system/launcher-config")
}

export async function setLauncherConfig(
  payload: LauncherConfig,
): Promise<LauncherConfig> {
  return request<LauncherConfig>("/api/system/launcher-config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function getOrchestratorStatus(): Promise<OrchestratorStatus> {
  return request<OrchestratorStatus>("/internal/v1/orchestrator/status")
}

export async function startOrchestratorWatch(
  intervalSeconds: number,
): Promise<OrchestratorStatus> {
  return request<OrchestratorStatus>("/internal/v1/orchestrator/watch:start", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ interval_seconds: intervalSeconds }),
  })
}

export async function stopOrchestratorWatch(): Promise<OrchestratorStatus> {
  return request<OrchestratorStatus>("/internal/v1/orchestrator/watch:stop", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  })
}

export async function unlockOrchestratorWatch(
  force: boolean,
): Promise<OrchestratorStatus> {
  return request<OrchestratorStatus>("/internal/v1/orchestrator:unlock-watch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ force }),
  })
}

export async function getJobs(status?: string): Promise<{
  items: PublicJobRecord[]
}> {
  const params = new URLSearchParams()
  if (status && status.trim() !== "") {
    params.set("status", status.trim())
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : ""
  return request<{ items: PublicJobRecord[] }>(`/api/v1/jobs${suffix}`)
}

export async function getJob(jobId: string): Promise<PublicJobRecord> {
  return request<PublicJobRecord>(`/api/v1/jobs/${encodeURIComponent(jobId)}`)
}

export async function getJobEvents(jobId: string): Promise<{
  job_id: string
  items: PublicJobEvent[]
}> {
  return request<{ job_id: string; items: PublicJobEvent[] }>(
    `/api/v1/jobs/${encodeURIComponent(jobId)}/events`,
  )
}

export async function getJobArtifacts(jobId: string): Promise<ArtifactManifest> {
  return request<ArtifactManifest>(
    `/api/v1/jobs/${encodeURIComponent(jobId)}/artifacts`,
  )
}

export async function getNotifications(acknowledged?: boolean): Promise<{
  items: PublicNotification[]
}> {
  const params = new URLSearchParams()
  if (typeof acknowledged === "boolean") {
    params.set("acknowledged", String(acknowledged))
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : ""
  return request<{ items: PublicNotification[] }>(
    `/api/v1/notifications${suffix}`,
  )
}

export async function acknowledgeNotification(
  notificationId: string,
): Promise<PublicNotification> {
  return request<PublicNotification>(
    `/api/v1/notifications/${encodeURIComponent(notificationId)}:ack`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    },
  )
}

export async function rebuildNotifications(): Promise<{
  items: PublicNotification[]
  rebuild: boolean
}> {
  return request<{ items: PublicNotification[]; rebuild: boolean }>(
    "/api/v1/notifications:rebuild",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    },
  )
}
