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

export interface PublicJobCurrentRound {
  round_id?: string
  attempt?: number
  checkpoint_key?: string
  current_phase?: string
  phase_trace?: string[]
  target_paths?: string[]
}

export interface PublicJobLogs {
  summary_path?: string
  event_log_path?: string
}

export interface PublicJobFailureContext {
  failure_signature?: string
  failure_domain?: string
  failure_category?: string
  last_error_summary?: string
  retryable: boolean
}

export interface PublicJobStatusContext {
  reason_code?: string
  summary?: string
  suggested_action?: string
}

export interface PublicJobResumeContext {
  resume_allowed: boolean
  failure_domain?: string
  failure_category?: string
  recommended_resume_mode?: string
  requires_human_confirmation: boolean
  requires_preserved_workspace: boolean
  preserved_workspace_path?: string
  suggested_action?: string
}

export interface PublicDeviceVerificationContext {
  record_path?: string
  status?: string
  summary?: string
  evidence_paths?: string[]
  verified_at?: string
  suggested_action?: string
}

export interface PublicReleaseFollowUpContext {
  record_path?: string
  status?: string
  summary?: string
  owner_id?: string
  evidence_paths?: string[]
  updated_at?: string
  suggested_action?: string
}

export interface PublicJobDeliveryContext {
  delivery_record_path: string
  status: string
  summary?: string
  suggested_action?: string
  release_channel?: string
  rollout_percent?: number
  reviewer_id?: string
  evidence_paths?: string[]
  required_changes?: string[]
  signed_artifact_paths?: string[]
  device_verification?: PublicDeviceVerificationContext
  release_follow_up?: PublicReleaseFollowUpContext
  recorded_at: string
}

export interface PublicJobRecord {
  schema_version: string
  job_id: string
  title?: string
  prd_id: string
  prd_version: string
  template_id: string
  status: string
  phase: string
  workspace_path: string
  artifact_dir: string
  status_context?: PublicJobStatusContext
  builder_input_path?: string
  builder_output_path?: string
  budgets: PublicJobBudgets
  runtime?: PublicJobRuntime
  current_round?: PublicJobCurrentRound
  logs?: PublicJobLogs
  artifacts?: string[]
  human_approvals: string[]
  failure_context?: PublicJobFailureContext
  resume_context?: PublicJobResumeContext
  delivery_context?: PublicJobDeliveryContext
  created_at: string
  updated_at: string
  started_at?: string
  finished_at?: string
}

export interface CompilePRDRequest {
  requirement_text: string
  requirement_source?: string
  title?: string
  job_id?: string
  prd_id?: string
  template_id?: string
  real_checks?: boolean
  executor_image?: string
}

export interface CompilePRDResponse {
  job_id: string
  prd_id: string
  template_id: string
  bundle_dir: string
  files?: {
    requirement_path?: string
    prd_markdown_path?: string
    prd_json_path?: string
    prd_approval_path?: string
    template_approval_path?: string
    template_fit_report_path?: string
    implementation_plan_path?: string
    builder_input_path?: string
  }
}

export interface CreateJobRequest {
  prd_id: string
  prd_version?: string
  template_id: string
  goal_summary?: string
}

export interface BuilderWorkerProfile {
  image: string
  android_api_level?: number
  emulator_profile?: string
  mount_templates?: string[]
  env?: Record<string, string>
  network_policy?: string
  workspace_root?: string
  artifacts_root?: string
  preserve_ttl_minutes?: number
}

export interface RegisterBuilderRequest {
  builder_id: string
  display_name: string
  endpoint?: string
  capability_tags?: string[]
  model_tags?: string[]
  priority?: number
  max_parallel_runs?: number
  worker_profile: BuilderWorkerProfile
}

export interface BuilderNode {
  builder_id: string
  display_name: string
  endpoint?: string
  status: string
  capability_tags?: string[]
  model_tags?: string[]
  priority?: number
  max_parallel_runs: number
  current_run_id?: string
  worker_profile: BuilderWorkerProfile
  last_seen_at?: string
  last_assigned_at?: string
  last_failure_reason?: string
  disabled_reason?: string
  created_at: string
  updated_at: string
}

