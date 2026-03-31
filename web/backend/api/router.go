package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	appadapter "github.com/sipeed/picoclaw/pkg/appfactory/adapter"
	"github.com/sipeed/picoclaw/web/backend/launcherconfig"
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
	oauthMu                 sync.Mutex
	oauthFlows              map[string]*oauthFlow
	oauthState              map[string]string
	weixinMu                sync.Mutex
	weixinFlows             map[string]*weixinFlow
	wecomMu                 sync.Mutex
	wecomFlows              map[string]*wecomFlow
	orchestratorPassMu      sync.Mutex
	orchestratorWatchMu     sync.Mutex
	orchestratorMu          sync.Mutex
	orchestratorActive      map[string]bool
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
		oauthFlows:             make(map[string]*oauthFlow),
		oauthState:             make(map[string]string),
		weixinFlows:            make(map[string]*weixinFlow),
		wecomFlows:             make(map[string]*wecomFlow),
		orchestratorActive:     make(map[string]bool),
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
	// Config CRUD
	h.registerConfigRoutes(mux)

	// Pico Channel (WebSocket chat)
	h.registerPicoRoutes(mux)

	// Gateway process lifecycle
	h.registerGatewayRoutes(mux)

	// Session history
	h.registerSessionRoutes(mux)

	// App Factory internal endpoints
	h.registerAppFactoryInternalRoutes(mux)

	// OAuth login and credential management
	h.registerOAuthRoutes(mux)

	// Model list management
	h.registerModelRoutes(mux)

	// Channel catalog (for frontend navigation/config pages)
	h.registerChannelRoutes(mux)

	// Skills and tools support/actions
	h.registerSkillRoutes(mux)
	h.registerToolRoutes(mux)

	// OS startup / launch-at-login
	h.registerStartupRoutes(mux)

	// Launcher service parameters (port/public)
	h.registerLauncherConfigRoutes(mux)

	// WeChat QR login flow
	h.registerWeixinRoutes(mux)

	// WeCom QR login flow
	h.registerWecomRoutes(mux)

	if h.startupOrchestrator {
		h.StartPublicJobOrchestrator()
	}
}

// Shutdown gracefully shuts down the handler, stopping the gateway if it was started by this handler.
func (h *Handler) Shutdown() {
	_ = h.StopPublicJobOrchestratorWatch()
	h.WaitForAsyncJobs()
	h.StopGateway()
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
