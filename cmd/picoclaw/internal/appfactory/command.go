package appfactory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/appfactory/adapter"
	appprepare "github.com/sipeed/picoclaw/pkg/appfactory/prepare"
	appruns "github.com/sipeed/picoclaw/pkg/appfactory/runs"
	"github.com/sipeed/picoclaw/pkg/envfile"
	api "github.com/sipeed/picoclaw/web/backend/api"
	"github.com/sipeed/picoclaw/web/backend/launcherconfig"
)

func NewAppFactoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "appfactory",
		Short: "Run minimal app factory helper commands",
	}
	cmd.AddCommand(newPrepareCommand())
	cmd.AddCommand(newRunRequirementCommand())
	cmd.AddCommand(newRunExecutorCommand())
	cmd.AddCommand(newRunExecutorOnceCommand())
	cmd.AddCommand(newOrchestratorCommand())
	return cmd
}

func newOrchestratorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orchestrator",
		Short: "Run the minimal public job orchestrator outside HTTP route startup",
	}
	cmd.AddCommand(newOrchestratorRunCommand())
	cmd.AddCommand(newOrchestratorStatusCommand())
	cmd.AddCommand(newOrchestratorUnlockWatchCommand())
	return cmd
}

func loadClosestAppFactoryEnv(configPath string) error {
	searchRoot := filepath.Dir(configPath)
	if strings.TrimSpace(configPath) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory for appfactory env loading: %w", err)
		}
		searchRoot = wd
	}
	if _, err := envfile.LoadClosest(searchRoot, false); err != nil {
		if errors.Is(err, envfile.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("load appfactory env from %s: %w", filepath.Clean(searchRoot), err)
	}
	return nil
}

func newOrchestratorRunCommand() *cobra.Command {
	var (
		configPath string
		watch      bool
		interval   time.Duration
		maxPasses  int
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run persisted public job orchestrator recovery/dispatch passes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			if strings.TrimSpace(configPath) == "" {
				configPath = internal.GetConfigPath()
			}
			if err := loadClosestAppFactoryEnv(configPath); err != nil {
				return err
			}
			handler := api.NewHandler(configPath)
			if !watch {
				status, err := handler.RunPublicJobOrchestratorPass("cli_once")
				if err != nil {
					return err
				}
				handler.WaitForAsyncJobs()
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "mode=once\nconfig=%s\nstate=%s\nlast_trigger=%s\nqueued_recoveries=%d\nrunning_recoveries=%d\nforeign_live_lease_skips=%d\n", configPath, status.State, status.LastTrigger, status.QueuedRecoveries, status.RunningRecoveries, status.ForeignLiveLeaseSkips)
				return err
			}
			result, err := handler.RunPublicJobOrchestratorWatch(cmd.Context(), "cli_watch", interval, maxPasses)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "mode=watch\nconfig=%s\ninterval=%s\nwatch_runner_mode=%s\nwatch_runner_owner_id=%s\ncompleted_passes=%d\nstate=%s\nlast_trigger=%s\n", configPath, interval, result.Status.WatchRunnerMode, result.Status.WatchRunnerOwnerID, result.CompletedPasses, result.Status.State, result.Status.LastTrigger)
			if err != nil {
				return err
			}
			handler.WaitForAsyncJobs()
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config.json; defaults to resolved PicoClaw config path")
	cmd.Flags().BoolVar(&watch, "watch", false, "Keep polling orchestrator recovery instead of running a single pass")
	cmd.Flags().DurationVar(&interval, "interval", 15*time.Second, "Polling interval used with --watch")
	cmd.Flags().IntVar(&maxPasses, "max-passes", 0, "Stop watch mode after N passes; 0 means run until cancelled")
	return cmd
}

func newOrchestratorStatusCommand() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the latest persisted orchestrator status snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(configPath) == "" {
				configPath = internal.GetConfigPath()
			}
			if err := loadClosestAppFactoryEnv(configPath); err != nil {
				return err
			}
			handler := api.NewHandler(configPath)
			status, err := handler.LoadPublicJobOrchestratorStatus()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "config=%s\nstate=%s\nlast_trigger=%s\nlast_pass_started_at=%s\nlast_pass_finished_at=%s\nqueued_recoveries=%d\nrunning_recoveries=%d\nforeign_live_lease_skips=%d\nwatch_runner_state=%s\nwatch_runner_mode=%s\nwatch_runner_owner_id=%s\nwatch_lock_state=%s\nwatch_lock_owner_id=%s\nwatch_lock_mode=%s\nwatch_lock_acquired_at=%s\nwatch_lock_updated_at=%s\nwatch_lock_expires_at=%s\nlast_error=%s\n", configPath, status.State, status.LastTrigger, status.LastPassStartedAt, status.LastPassFinishedAt, status.QueuedRecoveries, status.RunningRecoveries, status.ForeignLiveLeaseSkips, status.WatchRunnerState, status.WatchRunnerMode, status.WatchRunnerOwnerID, status.WatchLockState, status.WatchLockOwnerID, status.WatchLockMode, status.WatchLockAcquiredAt, status.WatchLockUpdatedAt, status.WatchLockExpiresAt, status.LastError)
			return err
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config.json; defaults to resolved PicoClaw config path")
	return cmd
}

