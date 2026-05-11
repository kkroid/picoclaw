package utils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	EnvOneAppFactoryHome   = "ONEAPPFACTORY_HOME"
	EnvOneAppFactoryConfig = "ONEAPPFACTORY_CONFIG"
	EnvOneAppFactoryBinary = "ONEAPPFACTORY_BINARY"
	DefaultHomeDir         = ".appfactory"
)

// GetOneAppFactoryHome returns the OneAppFactory home directory.
// Priority: $ONEAPPFACTORY_HOME > ~/.appfactory.
func GetOneAppFactoryHome() string {
	if home := os.Getenv(EnvOneAppFactoryHome); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, DefaultHomeDir)
}

// GetDefaultConfigPath returns the default path to the OneAppFactory config file.
func GetDefaultConfigPath() string {
	if configPath := os.Getenv(EnvOneAppFactoryConfig); configPath != "" {
		return configPath
	}
	return filepath.Join(GetOneAppFactoryHome(), "config.json")
}

// FindOneAppFactoryBinary locates the oneappfactory executable.
// Search order:
//  1. ONEAPPFACTORY_BINARY environment variable (explicit override)
//  2. Same directory as the current executable
//  3. Falls back to "oneappfactory" and relies on $PATH
func FindOneAppFactoryBinary() string {
	binaryName := "oneappfactory"
	if runtime.GOOS == "windows" {
		binaryName = "oneappfactory.exe"
	}

	if p := os.Getenv(EnvOneAppFactoryBinary); p != "" {
		if info, _ := os.Stat(p); info != nil && !info.IsDir() {
			return p
		}
	}

	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), binaryName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return "oneappfactory"
}

// GetLocalIP returns the local IP address of the machine.
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

// OpenBrowser automatically opens the given URL in the default browser.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
