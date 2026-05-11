package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	appadapter "github.com/sipeed/oneappfactory/pkg/appfactory/adapter"
	"github.com/sipeed/oneappfactory/web/backend/launcherconfig"
)

type appFactoryRuntime struct {
	mu            sync.Mutex
	workspace     string
	builders      builderService
	runs          runService
	runnerFactory func(runsSvc runService) *appadapter.Runner
	envOnce       sync.Once
	envErr        error
}

// Handler serves HTTP API requests.
type Handler struct {
	configPath              string
	serverPort              int
	serverPublic            bool
	serverPublicExplicit    bool
	serverCIDRs             []string
	startupOrchestrator     bool
	appFactory              appFactoryRuntime
	orchestratorPassMu      sync.Mutex
	orchestratorWatchMu     sync.Mutex
	orchestratorMu          sync.Mutex
	executionCancelMu       sync.Mutex
	orchestratorActive      map[string]bool
	executionCancels        map[string]context.CancelFunc
	orchestratorInstanceID  string
	orchestratorWatchRun    bool
	orchestratorWatchStop   bool
	orchestratorWatchMode   string
	orchestratorWatchEvery  time.Duration
	orchestratorWatchStart  string
	orchestratorWatchEnd    string
	orchestratorWatchError  string
	orchestratorWatchDone   chan struct{}
	orchestratorWatchCancel context.CancelFunc
	asyncJobs               sync.WaitGroup
}

// NewHandler creates an instance of the API handler.
func NewHandler(configPath string) *Handler {
	return &Handler{
		configPath:             configPath,
		serverPort:             launcherconfig.DefaultPort,
		orchestratorActive:     make(map[string]bool),
		executionCancels:       make(map[string]context.CancelFunc),
		orchestratorInstanceID: newOrchestratorInstanceID(),
	}
}

func newOrchestratorInstanceID() string {
	return fmt.Sprintf("orchestrator-%d", time.Now().UTC().UnixNano())
}

// SetServerOptions stores current backend listen options for fallback behavior.
func (h *Handler) SetServerOptions(port int, public bool, publicExplicit bool, allowedCIDRs []string) {
	h.serverPort = port
	h.serverPublic = public
	h.serverPublicExplicit = publicExplicit
	h.serverCIDRs = append([]string(nil), allowedCIDRs...)
}

// RegisterRoutes binds all API endpoint handlers to the ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	h.registerConfigRoutes(mux)
	h.registerAppFactoryInternalRoutes(mux)
	h.registerLauncherConfigRoutes(mux)

	if h.startupOrchestrator {
		h.StartPublicJobOrchestrator()
	}
}

// Shutdown gracefully shuts down the handler and waits for background work.
func (h *Handler) Shutdown() {
	_ = h.StopPublicJobOrchestratorWatch()
	h.WaitForAsyncJobs()
}

func (h *Handler) WaitForAsyncJobs() {
	h.asyncJobs.Wait()
}

func (h *Handler) waitForAsyncJobs() {
	h.WaitForAsyncJobs()
}

func (h *Handler) EnableStartupOrchestrator() {
	h.startupOrchestrator = true
}

func (h *Handler) StartPublicJobOrchestrator() {
	h.asyncJobs.Add(1)
	go func() {
		defer h.asyncJobs.Done()
		_, _ = h.RunPublicJobOrchestratorPass("handler_startup")
	}()
}

func (h *Handler) RunPublicJobOrchestratorOnce() {
	_, _ = h.RunPublicJobOrchestratorPass("manual_once")
}