func newOrchestratorUnlockWatchCommand() *cobra.Command {
	var (
		configPath string
		force      bool
	)
	cmd := &cobra.Command{
		Use:   "unlock-watch",
		Short: "Release a stale or foreign orchestrator watch lock",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(configPath) == "" {
				configPath = internal.GetConfigPath()
			}
			if err := loadClosestAppFactoryEnv(configPath); err != nil {
				return err
			}
			handler := api.NewHandler(configPath)
			status, err := handler.UnlockPublicJobOrchestratorWatchLock(force)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "config=%s\nwatch_lock_state=%s\nwatch_lock_owner_id=%s\nwatch_lock_mode=%s\nwatch_lock_acquired_at=%s\nwatch_lock_updated_at=%s\nwatch_lock_expires_at=%s\n", configPath, status.WatchLockState, status.WatchLockOwnerID, status.WatchLockMode, status.WatchLockAcquiredAt, status.WatchLockUpdatedAt, status.WatchLockExpiresAt)
			return err
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "", "Path to config.json; defaults to resolved PicoClaw config path")
	cmd.Flags().BoolVar(&force, "force", false, "Release an active foreign watch lock instead of only clearing self-owned or stale locks")
	return cmd
}

func newPrepareCommand() *cobra.Command {
	var (
		requirement     string
		requirementFile string
		outputDir       string
		title           string
		jobID           string
		prdID           string
		templateID      string
		realChecks      bool
		executorImage   string
	)
	cmd := &cobra.Command{
		Use:   "prepare",
		Short: "Compile a natural-language requirement into PRD and builder-input artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(outputDir) == "" {
				return fmt.Errorf("--output-dir is required")
			}
			if strings.TrimSpace(requirement) == "" && strings.TrimSpace(requirementFile) == "" {
				return fmt.Errorf("either --requirement or --requirement-file is required")
			}
			if strings.TrimSpace(requirement) != "" && strings.TrimSpace(requirementFile) != "" {
				return fmt.Errorf("--requirement and --requirement-file are mutually exclusive")
			}
			requirementText := requirement
			requirementSource := "inline:requirement"
			if strings.TrimSpace(requirementFile) != "" {
				data, err := os.ReadFile(requirementFile)
				if err != nil {
					return fmt.Errorf("read requirement file: %w", err)
				}
				requirementText = string(data)
				requirementSource = requirementFile
			}
			bundle, err := appprepare.Compile(appprepare.Request{
				RequirementText:   requirementText,
				RequirementSource: requirementSource,
				TitleHint:         title,
				JobID:             jobID,
				PRDID:             prdID,
				TemplateID:        templateID,
				ExecutorImage:     executorImage,
				RealBuild:         realChecks || strings.TrimSpace(executorImage) != "",
			})
			if err != nil {
				return err
			}
			if err := appprepare.WriteBundle(outputDir, bundle); err != nil {
				return err
			}
			for _, name := range bundle.FileNames() {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n", filepath.Join(outputDir, name)); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&requirement, "requirement", "", "Inline natural-language requirement text")
	cmd.Flags().StringVar(&requirementFile, "requirement-file", "", "Path to a requirement text or markdown file")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to write generated artifacts into")
	cmd.Flags().StringVar(&title, "title", "", "Optional PRD title override")
	cmd.Flags().StringVar(&jobID, "job-id", "", "Optional job ID override")
	cmd.Flags().StringVar(&prdID, "prd-id", "", "Optional PRD ID override")
	cmd.Flags().StringVar(&templateID, "template-id", "", "Optional template ID override")
	cmd.Flags().BoolVar(&realChecks, "real-checks", false, "Generate real Flutter acceptance checks instead of echo smoke checks")
	cmd.Flags().StringVar(&executorImage, "executor-image", "", "Optional default container image written into generated builder-input.json")
	return cmd
}

func newRunRequirementCommand() *cobra.Command {
	var (
		requirement     string
		requirementFile string
		outputDir       string
		title           string
		jobID           string
		prdID           string
		templateID      string
		apiBase         string
		executorImage   string
		realChecks      bool
		builderID       string
		displayName     string
		builderImage    string
		capabilityTags  []string
		modelTags       []string
		timeout         time.Duration
	)
	cmd := &cobra.Command{
		Use:   "run-requirement",
		Short: "Compile a requirement into artifacts and execute one minimal appfactory run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(builderID) == "" {
				return fmt.Errorf("--builder-id is required")
			}
			if strings.TrimSpace(outputDir) == "" {
				tempDir, err := os.MkdirTemp("", "picoclaw-appfactory-")
				if err != nil {
					return fmt.Errorf("create temp output dir: %w", err)
				}
				outputDir = tempDir
			}
			bundle, err := compileRequirementBundle(requirementCompileOptions{
				Requirement:     requirement,
				RequirementFile: requirementFile,
				OutputDir:       outputDir,
				Title:           title,
				JobID:           jobID,
				PRDID:           prdID,
				TemplateID:      templateID,
				ExecutorImage:   executorImage,
				RealBuild:       realChecks || strings.TrimSpace(executorImage) != "",
			})
			if err != nil {
				return err
			}
			if err := appprepare.WriteBundle(outputDir, bundle); err != nil {
				return err
			}
			bundle.BuilderInput.ContextSourceDir = outputDir
			bundle.BuilderInput.ExecutorImage = strings.TrimSpace(executorImage)
			runResult, err := executeRunOnce(cmd.Context(), runOnceOptions{
				APIBase:        apiBase,
				Input:          bundle.BuilderInput,
				BuilderID:      builderID,
				DisplayName:    displayName,
				BuilderImage:   builderImage,
				CapabilityTags: capabilityTags,
				ModelTags:      modelTags,
				Timeout:        timeout,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "bundle_dir=%s\nrun_id=%s\nworker_id=%s\n", outputDir, runResult.RunID, runResult.WorkerID); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&requirement, "requirement", "", "Inline natural-language requirement text")
	cmd.Flags().StringVar(&requirementFile, "requirement-file", "", "Path to a requirement text or markdown file")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to write generated artifacts into; defaults to a temp directory")
	cmd.Flags().StringVar(&title, "title", "", "Optional PRD title override")
	cmd.Flags().StringVar(&jobID, "job-id", "", "Optional job ID override")
	cmd.Flags().StringVar(&prdID, "prd-id", "", "Optional PRD ID override")
	cmd.Flags().StringVar(&templateID, "template-id", "", "Optional template ID override")
	cmd.Flags().StringVar(&apiBase, "api-base", "", "Internal API base URL, defaulting to local launcher backend")
	cmd.Flags().StringVar(&executorImage, "executor-image", "", "Optional container image used to execute launch commands")
	cmd.Flags().BoolVar(&realChecks, "real-checks", false, "Generate and execute real Flutter acceptance checks")
	cmd.Flags().StringVar(&builderID, "builder-id", "", "Builder ID used for this run")
	cmd.Flags().StringVar(&displayName, "display-name", "", "Builder display name")
	cmd.Flags().StringVar(&builderImage, "builder-image", "", "Builder worker image label to register")
	cmd.Flags().StringSliceVar(&capabilityTags, "capability-tag", nil, "Capability tag to register, repeatable")
	cmd.Flags().StringSliceVar(&modelTags, "model-tag", nil, "Model tag to register, repeatable")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Minute, "Overall run timeout")
	return cmd
}

