package backend

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/sipeed/oneappfactory/pkg/config"
	"github.com/sipeed/oneappfactory/pkg/logger"
	"github.com/sipeed/oneappfactory/web/backend/api"
	"github.com/sipeed/oneappfactory/web/backend/launcherconfig"
	"github.com/sipeed/oneappfactory/web/backend/middleware"
	"github.com/sipeed/oneappfactory/web/backend/utils"
)

const (
	appName = "OneAppFactory"

	logPath   = "logs"
	panicFile = "launcher_panic.log"
	logFile   = "launcher.log"
)

var (
	appVersion = config.Version

	server     *http.Server
	serverAddr string
	apiHandler *api.Handler

	noBrowser *bool
)

// Run starts the OneAppFactory web launcher.
func Run(args []string) error {
	flags := flag.NewFlagSet("oneappfactory-launcher", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	port := flags.String("port", "18800", "Port to listen on")
	public := flags.Bool("public", false, "Listen on all interfaces (0.0.0.0) instead of localhost only")
	noBrowser = flags.Bool("no-browser", false, "Do not auto-open browser on startup")
	lang := flags.String("lang", "", "Language: en (English) or zh (Chinese). Default: auto-detect from system locale")
	console := flags.Bool("console", false, "Console mode, no GUI")

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "OneAppFactory Launcher - Web management console\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [config.json]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  config.json    Path to the configuration file (default: ~/.appfactory/config.json)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flags.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s                          Use default config path\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s ./config.json             Specify a config file\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -public ./config.json     Allow access from other devices on the network\n", os.Args[0])
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	appHome := utils.GetOneAppFactoryHome()
	panicPath := filepath.Join(appHome, logPath, panicFile)
	panicFunc, err := logger.InitPanic(panicPath)
	if err != nil {
		return fmt.Errorf("initialize panic log: %w", err)
	}
	defer panicFunc()

	enableConsole := *console
	if !enableConsole {
		logger.SetConsoleLevel(logger.FATAL)

		launcherLogPath := filepath.Join(appHome, logPath, logFile)
		if err = logger.EnableFileLogging(launcherLogPath); err != nil {
			return fmt.Errorf("enable file logging: %w", err)
		}
		defer logger.DisableFileLogging()
	}

	logger.InfoC("web", fmt.Sprintf("%s Launcher %s starting...", appName, appVersion))
	logger.InfoC("web", fmt.Sprintf("OneAppFactory Home: %s", appHome))

	if *lang != "" {
		SetLanguage(*lang)
	}

	configPath := utils.GetDefaultConfigPath()
	if flags.NArg() > 0 {
		configPath = flags.Arg(0)
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	var explicitPort bool
	var explicitPublic bool
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "port":
			explicitPort = true
		case "public":
			explicitPublic = true
		}
	})

	launcherPath := launcherconfig.PathForAppConfig(absPath)
	launcherCfg, err := launcherconfig.Load(launcherPath, launcherconfig.Default())
	if err != nil {
		logger.ErrorC("web", fmt.Sprintf("Warning: Failed to load %s: %v", launcherPath, err))
		launcherCfg = launcherconfig.Default()
	}

	effectivePort := *port
	effectivePublic := *public
	if !explicitPort {
		effectivePort = strconv.Itoa(launcherCfg.Port)
	}
	if !explicitPublic {
		effectivePublic = launcherCfg.Public
	}

	portNum, err := strconv.Atoi(effectivePort)
	if err != nil || portNum < 1 || portNum > 65535 {
		if err == nil {
			err = errors.New("must be in range 1-65535")
		}
		return fmt.Errorf("invalid port %q: %w", effectivePort, err)
	}

	addr := "127.0.0.1:" + effectivePort
	if effectivePublic {
		addr = "0.0.0.0:" + effectivePort
	}

	mux := http.NewServeMux()
	apiHandler = api.NewHandler(absPath)
	apiHandler.SetServerOptions(portNum, effectivePublic, explicitPublic, launcherCfg.AllowedCIDRs)
	apiHandler.EnableStartupOrchestrator()
	apiHandler.RegisterRoutes(mux)

	registerEmbedRoutes(mux)

	accessControlledMux, err := middleware.IPAllowlist(launcherCfg.AllowedCIDRs, mux)
	if err != nil {
		return fmt.Errorf("invalid allowed CIDR configuration: %w", err)
	}

	handler := middleware.Recoverer(
		middleware.Logger(
			middleware.JSONContentType(accessControlledMux),
		),
	)

	if enableConsole {
		fmt.Print(utils.Banner)
		fmt.Println()
		fmt.Println("  Open the following URL in your browser:")
		fmt.Println()
		fmt.Printf("    >> http://localhost:%s <<\n", effectivePort)
		if effectivePublic {
			if ip := utils.GetLocalIP(); ip != "" {
				fmt.Printf("    >> http://%s:%s <<\n", ip, effectivePort)
			}
		}
		fmt.Println()
	}

	logger.InfoC("web", fmt.Sprintf("Server will listen on http://localhost:%s", effectivePort))
	if effectivePublic {
		if ip := utils.GetLocalIP(); ip != "" {
			logger.InfoC("web", fmt.Sprintf("Public access enabled at http://%s:%s", ip, effectivePort))
		}
	}

	serverAddr = fmt.Sprintf("http://localhost:%s", effectivePort)
	server = &http.Server{Addr: addr, Handler: handler}
	go func() {
		logger.InfoC("web", fmt.Sprintf("Server listening on %s", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server failed to start: %v", err)
		}
	}()

	defer shutdownApp()

	if enableConsole {
		if !*noBrowser {
			if err := openBrowser(); err != nil {
				logger.Errorf("Warning: Failed to auto-open browser: %v", err)
			}
		}

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		logger.Info("Shutting down...")
		return nil
	}

	runTray()
	return nil
}
