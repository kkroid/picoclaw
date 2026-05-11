//go:build (!darwin && !freebsd) || cgo

package backend

import (
	"fmt"

	"fyne.io/systray"

	"github.com/sipeed/oneappfactory/pkg/logger"
	"github.com/sipeed/oneappfactory/web/backend/utils"
)

func runTray() {
	systray.Run(onReady, onExit)
}

// onReady is called when the system tray is ready
func onReady() {
	// Set icon and tooltip
	systray.SetIcon(getIcon())
	systray.SetTooltip(fmt.Sprintf(T(AppTooltip), appName))

	// Create menu items
	mOpen := systray.AddMenuItem(T(MenuOpen), T(MenuOpenTooltip))
	mAbout := systray.AddMenuItem(T(MenuAbout), T(MenuAboutTooltip))

	// Add version info under About menu
	mVersion := mAbout.AddSubMenuItem(fmt.Sprintf(T(MenuVersion), appVersion), T(MenuVersionTooltip))
	mVersion.Disable()
	mRepo := mAbout.AddSubMenuItem(T(MenuGitHub), "")
	mDocs := mAbout.AddSubMenuItem(T(MenuDocs), "")

	systray.AddSeparator()

	// Quit option
	mQuit := systray.AddMenuItem(T(MenuQuit), T(MenuQuitTooltip))

	// Handle menu clicks
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				if err := openBrowser(); err != nil {
					logger.Errorf("Failed to open browser: %v", err)
				}

			case <-mVersion.ClickedCh:
				// Version info - do nothing, just shows current version

			case <-mRepo.ClickedCh:
				if err := utils.OpenBrowser("https://github.com/sipeed/oneappfactory"); err != nil {
					logger.Errorf("Failed to open GitHub: %v", err)
				}

			case <-mDocs.ClickedCh:
				if err := utils.OpenBrowser(T(DocUrl)); err != nil {
					logger.Errorf("Failed to open docs: %v", err)
				}

			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()

	if !*noBrowser {
		// Auto-open browser after systray is ready (if not disabled)
		// Check no-browser flag via environment or pass as parameter if needed
		if err := openBrowser(); err != nil {
			logger.Errorf("Warning: Failed to auto-open browser: %v", err)
		}
	}
}

// onExit is called when the system tray is exiting
func onExit() {
	logger.Info(T(Exiting))
}
