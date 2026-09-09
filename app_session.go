package main

import (
	"GoFLE/pkg/config"
	"GoFLE/pkg/ftp"
	"GoFLE/pkg/remote"
	"context"
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetRecentFiles returns the recent-file list with SSH credentials decrypted so
// the UI can reopen entries. Decryption failures leave the field empty and the
// UI prompts the user to re-enter it.
//
//nolint:nestif // independent decrypt-or-blank branches are inherently nested
func (a *App) GetRecentFiles() ([]config.RecentFileItem, error) {
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()

	recent := make([]config.RecentFileItem, len(a.cfg.RecentFiles))
	for i, item := range a.cfg.RecentFiles {
		cp := item
		if cp.SSH != nil {
			// Credentials are stored encrypted; decrypt them so the recent entry
			// can be reopened. If decryption fails the field is left empty and the
			// UI prompts the user to re-enter it.
			sshCopy := *cp.SSH
			if dec, err := config.DecryptSecret(sshCopy.Username, a.cfg.SecretKey); err == nil {
				sshCopy.Username = dec
			} else {
				// Leaving the ciphertext here would send it to the SSH server
				// as a literal login name — blank like the other fields.
				sshCopy.Username = ""
			}

			if dec, err := config.DecryptSecret(sshCopy.Password, a.cfg.SecretKey); err == nil {
				sshCopy.Password = dec
			} else {
				sshCopy.Password = ""
			}

			if dec, err := config.DecryptSecret(sshCopy.KeyPassphrase, a.cfg.SecretKey); err == nil {
				sshCopy.KeyPassphrase = dec
			} else {
				sshCopy.KeyPassphrase = ""
			}

			cp.SSH = &sshCopy
		}

		recent[i] = cp
	}

	return recent, nil
}

// SaveSession persists the tab-bar structure pushed by the frontend (order,
// groups, collapsed state, active tab). The frontend is the source of truth.
func (a *App) SaveSession(s config.SavedSession) error {
	return a.cfg.SaveSession(s)
}

// GetInputHistory returns the persisted input history for a namespace.
func (a *App) GetInputHistory(namespace string) ([]string, error) {
	return a.cfg.GetInputHistory(namespace), nil
}

// AddInputHistory records a value into a namespace's input history.
func (a *App) AddInputHistory(namespace, value string) error {
	return a.cfg.AddInputHistory(namespace, value)
}

// ClearInputHistory removes a namespace's input history.
func (a *App) ClearInputHistory(namespace string) error {
	return a.cfg.ClearInputHistory(namespace)
}

// RemoveInputHistoryItem removes a single entry from a namespace's history.
func (a *App) RemoveInputHistoryItem(namespace, value string) error {
	return a.cfg.RemoveInputHistoryItem(namespace, value)
}

// SetMaxHistorySize updates the cap on the number of retained history entries.
func (a *App) SetMaxHistorySize(size int) error {
	return a.cfg.SetMaxHistorySize(size)
}

// GetSavedSession returns the persisted tab-bar structure for restore on start.
func (a *App) GetSavedSession() (config.SavedSession, error) {
	return a.cfg.GetSavedSession(), nil
}

// ClearSavedSession drops the persisted tab-bar structure.
func (a *App) ClearSavedSession() error {
	return a.cfg.ClearSavedSession()
}

// RestoreTab reopens a persisted tab while preserving its original tab id, so
// the restored group membership / strip order stays valid.
func (a *App) RestoreTab(item config.SavedTabItem) (*FileStatsResponse, error) {
	if item.IsSSH {
		if item.SSH == nil {
			return nil, fmt.Errorf("saved SSH tab %q has no connection details", item.ID)
		}

		summary := *item.SSH
		proto := summary.Protocol
		if proto == "" {
			proto = remote.ProtocolSSH
		}

		// FTP/FTPS tabs are persisted with IsSSH=true (shared remote tab shape)
		// and Protocol="ftp". Route them through the FTP opener instead of SSH.
		if proto == remote.ProtocolFTP {
			ftpCfg := ftp.Config{
				Host:           summary.Host,
				Port:           summary.Port,
				Username:       summary.Username,
				AuthType:       summary.AuthType,
				Password:       summary.Password,
				KeyPath:        summary.KeyPath,
				KeyPassphrase:  summary.KeyPassphrase,
				RemoteFilePath: summary.RemoteFilePath,
				TLSMode:        summary.TLSMode,
				KnownHostsPath: resolveFTPCertStore(remote.Config{
					Protocol:       remote.ProtocolFTP,
					Host:           summary.Host,
					Port:           summary.Port,
					KnownHostsPath: a.cfg.KnownHostsPath(),
				}),
			}
			displayPath := remotePathDisplay(remote.ProtocolFTP, ftpCfg.Host, ftpCfg.Port, ftpCfg.RemoteFilePath)
			return a.openFTPStream(ftpCfg, displayPath, &summary, item.ID)
		}

		sshCfg := SSHConfig{
			Host:           summary.Host,
			Port:           summary.Port,
			Username:       summary.Username,
			AuthType:       summary.AuthType,
			Password:       summary.Password,
			KeyPath:        summary.KeyPath,
			KeyPassphrase:  summary.KeyPassphrase,
			RemoteFilePath: summary.RemoteFilePath,
		}
		sshPathDisplay := fmt.Sprintf("ssh://%s:%d%s", sshCfg.Host, sshCfg.Port, sshCfg.RemoteFilePath)

		return a.openSSHStream(sshCfg, sshPathDisplay, &summary, item.ID)
	}

	return a.openFileWithID(item.Path, item.ID)
}

// RemoveRecentFile deletes a single recent entry by path and persists the
// change. Returns true if an entry was removed; persistence failures are
// propagated to the caller.
func (a *App) RemoveRecentFile(path string) (bool, error) {
	return a.cfg.RemoveRecentFile(path)
}

// ClearRecentFiles empties the entire recent list and persists the change.
func (a *App) ClearRecentFiles() (bool, error) {
	if err := a.cfg.ClearRecentFiles(); err != nil {
		return false, err
	}

	return true, nil
}

// OpenFileDialog shows the native file picker and opens the chosen log.
func (a *App) OpenFileDialog() (*FileStatsResponse, error) {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Відкрити лог-файл",
		Filters: []runtime.FileFilter{
			{DisplayName: "Log Files (*.log, *.txt)", Pattern: "*.log;*.txt;*.out"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil || file == "" {
		return nil, err
	}

	return a.OpenFile(file)
}

// OpenMultipleFilesDialog shows the native multi-file picker and opens all
// selected logs (failures are skipped individually).
func (a *App) OpenMultipleFilesDialog() ([]*FileStatsResponse, error) {
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Відкрити лог-файли",
		Filters: []runtime.FileFilter{
			{DisplayName: "Log Files (*.log, *.txt)", Pattern: "*.log;*.txt;*.out"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil || len(files) == 0 {
		return nil, err
	}

	return a.OpenFiles(files)
}

// SaveSettings persists theme, language, and the save-tabs-on-exit flag. When
// saveTabs is false, any previously persisted session snapshot is dropped
// immediately so a disabled preference cannot resurrect stale tabs.
func (a *App) SaveSettings(theme, language string, saveTabs bool) error {
	a.cfg.Mu.Lock()
	a.cfg.Theme = theme
	a.cfg.Language = language
	a.cfg.SaveTabsExit = saveTabs
	a.cfg.Mu.Unlock()

	var clearErr error
	if !saveTabs {
		// Drop any previously persisted session snapshot immediately.
		clearErr = a.cfg.ClearSavedSession()
	}

	// Join both failures: dropping the snapshot and saving the settings are
	// independent operations, so one failing must not hide the other.
	return errors.Join(clearErr, a.cfg.Save())
}

// GetSettings returns the current persisted settings as a map for the frontend
// (theme, language, save-tabs flag, pane sizes, history cap, startup args).
func (a *App) GetSettings() (map[string]any, error) {
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()

	return map[string]any{
		"theme":               a.cfg.Theme,
		"language":            a.cfg.Language,
		"save_tabs_exit":      a.cfg.SaveTabsExit,
		"search_pane_height":  a.cfg.SearchPaneHeight,
		"search_pane_width":   a.cfg.SearchPaneWidth,
		"search_orientation":  a.cfg.SearchOrientation,
		"search_side":         a.cfg.SearchSide,
		"view_mode":          a.cfg.ViewMode,
		"log_font_size":      a.cfg.LogFontSize,
		"max_history_size":   a.cfg.MaxHistorySize,
		"startup_args":       a.startupArgs,
	}, nil
}

// SaveSearchPaneHeight persists the search-results panel height (used by the
// horizontal orientation). Returns an error if persistence fails.
func (a *App) SaveSearchPaneHeight(height int) error {
	a.cfg.Mu.Lock()
	a.cfg.SearchPaneHeight = height
	a.cfg.Mu.Unlock()
	a.cfg.SaveDebounced()
	return nil
}

// GetSearchPaneHeight returns the persisted search-results panel height.
func (a *App) GetSearchPaneHeight() (int, error) {
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()

	return a.cfg.SearchPaneHeight, nil
}

// SaveSearchPaneWidth persists the search-results panel thickness for the
// vertical (side-by-side) orientation. The horizontal orientation keeps using
// SaveSearchPaneHeight so existing saved values are preserved.
func (a *App) SaveSearchPaneWidth(width int) error {
	a.cfg.Mu.Lock()
	a.cfg.SearchPaneWidth = width
	a.cfg.Mu.Unlock()
	a.cfg.SaveDebounced()
	return nil
}

// GetSearchPaneWidth returns the persisted search-results panel width (used by
// the vertical orientation).
func (a *App) GetSearchPaneWidth() (int, error) {
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()

	return a.cfg.SearchPaneWidth, nil
}

// SaveSearchPaneLayout persists the search-results panel orientation
// ("horizontal" or "vertical") and side ("top", "bottom", "left", "right").
// The panel thickness is stored separately via SaveSearchPaneHeight.
func (a *App) SaveSearchPaneLayout(orientation, side string) error {
	a.cfg.Mu.Lock()

	switch orientation {
	case "vertical":
		a.cfg.SearchOrientation = "vertical"
	case "horizontal", "":
		a.cfg.SearchOrientation = "horizontal"
	default:
		a.cfg.Mu.Unlock()
		return fmt.Errorf("invalid search orientation: %q", orientation)
	}

	switch side {
	case "top", "bottom", "left", "right":
		a.cfg.SearchSide = side
	default:
		a.cfg.SearchSide = "bottom"
	}

	a.cfg.Mu.Unlock()
	a.cfg.SaveDebounced()
	return nil
}

// SaveLogFontSize persists the desktop log font size (px) into the shared config.
func (a *App) SaveLogFontSize(px int) error {
	if px < 11 {
		px = 11
	}
	if px > 22 {
		px = 22
	}
	a.cfg.Mu.Lock()
	a.cfg.LogFontSize = px
	a.cfg.Mu.Unlock()
	a.cfg.SaveDebounced()
	return nil
}

// GetSearchPaneLayout returns the persisted search-results panel orientation
// and side so the frontend can restore the layout on launch.
func (a *App) GetSearchPaneLayout() (map[string]string, error) {
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()

	return map[string]string{
		"orientation": a.cfg.SearchOrientation,
		"side":        a.cfg.SearchSide,
	}, nil
}

// GetWindowGeometry returns the persisted window width, height, and maximized
// state for the Wails window options at startup.
func (a *App) GetWindowGeometry() (int, int, bool) {
	a.cfg.Mu.RLock()
	defer a.cfg.Mu.RUnlock()

	return a.cfg.WindowWidth, a.cfg.WindowHeight, a.cfg.WindowMaximized
}

// SaveCurrentWindowGeometry persists the window size and maximized state on
// close (skipped while minimized). Called from Wails OnBeforeClose.
func (a *App) SaveCurrentWindowGeometry(ctx context.Context) {
	if runtime.WindowIsMinimised(ctx) {
		return
	}

	isMax := runtime.WindowIsMaximised(ctx)
	w, h := runtime.WindowGetSize(ctx)

	a.cfg.Mu.Lock()

	a.cfg.WindowMaximized = isMax
	if !isMax && w > 200 && h > 200 {
		a.cfg.WindowWidth = w
		a.cfg.WindowHeight = h
	}

	a.cfg.Mu.Unlock()
	// Flush any pending debounced writes + geometry in one pass.
	_ = a.cfg.FlushSave()
}