func newRunExecutorCommand() *cobra.Command {
	var (
		apiBase string
		runID   string
		timeout time.Duration
	)
	cmd := &cobra.Command{
		Use:     "run-executor",
		Aliases: []string{"run-adapter"},
		Short:   "Execute a prepared appfactory run via the thin executor mainline",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if runID == "" {
				return fmt.Errorf("--run-id is required")
			}
			if apiBase == "" {
				apiBase = defaultAPIBase()
			}
			runner := adapter.NewRunner(apiBase)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return runner.ExecuteRun(ctx, runID)
		},
	}
	cmd.Flags().StringVar(&apiBase, "api-base", "", "Internal API base URL, defaulting to local launcher backend")
	cmd.Flags().StringVar(&runID, "run-id", "", "Run ID to execute")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Overall thin executor timeout")
	return cmd
}

func defaultAPIBase() string {
	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", launcherconfig.DefaultPort),
	}
	return u.String()
}

func newRunExecutorOnceCommand() *cobra.Command {
	var (
		apiBase        string
		inputPath      string
		executorImage  string
		builderID      string
		displayName    string
		builderImage   string
		capabilityTags []string
		modelTags      []string
		timeout        time.Duration
	)
	cmd := &cobra.Command{
		Use:     "run-executor-once",
		Aliases: []string{"run-once"},
		Short:   "Register a builder, allocate a worker, create a run, and execute it once via thin executor",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(inputPath) == "" {
				return fmt.Errorf("--input is required")
			}
			inputData, err := os.ReadFile(inputPath)
			if err != nil {
				return fmt.Errorf("read input file: %w", err)
			}
			var input appruns.BuildInput
			if err := json.Unmarshal(inputData, &input); err != nil {
				return fmt.Errorf("decode input file: %w", err)
			}
			input.ContextSourceDir = filepath.Dir(inputPath)
			if strings.TrimSpace(executorImage) != "" {
				input.ExecutorImage = strings.TrimSpace(executorImage)
			}
			runResult, err := executeRunOnce(cmd.Context(), runOnceOptions{
				APIBase:        apiBase,
				Input:          input,
				BuilderID:      builderID,
				DisplayName:    displayName,
				BuilderImage:   builderImage,
				CapabilityTags: capabilityTags,
				ModelTags:      modelTags,
				Timeout:        timeout,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "run_id=%s\nworker_id=%s\n", runResult.RunID, runResult.WorkerID); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&apiBase, "api-base", "", "Internal API base URL, defaulting to local launcher backend")
	cmd.Flags().StringVar(&inputPath, "input", "", "Path to builder-input.json")
	cmd.Flags().StringVar(&executorImage, "executor-image", "", "Optional container image used to execute launch commands")
	cmd.Flags().StringVar(&builderID, "builder-id", "", "Builder ID used for this run")
	cmd.Flags().StringVar(&displayName, "display-name", "", "Builder display name")
	cmd.Flags().StringVar(&builderImage, "builder-image", "", "Builder worker image label to register")
	cmd.Flags().StringSliceVar(&capabilityTags, "capability-tag", nil, "Capability tag to register, repeatable")
	cmd.Flags().StringSliceVar(&modelTags, "model-tag", nil, "Model tag to register, repeatable")
	cmd.Flags().DurationVar(&timeout, "timeout", 45*time.Minute, "Overall thin executor single-run timeout")
	return cmd
}

type requirementCompileOptions struct {
	Requirement     string
	RequirementFile string
	OutputDir       string
	Title           string
	JobID           string
	PRDID           string
	TemplateID      string
	ExecutorImage   string
	RealBuild       bool
}

type runOnceOptions struct {
	APIBase        string
	Input          appruns.BuildInput
	BuilderID      string
	DisplayName    string
	BuilderImage   string
	CapabilityTags []string
	ModelTags      []string
	Timeout        time.Duration
}

type runOnceResult struct {
	RunID    string
	WorkerID string
}

func compileRequirementBundle(options requirementCompileOptions) (appprepare.Bundle, error) {
	if strings.TrimSpace(options.OutputDir) == "" {
		return appprepare.Bundle{}, fmt.Errorf("--output-dir is required")
	}
	if strings.TrimSpace(options.Requirement) == "" && strings.TrimSpace(options.RequirementFile) == "" {
		return appprepare.Bundle{}, fmt.Errorf("either --requirement or --requirement-file is required")
	}
	if strings.TrimSpace(options.Requirement) != "" && strings.TrimSpace(options.RequirementFile) != "" {
		return appprepare.Bundle{}, fmt.Errorf("--requirement and --requirement-file are mutually exclusive")
	}
	requirementText := options.Requirement
	requirementSource := "inline:requirement"
	if strings.TrimSpace(options.RequirementFile) != "" {
		data, err := os.ReadFile(options.RequirementFile)
		if err != nil {
			return appprepare.Bundle{}, fmt.Errorf("read requirement file: %w", err)
		}
		requirementText = string(data)
		requirementSource = options.RequirementFile
	}
	return appprepare.Compile(appprepare.Request{
		RequirementText:   requirementText,
		RequirementSource: requirementSource,
		TitleHint:         options.Title,
		JobID:             options.JobID,
		PRDID:             options.PRDID,
		TemplateID:        options.TemplateID,
		ExecutorImage:     options.ExecutorImage,
		RealBuild:         options.RealBuild,
	})
}

func executeRunOnce(parent context.Context, options runOnceOptions) (runOnceResult, error) {
	if strings.TrimSpace(options.BuilderID) == "" {
		return runOnceResult{}, fmt.Errorf("--builder-id is required")
	}
	if options.DisplayName == "" {
		options.DisplayName = options.BuilderID
	}
	if options.BuilderImage == "" {
		options.BuilderImage = strings.TrimSpace(options.Input.ExecutorImage)
	}
	if options.BuilderImage == "" {
		options.BuilderImage = "picoclaw/appfactory-builder:local"
	}
	if options.APIBase == "" {
		options.APIBase = defaultAPIBase()
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 45 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	client := newInternalAPIClient(options.APIBase)
	if err := client.registerBuilder(ctx, builderRegistrationRequest{
		BuilderID:      options.BuilderID,
		DisplayName:    options.DisplayName,
		CapabilityTags: options.CapabilityTags,
		ModelTags:      options.ModelTags,
		WorkerProfile: workerProfilePayload{
			Image: options.BuilderImage,
		},
	}); err != nil {
		return runOnceResult{}, err
	}
	allocation, err := client.allocateWorker(ctx, options.Input.JobID)
	if err != nil {
		return runOnceResult{}, err
	}
	createdRun, err := client.createBuildRun(ctx, allocation.WorkerID, allocation.LeaseID, options.Input)
	if err != nil {
		return runOnceResult{}, err
	}
	runner := adapter.NewRunner(options.APIBase)
	if err := runner.ExecuteRun(ctx, createdRun.RunID); err != nil {
		return runOnceResult{}, err
	}
	run, err := client.getBuildRun(ctx, createdRun.RunID)
	if err != nil {
		return runOnceResult{}, err
	}
	if run.Status != appruns.StatusCompleted {
		return runOnceResult{RunID: createdRun.RunID, WorkerID: createdRun.WorkerID}, fmt.Errorf("run_id=%s status=%s summary=%s", createdRun.RunID, run.Status, strings.TrimSpace(run.FailureSummary))
	}
	return runOnceResult{RunID: createdRun.RunID, WorkerID: createdRun.WorkerID}, nil
}

type internalAPIClient struct {
	baseURL string
	client  *httpClient
}

type httpClient struct {
	inner *http.Client
}

type builderRegistrationRequest struct {
	BuilderID      string               `json:"builder_id"`
	DisplayName    string               `json:"display_name"`
	CapabilityTags []string             `json:"capability_tags,omitempty"`
	ModelTags      []string             `json:"model_tags,omitempty"`
	WorkerProfile  workerProfilePayload `json:"worker_profile"`
}

type workerProfilePayload struct {
	Image string `json:"image"`
}

type allocationResponse struct {
	WorkerID string `json:"worker_id"`
	LeaseID  string `json:"lease_id"`
}

type createRunResponse struct {
	RunID    string `json:"run_id"`
	WorkerID string `json:"worker_id"`
}

func newInternalAPIClient(baseURL string) *internalAPIClient {
	return &internalAPIClient{
		baseURL: baseURL,
		client:  &httpClient{inner: &http.Client{Timeout: 30 * time.Second}},
	}
}

func (client *internalAPIClient) registerBuilder(ctx context.Context, payload builderRegistrationRequest) error {
	return client.doJSON(ctx, "POST", "/internal/v1/builders:register", payload, nil)
}

func (client *internalAPIClient) allocateWorker(ctx context.Context, jobID string) (allocationResponse, error) {
	var response allocationResponse
	err := client.doJSON(ctx, "POST", "/internal/v1/workers:allocate", map[string]any{"job_id": jobID}, &response)
	return response, err
}

func (client *internalAPIClient) createBuildRun(ctx context.Context, workerID, leaseID string, input appruns.BuildInput) (createRunResponse, error) {
	var response createRunResponse
	err := client.doJSON(ctx, "POST", "/internal/v1/build-runs", map[string]any{
		"worker_id":     workerID,
		"lease_id":      leaseID,
		"builder_input": input,
	}, &response)
	return response, err
}

func (client *internalAPIClient) getBuildRun(ctx context.Context, runID string) (appruns.RunRecord, error) {
	var response appruns.RunRecord
	err := client.doJSON(ctx, "GET", "/internal/v1/build-runs/"+runID, nil, &response)
	return response, err
}

func (client *internalAPIClient) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.client.inner.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var failure map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		return fmt.Errorf("request %s %s failed: status=%d body=%v", method, path, resp.StatusCode, failure)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