export interface PublicJobEvent {
  at: string
  type: string
  job_id: string
  run_id?: string
  summary?: string
  round_id?: string
  attempt?: number
  checkpoint_key?: string
  current_phase?: string
  phase_trace?: string[]
  round_summaries?: {
    round_id?: string
    attempt?: number
    status?: string
    summary?: string
    current_phase?: string
    phase_trace?: string[]
    target_paths?: string[]
    modified_paths?: string[]
    file_facts?: PublicJobFileFact[]
    failed_checks?: string[]
    failure_signatures?: string[]
  }[]
  snapshot_path?: string
  target_paths?: string[]
  affected_paths?: string[]
  file_facts?: PublicJobFileFact[]
  stage?: string
  status?: string
  failure_domain?: string
  failure_category?: string
  recommended_resume_mode?: string
  requires_human_confirmation?: boolean
  requires_preserved_workspace?: boolean
  delivery_status?: string
  delivery_record_path?: string
  release_channel?: string
  rollout_percent?: number
  device_verification_status?: string
  release_follow_up_status?: string
}

export interface PublicJobFileFact {
  path: string
  state?: string
  change_type?: string
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
  delivery_status?: string
  delivery_record_path?: string
  release_channel?: string
  rollout_percent?: number
  device_verification_status?: string
  release_follow_up_status?: string
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

export interface ReviewPreparationResponse {
  ack: boolean
  run_id: string
  job_id: string
  status: string
  review_bundle_metadata_path: string
  review_bundle_path: string
  handoff_checklist_path: string
  artifact_manifest_path: string
  metrics_path: string
}

export interface RecordDeliveryPayload {
  run_id: string
  reviewer_id: string
  status: string
  summary?: string
  next_action?: string
  release_channel?: string
  rollout_percent?: number
  evidence_paths?: string[]
  required_changes?: string[]
  signed_artifact_paths?: string[]
  device_verification_status?: string
  device_verification_summary?: string
  device_verification_evidence_paths?: string[]
}

export interface RecordDeliveryResponse {
  ack: boolean
  job_id: string
  run_id: string
  status: string
  next_action: string
  delivery_record_path: string
}

export interface RecordDeliveryFollowUpPayload {
  run_id: string
  owner_id: string
  status: string
  summary?: string
  evidence_paths?: string[]
}

export interface RecordDeliveryFollowUpResponse {
  ack: boolean
  job_id: string
  run_id: string
  status: string
  follow_up_record_path: string
}

export type ApprovalRecordResponse = Record<string, unknown>

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

export async function compilePRD(
  payload: CompilePRDRequest,
): Promise<CompilePRDResponse> {
  return request<CompilePRDResponse>("/api/v1/prds:compile", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function createJob(
  payload: CreateJobRequest,
): Promise<PublicJobRecord> {
  return request<PublicJobRecord>("/api/v1/jobs", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function registerBuilder(
  payload: RegisterBuilderRequest,
): Promise<BuilderNode> {
  return request<BuilderNode>("/internal/v1/builders:register", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function getJob(jobId: string): Promise<PublicJobRecord> {
  return request<PublicJobRecord>(`/api/v1/jobs/${encodeURIComponent(jobId)}`)
}

export async function startJob(jobId: string): Promise<PublicJobRecord> {
  return request<PublicJobRecord>(`/api/v1/jobs/${encodeURIComponent(jobId)}:start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  })
}

export async function compilePrepareBundle(jobId: string): Promise<PublicJobRecord> {
  return request<PublicJobRecord>(
    `/api/v1/jobs/${encodeURIComponent(jobId)}:compile-prepare`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({}),
    },
  )
}

export async function submitPRDApproval(
  prdId: string,
  jobId: string,
): Promise<ApprovalRecordResponse> {
  return request<ApprovalRecordResponse>(
    `/api/v1/prds/${encodeURIComponent(prdId)}:submit-approval`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ job_id: jobId }),
    },
  )
}

export async function submitTemplateApproval(
  templateId: string,
  jobId: string,
  prdId: string,
): Promise<ApprovalRecordResponse> {
  return request<ApprovalRecordResponse>(
    `/api/v1/templates/${encodeURIComponent(templateId)}:submit-approval`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ job_id: jobId, prd_id: prdId }),
    },
  )
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

export async function prepareReview(
  runId: string,
): Promise<ReviewPreparationResponse> {
  return request<ReviewPreparationResponse>("/internal/v1/reviews:prepare", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ run_id: runId }),
  })
}

export async function recordDelivery(
  payload: RecordDeliveryPayload,
): Promise<RecordDeliveryResponse> {
  return request<RecordDeliveryResponse>("/internal/v1/deliveries:record", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}

export async function recordDeliveryFollowUp(
  payload: RecordDeliveryFollowUpPayload,
): Promise<RecordDeliveryFollowUpResponse> {
  return request<RecordDeliveryFollowUpResponse>("/internal/v1/deliveries:follow-up", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  })
}
