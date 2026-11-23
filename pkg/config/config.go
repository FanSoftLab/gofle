// Package config manages application configuration, including settings persistence,
// recent files tracking, SSH credential encryption, and session management.
//
// The config package is the single source of truth for persisted application state.
// It handles loading/saving configuration from JSON files, managing recent log file
// history, and securing SSH credentials using AES-GCM encryption.
package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

// SSHConfigSummary is a lightweight, serializable summary of a remote (SSH or
// FTP) connection embedded into recent-file entries so they can be re-opened
// directly. The password (and key material) is stored in plaintext here on
// purpose: this is a local desktop application and re-opening from history
// requires the credentials. The value originates from an AES-GCM encrypted
// field in the SSH history store and is only decrypted for the local user.
//
// Protocol selects the remote backend ("ssh" or "ftp"). It is omitted from JSON
// when empty, and an empty value MUST be treated as "ssh" by consumers so that
// recent entries persisted before FTP support keep working unchanged.
type SSHConfigSummary struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	RemoteFilePath string `json:"remote_file_path"`
	AuthType       string `json:"auth_type"`
	Password       string `json:"password,omitempty"`
	KeyPath        string `json:"key_path,omitempty"`
	KeyPassphrase  string `json:"key_passphrase,omitempty"`
	// Protocol is "ssh" or "ftp"; empty (legacy entries) means "ssh".
	Protocol string `json:"protocol,omitempty"`
	// TLSMode applies to ftp only: "" (plain), "explicit" or "implicit".
	// Ignored for ssh.
	TLSMode string `json:"tls_mode,omitempty"`
}

// RecentFileItem represents a single entry in the "recently opened" list.
// Local files have IsSSH=false and SSH=nil; SSH-backed entries carry an
// SSHConfigSummary describing how to reconnect.
type RecentFileItem struct {
	Path       string            `json:"path"`
	FileSize   int64             `json:"file_size"`
	LastOpened string            `json:"last_opened"`
	IsSSH      bool              `json:"is_ssh"`
	SSH        *SSHConfigSummary `json:"ssh,omitempty"`
}

// SavedTabItem is a persisted snapshot of an open tab, used to restore the
// session on the next launch when "save tabs on exit" is enabled. ID is the
// runtime tab id: it is restored verbatim so the persisted group membership and
// top-level order keep referring to the right tabs across restarts.
type SavedTabItem struct {
	ID      string            `json:"id"`
	Title   string            `json:"title"`
	Path    string            `json:"path"`
	IsSSH   bool              `json:"is_ssh"`
	SSH     *SSHConfigSummary `json:"ssh,omitempty"`
	GroupID string            `json:"group_id,omitempty"`
}

// SavedGroupItem is a persisted tab group. MemberIDs is the ordered list of tab
// ids inside the group.
type SavedGroupItem struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Color     string   `json:"color"`
	Collapsed bool     `json:"collapsed"`
	MemberIDs []string `json:"member_ids"`
}

// SavedTopEntry is one slot of the tab strip: either a standalone tab or a
// whole group. Type is "tab" or "group".
type SavedTopEntry struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// SavedSession is the full persisted tab-bar structure (order, groups, active
// tab). The frontend is the single source of truth and pushes it via
// SaveSession; Go never rebuilds it from its own runtime tab map (that would
// lose groups and ordering).
type SavedSession struct {
	ActiveTabID string           `json:"active_tab_id"`
	TopLevel    []SavedTopEntry  `json:"top_level"`
	Tabs        []SavedTabItem   `json:"tabs"`
	Groups      []SavedGroupItem `json:"groups"`
}

// AppConfig is the single source of truth for persisted application state.
// The exported `Mu` mutex is used directly by the App layer to guard reads of
// the RecentFiles slice / SavedSession; the package methods lock internally for
// writes.
type AppConfig struct {
	configDir  string
	SecretKey  string
	SecretSalt string // hex-encoded salt for Argon2id key derivation

	Theme             string `json:"theme"`
	Language          string `json:"language"`
	SaveTabsExit      bool   `json:"save_tabs_exit"`
	SearchPaneHeight  int    `json:"search_pane_height"`
	SearchPaneWidth   int    `json:"search_pane_width"`
	SearchOrientation string `json:"search_orientation"`
	SearchSide        string `json:"search_side"`
	// ViewMode is the CLI (and shared) layout preference:
	// "normal" | "results" | "split". Empty means "split" (default).
	ViewMode string `json:"view_mode"`
	// LogFontSize is the desktop log font size in px (shared settings.json).
	LogFontSize int `json:"log_font_size"`
	// LogFontFamily is a CSS font-family stack for the log view.
	LogFontFamily string `json:"log_font_family"`
	// LineDensity is "compact" | "normal" | "comfortable" (row spacing).
	LineDensity string `json:"line_density"`
	// WordWrap wraps long log lines instead of horizontal scrolling.
	WordWrap bool `json:"word_wrap"`
	// ShowLineNumbers toggles the gutter with 1-based line numbers.
	ShowLineNumbers bool `json:"show_line_numbers"`
	// DefaultFollow enables tail/follow when a new tab is opened.
	DefaultFollow bool `json:"default_follow"`
	// Default search option toggles applied to newly opened tabs.
	DefaultSearchCase   bool `json:"default_search_case"`
	DefaultSearchRegex  bool `json:"default_search_regex"`
	DefaultSearchInvert bool `json:"default_search_invert"`
	// DefaultSearchCombine is "and" or "or" for the secondary search field.
	DefaultSearchCombine string `json:"default_search_combine"`
	// Search context / advanced defaults (GUI).
	CtxBefore           int    `json:"ctx_before"`
	CtxAfter            int    `json:"ctx_after"`
	NearbyMode          string `json:"nearby_mode"`
	NearbyWindow        int    `json:"nearby_window"`
	ExcludeRegexDefault bool   `json:"exclude_regex_default"`
	TimeRequireStamp    bool   `json:"time_require_stamp"`
	TopLimit            int    `json:"top_limit"`
	HlPrimary           bool   `json:"hl_primary"`
	HlSecondary         bool   `json:"hl_secondary"`
	HlNearby            bool   `json:"hl_nearby"`
	HlExclude           bool   `json:"hl_exclude"`
	// MaxRecentFiles caps the recent-files list (separate from input history).
	MaxRecentFiles int `json:"max_recent_files"`
	// ConfirmCloseTab asks before closing a tab that is SSH/FTP or has an active stream.
	ConfirmCloseTab bool `json:"confirm_close_tab"`
	// StartupBehavior: "session" (restore tabs) | "empty" | "open_dialog".
	StartupBehavior string `json:"startup_behavior"`
	// Remote connection preferences (desktop GUI).
	SSHConnectTimeoutSec  int  `json:"ssh_connect_timeout_sec"`
	RemoteStallTimeoutSec int  `json:"remote_stall_timeout_sec"`
	AutoAcceptHostKey     bool `json:"auto_accept_host_key"`
	ConnMaxAttempts       int  `json:"conn_max_attempts"`
	ConnAttemptWindowMin  int  `json:"conn_attempt_window_min"`
	ConnLockoutMin        int  `json:"conn_lockout_min"`
	DefaultSSHPort        int  `json:"default_ssh_port"`
	DefaultFTPPort        int  `json:"default_ftp_port"`
	WindowWidth           int  `json:"window_width"`
	WindowHeight          int  `json:"window_height"`
	WindowMaximized       bool `json:"window_maximized"`
	MaxHistorySize        int  `json:"-"`

	RecentFiles  []RecentFileItem    `json:"-"`
	SavedSession *SavedSession       `json:"-"`
	History      map[string][]string `json:"-"`

	// recentFileCache caches the decrypted list returned by GetRecentFiles so
	// the AES-GCM decryption work runs only when RecentFiles is mutated. It is
	// guarded by Mu together with RecentFiles.
	recentFileCache      []RecentFileItem
	recentFileCacheValid bool

	// saveTimer implements debounced auto-save so rapid UI updates coalesce
	// into a single disk write.
	saveMu    sync.Mutex
	saveTimer *time.Timer

	Mu sync.RWMutex
}

// settingsFile is the on-disk shape of the settings.json file.
// Shared by the CLI viewer and the desktop app (same config directory).
// SaveTabsExit is a pointer so a missing JSON key does not silently force the
// flag to false (Go's zero value) and wipe the "save open tabs" default.
type settingsFile struct {
	Theme                 string `json:"theme"`
	Language              string `json:"language"`
	SaveTabsExit          *bool  `json:"save_tabs_exit"`
	SearchPaneHeight      int    `json:"search_pane_height"`
	WindowWidth           int    `json:"window_width"`
	WindowHeight          int    `json:"window_height"`
	SearchOrientation     string `json:"search_orientation"`
	SearchPaneWidth       int    `json:"search_pane_width"`
	SearchSide            string `json:"search_side"`
	ViewMode              string `json:"view_mode"`
	LogFontSize           int    `json:"log_font_size"`
	LogFontFamily         string `json:"log_font_family"`
	LineDensity           string `json:"line_density"`
	WordWrap              *bool  `json:"word_wrap"`
	ShowLineNumbers       *bool  `json:"show_line_numbers"`
	DefaultFollow         *bool  `json:"default_follow"`
	DefaultSearchCase     *bool  `json:"default_search_case"`
	DefaultSearchRegex    *bool  `json:"default_search_regex"`
	DefaultSearchInvert   *bool  `json:"default_search_invert"`
	DefaultSearchCombine  string `json:"default_search_combine"`
	CtxBefore             int    `json:"ctx_before"`
	CtxAfter              int    `json:"ctx_after"`
	NearbyMode            string `json:"nearby_mode"`
	NearbyWindow          int    `json:"nearby_window"`
	ExcludeRegexDefault   *bool  `json:"exclude_regex_default"`
	TimeRequireStamp      *bool  `json:"time_require_stamp"`
	TopLimit              int    `json:"top_limit"`
	HlPrimary             *bool  `json:"hl_primary"`
	HlSecondary           *bool  `json:"hl_secondary"`
	HlNearby              *bool  `json:"hl_nearby"`
	HlExclude             *bool  `json:"hl_exclude"`
	MaxRecentFiles        int    `json:"max_recent_files"`
	ConfirmCloseTab       *bool  `json:"confirm_close_tab"`
	StartupBehavior       string `json:"startup_behavior"`
	SSHConnectTimeoutSec  int    `json:"ssh_connect_timeout_sec"`
	RemoteStallTimeoutSec int    `json:"remote_stall_timeout_sec"`
	AutoAcceptHostKey     *bool  `json:"auto_accept_host_key"`
	ConnMaxAttempts       int    `json:"conn_max_attempts"`
	ConnAttemptWindowMin  int    `json:"conn_attempt_window_min"`
	ConnLockoutMin        int    `json:"conn_lockout_min"`
	DefaultSSHPort        int    `json:"default_ssh_port"`
	DefaultFTPPort        int    `json:"default_ftp_port"`
	WindowMaximized       bool   `json:"window_maximized"`
	MaxHistorySize        int    `json:"max_history_size"`
}

const (
	recentFile        = "recent.json"
	settingsFileNm    = "settings.json"
	savedTabsFile     = "saved_tabs.json"
	savedSessionFile  = "saved_session.json"
	historyFile       = "history.json"
	searchPresetsFile = "search_presets.json"
	// secretKeyFile holds the AES key used to encrypt at-rest SSH credentials.
	// It lives in its own 0600 file, separate from settings.json, so the key is
	// not co-mingled with general config and the blast radius of a settings
	// file read is reduced (B7).
	// Format: JSON with "key" (hex-encoded 32-byte key) and "salt" (hex-encoded 16-byte salt).
	// Legacy format: raw hex-encoded 32-byte key (for backward compatibility).
	secretKeyFile = "secret.key"
)

// secretKeyData represents the on-disk format of the secret key file.
type secretKeyData struct {
	Key  string `json:"key"`  // hex-encoded 32-byte master key
	Salt string `json:"salt"` // hex-encoded 16-byte salt for Argon2id
}

// AppDirName is the single, architecture-agnostic directory name used under
// the OS user config/cache roots (e.g. ~/.config/gofle). Prefer lowercase so
// the path matches the CLI binary name and stays consistent across platforms.
const AppDirName = "gofle"

// legacyAppDirName is the pre-rename config folder. DefaultDir migrates it
// once when the new path does not yet exist.
const legacyAppDirName = "GoFLE"

// DefaultDir returns the configuration directory used by LoadConfig
// (<os.UserConfigDir>/gofle), falling back to the current directory when the
// user config dir cannot be determined. It is exported so both binaries can
// place their log file beside the config before LoadConfig runs.
//
// On first run after the rename, an existing legacy <UserConfigDir>/GoFLE
// directory is renamed to gofle so settings, known_hosts and secrets are kept.
//
// Example:
//
//	fmt.Println(config.DefaultDir())
//	// Output: /home/user/.config/gofle
func DefaultDir() string {
	userConfig, err := os.UserConfigDir()
	if err != nil {
		return "."
	}

	dir := filepath.Join(userConfig, AppDirName)
	legacy := filepath.Join(userConfig, legacyAppDirName)
	if _, err := os.Stat(dir); err != nil {
		if _, legErr := os.Stat(legacy); legErr == nil {
			if renErr := os.Rename(legacy, dir); renErr != nil {
				// Fall back to the legacy path if rename fails (e.g. cross-device).
				return legacy
			}
		}
	}
	return dir
}

// LoadConfig loads (or initializes) the application configuration. A random
// secret key is generated on first run and persisted so that encrypted SSH
// passwords remain stable across restarts.
//
// The returned AppConfig is the single source of truth for application settings.
// It must be saved using Save() when modified to persist changes to disk.
//
// settings.json, recent.json, history.json and saved_session.json are loaded
// best-effort: a missing or corrupt file falls back to defaults rather than
// failing startup (corrupt files are reported via the log so silent resets of
// user data remain diagnosable).
//
// Returns an error only if the AES secret key cannot be generated or loaded.
//
// Play: https://go.dev/play/p/CONFIG_LOADCONFIG
func LoadConfig() (*AppConfig, error) {
	if _, err := os.UserConfigDir(); err != nil {
		// Degraded mode: dotfiles land in the working directory. Surface it —
		// a silently wrong config location looks like lost settings later.
		slog.Warn("cannot determine user config dir; falling back to current directory", "err", err)
	}

	appDir := DefaultDir()
	// security: the config dir holds the secret key, encrypted credentials and
	// known_hosts — restrict it to the owning user (was 0755).
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create config dir: %w", err)
	}

	cfg := &AppConfig{
		configDir:             appDir,
		Theme:                 "system",
		Language:              "system",
		SaveTabsExit:          true,
		SearchPaneHeight:      0,
		SearchPaneWidth:       0,
		SearchOrientation:     "horizontal",
		SearchSide:            "bottom",
		ViewMode:              "split", // CLI default: split view
		LogFontSize:           13,
		LogFontFamily:         "monospace",
		LineDensity:           "normal",
		WordWrap:              false,
		ShowLineNumbers:       true,
		DefaultFollow:         true,
		DefaultSearchCase:     false,
		DefaultSearchRegex:    false,
		DefaultSearchInvert:   false,
		CtxBefore:             0,
		CtxAfter:              0,
		NearbyMode:            "either",
		NearbyWindow:          10,
		ExcludeRegexDefault:   false,
		TimeRequireStamp:      false,
		TopLimit:              100,
		HlPrimary:             true,
		HlSecondary:           true,
		HlNearby:              true,
		HlExclude:             false,
		DefaultSearchCombine:  "or",
		MaxRecentFiles:        20,
		ConfirmCloseTab:       true,
		StartupBehavior:       "session",
		SSHConnectTimeoutSec:  10,
		RemoteStallTimeoutSec: 60,
		AutoAcceptHostKey:     false,
		ConnMaxAttempts:       20,
		ConnAttemptWindowMin:  5,
		ConnLockoutMin:        1,
		DefaultSSHPort:        22,
		DefaultFTPPort:        21,
		WindowWidth:           1280,
		WindowHeight:          800,
		WindowMaximized:       true,
		MaxHistorySize:        100,
	}

	// The AES secret key must be initialized BEFORE any persisted state loads:
	// migrating a legacy saved_tabs.json re-encrypts SSH credentials, which
	// requires the key. (Previously the key was created after the loads, so a
	// one-time legacy migration encrypted credentials under an empty-key-derived
	// AES key — silently corrupting them for every later decrypt.)
	if err := cfg.loadOrCreateSecretKey(); err != nil {
		return nil, fmt.Errorf("failed to initialize secret key: %w", err)
	}

	cfg.loadSettings()
	cfg.loadRecent()
	cfg.loadSavedSession()
	cfg.loadHistory()

	return cfg, nil
}

func (c *AppConfig) loadSettings() {
	path := filepath.Join(c.configDir, settingsFileNm)

	data, err := os.ReadFile(path) //nolint:gosec // user-provided config path is intended
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("failed to read settings file; using defaults", "file", settingsFileNm, "err", err)
		}

		return
	}

	var sf settingsFile
	if err := json.Unmarshal(data, &sf); err != nil {
		// A corrupt (not merely missing) file resets to defaults and the next
		// Save() overwrites it — the user must be able to diagnose that.
		slog.Warn("settings file is corrupt; using defaults", "file", settingsFileNm, "err", err)
		return
	}

	c.Theme = sf.Theme
	c.Language = sf.Language
	if sf.SaveTabsExit != nil {
		c.SaveTabsExit = *sf.SaveTabsExit
	}
	// else keep the constructor default (true) when the key is absent
	c.SearchPaneHeight = sf.SearchPaneHeight
	c.SearchPaneWidth = sf.SearchPaneWidth
	c.SearchOrientation = sf.SearchOrientation
	c.SearchSide = sf.SearchSide
	c.ViewMode = sf.ViewMode
	if c.ViewMode == "" {
		c.ViewMode = "split"
	}
	c.LogFontSize = sf.LogFontSize
	if c.LogFontSize < 11 || c.LogFontSize > 22 {
		c.LogFontSize = 13
	}
	if sf.LogFontFamily != "" {
		c.LogFontFamily = sf.LogFontFamily
	}
	if sf.LineDensity != "" {
		c.LineDensity = sf.LineDensity
	}
	if sf.WordWrap != nil {
		c.WordWrap = *sf.WordWrap
	}
	if sf.ShowLineNumbers != nil {
		c.ShowLineNumbers = *sf.ShowLineNumbers
	}
	if sf.DefaultFollow != nil {
		c.DefaultFollow = *sf.DefaultFollow
	}
	if sf.DefaultSearchCase != nil {
		c.DefaultSearchCase = *sf.DefaultSearchCase
	}
	if sf.DefaultSearchRegex != nil {
		c.DefaultSearchRegex = *sf.DefaultSearchRegex
	}
	if sf.DefaultSearchInvert != nil {
		c.DefaultSearchInvert = *sf.DefaultSearchInvert
	}
	if sf.DefaultSearchCombine == "and" || sf.DefaultSearchCombine == "or" {
		c.DefaultSearchCombine = sf.DefaultSearchCombine
	}
	if sf.CtxBefore >= 0 {
		c.CtxBefore = sf.CtxBefore
	}
	if sf.CtxAfter >= 0 {
		c.CtxAfter = sf.CtxAfter
	}
	if sf.NearbyMode != "" {
		c.NearbyMode = sf.NearbyMode
	}
	if sf.NearbyWindow > 0 {
		c.NearbyWindow = sf.NearbyWindow
	}
	if sf.ExcludeRegexDefault != nil {
		c.ExcludeRegexDefault = *sf.ExcludeRegexDefault
	}
	if sf.TimeRequireStamp != nil {
		c.TimeRequireStamp = *sf.TimeRequireStamp
	}
	if sf.TopLimit > 0 {
		c.TopLimit = sf.TopLimit
	}
	if sf.HlPrimary != nil {
		c.HlPrimary = *sf.HlPrimary
	}
	if sf.HlSecondary != nil {
		c.HlSecondary = *sf.HlSecondary
	}
	if sf.HlNearby != nil {
		c.HlNearby = *sf.HlNearby
	}
	if sf.HlExclude != nil {
		c.HlExclude = *sf.HlExclude
	}
	if sf.MaxRecentFiles > 0 {
		c.MaxRecentFiles = sf.MaxRecentFiles
	}
	if sf.ConfirmCloseTab != nil {
		c.ConfirmCloseTab = *sf.ConfirmCloseTab
	}
	if sf.StartupBehavior != "" {
		c.StartupBehavior = sf.StartupBehavior
	}
	if sf.SSHConnectTimeoutSec > 0 {
		c.SSHConnectTimeoutSec = sf.SSHConnectTimeoutSec
	}
	if sf.RemoteStallTimeoutSec > 0 {
		c.RemoteStallTimeoutSec = sf.RemoteStallTimeoutSec
	}
	if sf.AutoAcceptHostKey != nil {
		c.AutoAcceptHostKey = *sf.AutoAcceptHostKey
	}
	if sf.ConnMaxAttempts > 0 {
		c.ConnMaxAttempts = sf.ConnMaxAttempts
	}
	if sf.ConnAttemptWindowMin > 0 {
		c.ConnAttemptWindowMin = sf.ConnAttemptWindowMin
	}
	if sf.ConnLockoutMin > 0 {
		c.ConnLockoutMin = sf.ConnLockoutMin
	}
	if sf.DefaultSSHPort > 0 {
		c.DefaultSSHPort = sf.DefaultSSHPort
	}
	if sf.DefaultFTPPort > 0 {
		c.DefaultFTPPort = sf.DefaultFTPPort
	}
	c.WindowWidth = sf.WindowWidth
	c.WindowHeight = sf.WindowHeight

	c.WindowMaximized = sf.WindowMaximized
	if sf.MaxHistorySize > 0 {
		c.MaxHistorySize = sf.MaxHistorySize
	} else {
		c.MaxHistorySize = 100
	}
}

func (c *AppConfig) persistSettings() error {
	saveTabs := c.SaveTabsExit
	wordWrap := c.WordWrap
	showLN := c.ShowLineNumbers
	defFollow := c.DefaultFollow
	defCase := c.DefaultSearchCase
	defRegex := c.DefaultSearchRegex
	defInvert := c.DefaultSearchInvert
	confirmClose := c.ConfirmCloseTab
	autoHost := c.AutoAcceptHostKey
	sf := settingsFile{
		Theme:                 c.Theme,
		Language:              c.Language,
		SaveTabsExit:          &saveTabs,
		SearchPaneHeight:      c.SearchPaneHeight,
		SearchPaneWidth:       c.SearchPaneWidth,
		SearchOrientation:     c.SearchOrientation,
		SearchSide:            c.SearchSide,
		ViewMode:              c.ViewMode,
		LogFontSize:           c.LogFontSize,
		LogFontFamily:         c.LogFontFamily,
		LineDensity:           c.LineDensity,
		WordWrap:              &wordWrap,
		ShowLineNumbers:       &showLN,
		DefaultFollow:         &defFollow,
		DefaultSearchCase:     &defCase,
		DefaultSearchRegex:    &defRegex,
		DefaultSearchInvert:   &defInvert,
		MaxRecentFiles:        c.MaxRecentFiles,
		ConfirmCloseTab:       &confirmClose,
		StartupBehavior:       c.StartupBehavior,
		SSHConnectTimeoutSec:  c.SSHConnectTimeoutSec,
		RemoteStallTimeoutSec: c.RemoteStallTimeoutSec,
		AutoAcceptHostKey:     &autoHost,
		ConnMaxAttempts:       c.ConnMaxAttempts,
		ConnAttemptWindowMin:  c.ConnAttemptWindowMin,
		ConnLockoutMin:        c.ConnLockoutMin,
		DefaultSSHPort:        c.DefaultSSHPort,
		DefaultFTPPort:        c.DefaultFTPPort,
		WindowWidth:           c.WindowWidth,
		WindowHeight:          c.WindowHeight,
		WindowMaximized:       c.WindowMaximized,
		DefaultSearchCombine:  c.DefaultSearchCombine,
		CtxBefore:             c.CtxBefore,
		CtxAfter:              c.CtxAfter,
		NearbyMode:            c.NearbyMode,
		NearbyWindow:          c.NearbyWindow,
		ExcludeRegexDefault:   &c.ExcludeRegexDefault,
		TimeRequireStamp:      &c.TimeRequireStamp,
		TopLimit:              c.TopLimit,
		HlPrimary:             &c.HlPrimary,
		HlSecondary:           &c.HlSecondary,
		HlNearby:              &c.HlNearby,
		HlExclude:             &c.HlExclude,
		MaxHistorySize:        c.MaxHistorySize,
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", settingsFileNm, err)
	}
	// security: settings.json holds the AES secret key — 0600, not 0644.
	if err := os.WriteFile(filepath.Join(c.configDir, settingsFileNm), data, 0o600); err != nil {
		return fmt.Errorf("%s: %w", settingsFileNm, err)
	}

	return nil
}

func (c *AppConfig) loadRecent() {
	path := filepath.Join(c.configDir, recentFile)

	data, err := os.ReadFile(path) //nolint:gosec // user-provided config path is intended
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("failed to read recent list; using defaults", "file", recentFile, "err", err)
		}

		return
	}

	var items []RecentFileItem
	if err := json.Unmarshal(data, &items); err != nil {
		slog.Warn("recent list file is corrupt; using defaults", "file", recentFile, "err", err)
		return
	}

	c.RecentFiles = items
	c.recentFileCacheValid = false
}

func (c *AppConfig) persistRecent() error {
	data, err := json.MarshalIndent(c.RecentFiles, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", recentFile, err)
	}
	// security: recent.json holds encrypted SSH credentials — 0600, not 0644.
	if err := os.WriteFile(filepath.Join(c.configDir, recentFile), data, 0o600); err != nil {
		return fmt.Errorf("%s: %w", recentFile, err)
	}

	return nil
}

// loadHistory reads the per-namespace input history (newest first) if present.
func (c *AppConfig) loadHistory() {
	c.History = make(map[string][]string)

	data, err := os.ReadFile(filepath.Join(c.configDir, historyFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("failed to read history file; using defaults", "file", historyFile, "err", err)
		}

		return
	}

	var m map[string][]string
	if err := json.Unmarshal(data, &m); err != nil {
		slog.Warn("history file is corrupt; using defaults", "file", historyFile, "err", err)
		return
	}

	c.History = m
}

// persistHistory writes the input history atomically (temp file + rename) so an
// interrupted write can never leave a truncated file behind.
func (c *AppConfig) persistHistory() error {
	data, err := json.MarshalIndent(c.History, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", historyFile, err)
	}

	path := filepath.Join(c.configDir, historyFile)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("%s: %w", historyFile, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // cleanup of the failed temp write; the rename error is what matters
		return fmt.Errorf("%s: %w", historyFile, err)
	}

	return nil
}

// GetInputHistory returns a copy of a namespace's input history (newest first).
//
// Parameters:
//   - namespace: The history namespace (e.g., "search", "ssh_host").
//
// Returns a copy of the history slice (newest first), or nil if the namespace doesn't exist.
//
// Example:
//
//	cfg := config.LoadConfig()
//	history := cfg.GetInputHistory("search")
//	for i, v := range history {
//	    fmt.Printf("%d: %s\n", i, v)
//	}
func (c *AppConfig) GetInputHistory(namespace string) []string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	items := c.History[namespace]
	res := make([]string, len(items))
	copy(res, items)

	return res
}

// AddInputHistory prepends a value to a namespace's history, de-duplicating by
// normalized (lower-cased, trimmed) form, capping at MaxHistorySize, and
// persisting. Empty/whitespace-only values are ignored.
//
// Parameters:
//   - namespace: The history namespace (e.g., "search", "ssh_host").
//   - value: The value to add. Must not be empty or whitespace-only.
//
// Returns an error if persisting fails.
//
// Example:
//
//	cfg := config.LoadConfig()
//	cfg.AddInputHistory("search", "error timeout")
func (c *AppConfig) AddInputHistory(namespace, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	if c.History == nil {
		c.History = make(map[string][]string)
	}

	norm := strings.ToLower(value)
	existing := c.History[namespace]
	filtered := slices.DeleteFunc(existing, func(item string) bool {
		return strings.ToLower(strings.TrimSpace(item)) == norm
	})

	filtered = append([]string{value}, filtered...)
	if len(filtered) > c.MaxHistorySize {
		filtered = filtered[:c.MaxHistorySize]
	}

	c.History[namespace] = filtered

	return c.persistHistory()
}

// ClearInputHistory removes a namespace's history and persists the empty state.
//
// Parameters:
//   - namespace: The history namespace to clear.
//
// Returns an error if persisting fails.
func (c *AppConfig) ClearInputHistory(namespace string) error {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	delete(c.History, namespace)

	return c.persistHistory()
}

// RemoveInputHistoryItem removes a single value (matched by normalized
// lower-cased/trimmed form) from a namespace's history and persists the change.
// Removing the last item prunes the namespace entirely.
//
// Parameters:
//   - namespace: The history namespace.
//   - value: The value to remove. Matched by normalized (lower-cased, trimmed) form.
//
// Returns an error if persisting fails.
func (c *AppConfig) RemoveInputHistoryItem(namespace, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	if c.History == nil {
		return nil
	}

	norm := strings.ToLower(value)
	existing := c.History[namespace]

	filtered := slices.DeleteFunc(existing, func(item string) bool {
		return strings.ToLower(strings.TrimSpace(item)) == norm
	})
	if len(filtered) == 0 {
		delete(c.History, namespace)
	} else {
		c.History[namespace] = filtered
	}

	return c.persistHistory()
}

// SetMaxHistorySize updates the per-namespace history cap and persists it.
// Values below 1 are clamped to 1.
//
// Parameters:
//   - size: The new maximum history size per namespace. Must be >= 1.
//
// Returns an error if persisting fails.
func (c *AppConfig) SetMaxHistorySize(size int) error {
	if size < 1 {
		size = 1
	}

	c.Mu.Lock()
	c.MaxHistorySize = size
	c.Mu.Unlock()

	return c.persistSettings()
}

// Save persists all configuration files. It takes a write lock because the
// persist* helpers perform os.WriteFile — an RLock here would let two callers
// race on the same file (truncation / interleaved writes).
//
// Every file is persisted even when another one fails: aborting on the first
// error left later files unwritten and hid earlier failures behind a single
// message. All failures are joined and returned together.
//
// Returns an error if any file fails to persist, or nil on success.
func (c *AppConfig) Save() error {
	// Cancel a pending debounced save so we don't double-write right after.
	c.saveMu.Lock()
	if c.saveTimer != nil {
		c.saveTimer.Stop()
		c.saveTimer = nil
	}
	c.saveMu.Unlock()

	c.Mu.Lock()
	defer c.Mu.Unlock()

	return errors.Join(
		c.persistSettings(),
		c.persistRecent(),
		// Persist input history too. Previously an explicit Save() rewrote
		// settings.json + recent.json but silently dropped the in-memory History
		// map, so any accumulated input history was lost on the next restart.
		c.persistHistory(),
	)
}

// SaveDebounced schedules a full Save after a short quiet period. Rapid
// successive calls (pane resize, view-mode toggles, settings edits) coalesce
// into one disk write. Safe to call from any goroutine.
func (c *AppConfig) SaveDebounced() {
	const delay = 400 * time.Millisecond
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	if c.saveTimer != nil {
		c.saveTimer.Stop()
	}
	c.saveTimer = time.AfterFunc(delay, func() {
		if err := c.Save(); err != nil {
			slog.Warn("auto-save config failed", "err", err)
		}
	})
}

// FlushSave cancels any pending debounce timer and writes immediately.
// Call on application shutdown so the last in-memory changes are not lost.
//
// Returns an error if persisting fails, or nil on success.
func (c *AppConfig) FlushSave() error {
	c.saveMu.Lock()
	if c.saveTimer != nil {
		c.saveTimer.Stop()
		c.saveTimer = nil
	}
	c.saveMu.Unlock()
	return c.Save()
}

// encryptSSHSecrets returns a copy of s with Username, Password and
// KeyPassphrase encrypted at rest via EncryptSecret. The summary is deep-copied
// so the caller's struct keeps the live (cleartext) credentials. A nil input
// yields nil. Fields that fail to encrypt are blanked rather than left clear.
func (c *AppConfig) encryptSSHSecrets(s *SSHConfigSummary) *SSHConfigSummary { //nolint:dupl // symmetric encrypt/decrypt helpers
	if s == nil {
		return nil
	}

	cp := *s
	if enc, err := c.EncryptSecret(s.Username); err == nil {
		cp.Username = enc
	} else {
		cp.Username = ""
	}

	if enc, err := c.EncryptSecret(s.Password); err == nil {
		cp.Password = enc
	} else {
		cp.Password = ""
	}

	if enc, err := c.EncryptSecret(s.KeyPassphrase); err == nil {
		cp.KeyPassphrase = enc
	} else {
		cp.KeyPassphrase = ""
	}

	return &cp
}

// decryptSSHSecrets reverses encryptSSHSecrets. The in-memory copy is always
// cleartext so the app can reconnect; on-disk storage stays encrypted. Fields
// that fail to decrypt are left empty so the UI can prompt for re-entry — for
// every field: leaving the ciphertext in Username would send it to the SSH
// server as a literal login name.
func (c *AppConfig) decryptSSHSecrets(s *SSHConfigSummary) *SSHConfigSummary { //nolint:dupl // symmetric encrypt/decrypt helpers
	if s == nil {
		return nil
	}

	cp := *s
	if dec, err := c.DecryptSecret(s.Username); err == nil {
		cp.Username = dec
	} else {
		cp.Username = ""
	}

	if dec, err := c.DecryptSecret(s.Password); err == nil {
		cp.Password = dec
	} else {
		cp.Password = ""
	}

	if dec, err := c.DecryptSecret(s.KeyPassphrase); err == nil {
		cp.KeyPassphrase = dec
	} else {
		cp.KeyPassphrase = ""
	}

	return &cp
}

// AddRecentFile prepends a file (or SSH entry) to the recent list, de-duplicating
// by path, capping the list, and persisting. The persist error is returned so
// callers that surface persistence failures can; fire-and-forget callers may
// log it.
func (c *AppConfig) AddRecentFile(path string, size int64, isSSH bool, ssh *SSHConfigSummary) error {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	var prevSummary *SSHConfigSummary

	filtered := slices.DeleteFunc(c.RecentFiles, func(item RecentFileItem) bool {
		if item.Path == path && item.IsSSH && item.SSH != nil {
			prevSummary = item.SSH
		}

		return item.Path == path
	})

	// Store the SSH username, password and key passphrase encrypted at rest
	// (B1/B2). Deep-copy the summary so the caller's struct is never mutated —
	// it is also reused for the live tab and the saved-tab snapshot, which must
	// keep the real credentials.
	var sshEnc *SSHConfigSummary
	if ssh != nil {
		sshEnc = c.encryptSSHSecrets(ssh)
	} else if isSSH && prevSummary != nil {
		// A re-open recorded without credentials (e.g. the CLI resolving an
		// ssh:// URL) must not wipe the stored summary — otherwise the next
		// reopen loses its credentials entirely.
		sshEnc = prevSummary
	}

	newItem := RecentFileItem{
		Path:       path,
		FileSize:   size,
		LastOpened: time.Now().Format("2006-01-02 15:04:05"),
		IsSSH:      isSSH,
		SSH:        sshEnc,
	}

	c.RecentFiles = append([]RecentFileItem{newItem}, filtered...)
	maxRecent := c.MaxRecentFiles
	if maxRecent <= 0 {
		maxRecent = 20
	}
	if len(c.RecentFiles) > maxRecent {
		c.RecentFiles = c.RecentFiles[:maxRecent]
	}

	c.recentFileCacheValid = false

	return c.persistRecent()
}

// GetRecentFiles returns a copy of the recent list with SSH summaries
// DECRYPTED. Consumers (CLI list/picker, ssh:// resolution) need the plaintext
// credentials to reconnect; the ciphertext must never leak into UI strings or
// connection attempts (it used to render as the "username" in `gofle list`).
//
// Returns a slice of RecentFileItem with decrypted SSH credentials.
//
// Example:
//
//	cfg := config.LoadConfig()
//	recent := cfg.GetRecentFiles()
//	for i, f := range recent {
//	    fmt.Printf("%d: %s (SSH: %v)\n", i, f.Path, f.IsSSH)
//	}
func (c *AppConfig) GetRecentFiles() []RecentFileItem {
	c.Mu.RLock()

	if c.recentFileCacheValid {
		res := make([]RecentFileItem, len(c.recentFileCache))
		copy(res, c.recentFileCache)
		c.Mu.RUnlock()

		return res
	}

	c.Mu.RUnlock()

	c.Mu.Lock()
	defer c.Mu.Unlock()
	// Re-check: another caller may have populated the cache while we waited.
	if c.recentFileCacheValid {
		res := make([]RecentFileItem, len(c.recentFileCache))
		copy(res, c.recentFileCache)

		return res
	}

	res := make([]RecentFileItem, len(c.RecentFiles))
	copy(res, c.RecentFiles)

	for i := range res {
		res[i].SSH = c.decryptSSHSecrets(res[i].SSH)
	}

	c.recentFileCache = make([]RecentFileItem, len(res))
	copy(c.recentFileCache, res)
	c.recentFileCacheValid = true

	return res
}

// RemoveRecentFile removes a recent entry by path. Returns true if something
// was removed, together with the persist error (nil when nothing changed).
//
// Parameters:
//   - path: The path of the recent entry to remove.
//
// Returns a tuple of (bool, error): true if an entry was removed, and any persist error.
func (c *AppConfig) RemoveRecentFile(path string) (bool, error) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	found := false
	filtered := slices.DeleteFunc(c.RecentFiles, func(item RecentFileItem) bool {
		if item.Path == path {
			found = true
			return true
		}

		return false
	})

	if !found {
		return false, nil
	}

	c.RecentFiles = filtered
	c.recentFileCacheValid = false

	return true, c.persistRecent()
}

// ClearRecentFiles empties the recent list and returns the persist error.
//
// Returns an error if persisting fails, or nil on success.
func (c *AppConfig) ClearRecentFiles() error {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.RecentFiles = make([]RecentFileItem, 0)
	c.recentFileCacheValid = false

	return c.persistRecent()
}

// loadSavedSession reads the persisted tab-bar session (if any). When only the
// legacy flat saved_tabs.json exists it is migrated once into the new shape:
// every tab becomes a standalone top-level entry with a synthetic id.
func (c *AppConfig) loadSavedSession() {
	c.SavedSession = nil

	data, err := os.ReadFile(filepath.Join(c.configDir, savedSessionFile))
	if err == nil {
		var s SavedSession
		if uerr := json.Unmarshal(data, &s); uerr != nil {
			slog.Warn("session file is corrupt; ignoring saved tabs", "file", savedSessionFile, "err", uerr)
			return
		}

		c.SavedSession = &s
		// A session file on disk with SaveTabsExit=false is an inconsistent
		// state (disabling the flag is supposed to delete the file). It usually
		// comes from older builds that wrote save_tabs_exit=false when the key
		// was missing from settings.json. Re-enable so restore works again.
		if !c.SaveTabsExit && len(s.Tabs) > 0 {
			slog.Info("re-enabling save_tabs_exit because a saved session is present")
			c.SaveTabsExit = true
			if perr := c.persistSettings(); perr != nil {
				slog.Warn("failed to persist re-enabled save_tabs_exit", "err", perr)
			}
		}

		return
	}

	if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("failed to read session file; ignoring saved tabs", "file", savedSessionFile, "err", err)
	}

	c.migrateLegacySavedTabs()
}

// migrateLegacySavedTabs converts an old saved_tabs.json (flat list, no groups)
// into a SavedSession and persists it, then removes the legacy file.
func (c *AppConfig) migrateLegacySavedTabs() {
	legacyPath := filepath.Join(c.configDir, savedTabsFile)

	data, err := os.ReadFile(legacyPath) //nolint:gosec // user-provided config path is intended
	if err != nil {
		return
	}

	var items []SavedTabItem
	if err := json.Unmarshal(data, &items); err != nil {
		slog.Warn("legacy saved-tabs file is corrupt; skipping migration", "file", savedTabsFile, "err", err)
		return
	}

	s := SavedSession{
		TopLevel: make([]SavedTopEntry, 0, len(items)),
		Tabs:     make([]SavedTabItem, 0, len(items)),
		Groups:   make([]SavedGroupItem, 0),
	}
	for i, item := range items {
		if item.ID == "" {
			item.ID = fmt.Sprintf("tab_legacy_%d_%d", time.Now().UnixNano(), i)
		}

		item.GroupID = ""
		item.SSH = c.encryptSSHSecrets(item.SSH)
		s.Tabs = append(s.Tabs, item)
		s.TopLevel = append(s.TopLevel, SavedTopEntry{Type: "tab", ID: item.ID})
	}

	if len(s.Tabs) > 0 {
		s.ActiveTabID = s.Tabs[len(s.Tabs)-1].ID
	}

	c.SavedSession = &s
	// Only drop the legacy file once the new one is safely on disk: otherwise a
	// failed write would destroy the user's only copy of the saved tabs.
	if err := c.persistSavedSession(); err != nil {
		slog.Warn("failed to persist migrated session; keeping legacy saved_tabs.json", "err", err)
		return
	}

	if err := os.Remove(legacyPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Harmless (the migration re-runs idempotently next start), but the
		// leftover legacy file should not vanish from diagnostics.
		slog.Warn("failed to remove legacy saved_tabs.json after migration", "err", err)
	}
}

// persistSavedSession writes the tab-bar session to disk atomically (temp file +
// rename) so an interrupted write can never leave a truncated session behind.
func (c *AppConfig) persistSavedSession() error {
	path := filepath.Join(c.configDir, savedSessionFile)

	if c.SavedSession == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", savedSessionFile, err)
		}

		return nil
	}

	data, err := json.MarshalIndent(c.SavedSession, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", savedSessionFile, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("%s: %w", savedSessionFile, err)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // cleanup of the failed temp write; the rename error is what matters
		return fmt.Errorf("%s: %w", savedSessionFile, err)
	}

	return nil
}

// GetSavedSession returns a deep-enough copy of the persisted session for the UI
// to restore. An absent session yields a zero value with empty slices.
func (c *AppConfig) GetSavedSession() SavedSession {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	out := SavedSession{
		TopLevel: make([]SavedTopEntry, 0),
		Tabs:     make([]SavedTabItem, 0),
		Groups:   make([]SavedGroupItem, 0),
	}
	if c.SavedSession == nil {
		return out
	}

	out.ActiveTabID = c.SavedSession.ActiveTabID
	out.TopLevel = append(out.TopLevel, c.SavedSession.TopLevel...)

	out.Tabs = append(out.Tabs, c.SavedSession.Tabs...)
	for i := range out.Tabs {
		out.Tabs[i].SSH = c.decryptSSHSecrets(out.Tabs[i].SSH)
	}

	for _, g := range c.SavedSession.Groups {
		cp := g
		cp.MemberIDs = append([]string(nil), g.MemberIDs...)
		out.Groups = append(out.Groups, cp)
	}

	return out
}

// KnownHostsPath returns the path to the OpenSSH known_hosts file used to
// verify SSH server host keys (created on first acceptance).
func (c *AppConfig) KnownHostsPath() string {
	return filepath.Join(c.configDir, "known_hosts")
}

// SaveSession stores the tab-bar session pushed by the frontend. It is a no-op
// (and clears any previous snapshot) when "save tabs on exit" is disabled.
func (c *AppConfig) SaveSession(s SavedSession) error {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	if !c.SaveTabsExit {
		c.SavedSession = nil
		// A failed removal means the old snapshot resurrects on next launch,
		// contradicting the user's explicit opt-out — surface it.
		if err := os.Remove(filepath.Join(c.configDir, savedSessionFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", savedSessionFile, err)
		}

		return nil
	}

	cp := s
	cp.TopLevel = append([]SavedTopEntry(nil), s.TopLevel...)
	cp.Tabs = append([]SavedTabItem(nil), s.Tabs...)
	// security: persist SSH credentials (incl. key passphrase) encrypted at
	// rest; they are decrypted again on read by GetSavedSession.
	for i := range cp.Tabs {
		cp.Tabs[i].SSH = c.encryptSSHSecrets(cp.Tabs[i].SSH)
	}

	cp.Groups = make([]SavedGroupItem, 0, len(s.Groups))
	for _, g := range s.Groups {
		gc := g
		gc.MemberIDs = append([]string(nil), g.MemberIDs...)
		cp.Groups = append(cp.Groups, gc)
	}

	c.SavedSession = &cp

	return c.persistSavedSession()
}

// ClearSavedSession removes any persisted session snapshot. Removal failures
// are returned: a surviving snapshot would silently resurrect tabs the user
// asked to drop.
func (c *AppConfig) ClearSavedSession() error {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.SavedSession = nil

	var errs []error

	for _, name := range []string{savedSessionFile, savedTabsFile} {
		if err := os.Remove(filepath.Join(c.configDir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}

	return errors.Join(errs...)
}

// loadOrCreateSecretKey loads the AES key from secret.key (0600), migrating it
// from a legacy settings.json location if present, or generating a fresh one on
// first run. The key is kept separate from settings.json (B7).
//
// An existing-but-unreadable key file is a hard error, never an overwrite:
// regenerating would permanently destroy every SSH credential encrypted under
// the old key.
func (c *AppConfig) loadOrCreateSecretKey() error {
	path := filepath.Join(c.configDir, secretKeyFile)

	data, err := os.ReadFile(path) //nolint:gosec // user-provided config path is intended
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to read %s: %w", secretKeyFile, err)
	}

	// Try to parse as new JSON format first
	if len(data) > 0 {
		var skd secretKeyData
		if json.Unmarshal(data, &skd) == nil && skd.Key != "" && skd.Salt != "" {
			c.SecretKey = skd.Key
			return nil
		}
		// Fall back to legacy format (raw hex key)
		if k := strings.TrimSpace(string(data)); k != "" {
			c.SecretKey = k
			// Migrate to new format with a generated salt
			salt := make([]byte, 16)
			if _, err := io.ReadFull(rand.Reader, salt); err == nil {
				skd := secretKeyData{
					Key:  c.SecretKey,
					Salt: hex.EncodeToString(salt),
				}
				if b, err := json.Marshal(skd); err == nil {
					_ = os.WriteFile(path, b, 0o600)
				}
			}
			return nil
		}
	}

	if len(data) > 0 {
		slog.Warn(secretKeyFile + " exists but is empty; a new key will be generated and previously stored SSH credentials will not decrypt")
	}
	// Migrate from the old settings.json location so previously-encrypted
	// credentials stay decryptable across the upgrade.
	if k := readLegacySecretKey(c.configDir); k != "" {
		c.SecretKey = k
		// Write in new format with generated salt
		salt := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, salt); err == nil {
			skd := secretKeyData{
				Key:  c.SecretKey,
				Salt: hex.EncodeToString(salt),
			}
			if b, err := json.Marshal(skd); err == nil {
				_ = os.WriteFile(path, b, 0o600)
			}
		} else {
			// Fallback to legacy format
			if werr := os.WriteFile(path, []byte(k), 0o600); werr != nil {
				slog.Warn("failed to persist migrated secret key", "err", werr)
			}
		}

		return nil
	}

	k, err := generateSecretKey()
	if err != nil {
		return fmt.Errorf("failed to generate %s: %w", secretKeyFile, err)
	}

	c.SecretKey = k

	// Write in new format with generated salt
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err == nil {
		skd := secretKeyData{
			Key:  c.SecretKey,
			Salt: hex.EncodeToString(salt),
		}
		if b, err := json.Marshal(skd); err == nil {
			return os.WriteFile(path, b, 0o600)
		}
	}

	// Fallback to legacy format if JSON marshaling fails
	return os.WriteFile(path, []byte(c.SecretKey), 0o600)
}

// readLegacySecretKey reads the secret_key field from settings.json (the
// pre-separation location). Returns "" if absent or unreadable.
func readLegacySecretKey(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, settingsFileNm)) //nolint:gosec // user-provided config path is intended
	if err != nil {
		return ""
	}

	var sf struct {
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return ""
	}

	return strings.TrimSpace(sf.SecretKey)
}

// generateSecretKey returns a hex-encoded 32-byte key from crypto/rand for
// AES-256. An RNG failure is an error, never a fallback: deriving the key from
// a predictable source (e.g. the current time) would silently protect SSH
// passwords with a guessable key.
func generateSecretKey() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("failed to read crypto/rand: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// cachedAESKey holds a derived AES-GCM key cached by secret key hash.
// This avoids re-deriving the key (Argon2id) on every encrypt/decrypt call,
// which is significant when many SSH credentials are stored/loaded frequently.
var cachedAESKey struct {
	mu         sync.Mutex
	key        [32]byte // Argon2id derived key
	cachedRaw  string   // raw secret key for quick comparison
	cachedSalt string   // salt for quick comparison
}

// getOrCreateAESKey returns the derived AES key for the given secretKey and salt.
// The key is derived using Argon2id(secretKey, salt). The raw secret key and salt
// strings are kept for a cheap inequality check, so a cache hit performs no
// hashing at all.
func getOrCreateAESKey(secretKey, salt string) [32]byte {
	cachedAESKey.mu.Lock()
	defer cachedAESKey.mu.Unlock()

	if cachedAESKey.cachedRaw == secretKey && cachedAESKey.cachedSalt == salt {
		return cachedAESKey.key
	}

	// Argon2id parameters: time=3, memory=64MB, parallelism=4, keyLen=32
	// These are conservative defaults providing good security/performance balance.
	saltBytes, _ := hex.DecodeString(salt)
	key := argon2.IDKey([]byte(secretKey), saltBytes, 3, 64*1024, 4, 32)
	copy(cachedAESKey.key[:], key)
	cachedAESKey.cachedRaw = secretKey
	cachedAESKey.cachedSalt = salt

	return cachedAESKey.key
}

// EncryptSecret encrypts plainText with AES-GCM using a key derived from
// the config's secretKey and secretSalt (Argon2id). The output is hex-encoded
// and includes a unique random nonce.
// NOTE: AES-GCM only. CFB (or any other non-authenticated mode) must never be
// used for password material.
func (c *AppConfig) EncryptSecret(plainText string) (string, error) {
	if plainText == "" {
		return "", nil
	}

	key := getOrCreateAESKey(c.SecretKey, c.SecretSalt)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to initialize GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt: failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plainText), nil)

	return hex.EncodeToString(ciphertext), nil
}

// DecryptSecret reverses EncryptSecret. It returns an error (never panics) on
// tampered or otherwise invalid input.
func (c *AppConfig) DecryptSecret(encryptedHex string) (string, error) {
	if encryptedHex == "" {
		return "", nil
	}

	data, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", fmt.Errorf("decrypt: failed to decode hex: %w", err)
	}

	key := getOrCreateAESKey(c.SecretKey, c.SecretSalt)

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("decrypt: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("decrypt: failed to initialize GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("decrypt: ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		// GCM.Open is authenticated decryption: a failure here means the
		// ciphertext was tampered with or the key does not match.
		return "", fmt.Errorf("decrypt: failed to authenticate ciphertext: %w", err)
	}

	return string(plaintext), nil
}

// SearchPreset is a named snapshot of search + advanced filters, shared by the
// GUI (previously localStorage) and the CLI TUI via search_presets.json.
type SearchPreset struct {
	Name            string   `json:"name"`
	ExcludePatterns []string `json:"exclude_patterns,omitempty"`
	ExcludeIsRegex  bool     `json:"exclude_is_regex,omitempty"`
	NearbyQuery     string   `json:"nearby_query,omitempty"`
	NearbyMode      string   `json:"nearby_mode,omitempty"`
	NearbyWindow    int      `json:"nearby_window,omitempty"`
	Levels          []string `json:"levels,omitempty"`
	TimeFrom        string   `json:"time_from,omitempty"`
	TimeTo          string   `json:"time_to,omitempty"`
	TimeRequire     bool     `json:"time_require_stamp,omitempty"`
	LineStart       int64    `json:"line_start,omitempty"`
	LineEnd         int64    `json:"line_end,omitempty"`
	CtxBefore       int      `json:"ctx_before,omitempty"`
	CtxAfter        int      `json:"ctx_after,omitempty"`
	Query           string   `json:"query,omitempty"`
	SecondaryQuery  string   `json:"secondary_query,omitempty"`
	IsRegex         bool     `json:"is_regex,omitempty"`
	CaseSensitive   bool     `json:"case_sensitive,omitempty"`
	InvertMatch     bool     `json:"invert_match,omitempty"`
}

// GetSearchPresets loads search_presets.json from the config directory.
func (c *AppConfig) GetSearchPresets() []SearchPreset {
	if c == nil {
		return nil
	}
	path := filepath.Join(c.configDir, searchPresetsFile)
	data, err := os.ReadFile(path) //nolint:gosec // configDir is controlled
	if err != nil {
		return nil
	}
	var presets []SearchPreset
	if err := json.Unmarshal(data, &presets); err != nil {
		slog.Warn("search presets file is corrupt; ignoring", "file", searchPresetsFile, "err", err)
		return nil
	}
	return presets
}

// AddSearchPreset appends or replaces a preset by name and persists the list.
func (c *AppConfig) AddSearchPreset(p SearchPreset) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("preset name is required")
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	presets := c.GetSearchPresets()
	found := false
	for i := range presets {
		if strings.EqualFold(presets[i].Name, p.Name) {
			presets[i] = p
			found = true
			break
		}
	}
	if !found {
		presets = append(presets, p)
	}
	return c.persistSearchPresets(presets)
}

// RemoveSearchPreset deletes a preset by name.
func (c *AppConfig) RemoveSearchPreset(name string) error {
	if c == nil {
		return fmt.Errorf("nil config")
	}
	c.Mu.Lock()
	defer c.Mu.Unlock()
	presets := c.GetSearchPresets()
	filtered := presets[:0]
	for _, p := range presets {
		if !strings.EqualFold(p.Name, name) {
			filtered = append(filtered, p)
		}
	}
	return c.persistSearchPresets(filtered)
}

func (c *AppConfig) persistSearchPresets(presets []SearchPreset) error {
	if presets == nil {
		presets = []SearchPreset{}
	}
	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: %w", searchPresetsFile, err)
	}
	if err := os.WriteFile(filepath.Join(c.configDir, searchPresetsFile), data, 0o600); err != nil {
		return fmt.Errorf("%s: %w", searchPresetsFile, err)
	}
	return nil
}

// ImportGUIPresets merges presets exported from the desktop localStorage format:
//
//	[{"name":"…","filter":{"exclude_patterns":[…],"nearby_query":"…",…}}]
//
// Unknown fields are ignored. Existing presets with the same name are replaced.
func (c *AppConfig) ImportGUIPresets(raw []byte) (int, error) {
	if c == nil {
		return 0, fmt.Errorf("nil config")
	}
	var items []struct {
		Name   string `json:"name"`
		Filter struct {
			ExcludePatterns []string `json:"exclude_patterns"`
			ExcludeIsRegex  bool     `json:"exclude_is_regex"`
			NearbyQuery     string   `json:"nearby_query"`
			NearbyMode      string   `json:"nearby_mode"`
			NearbyWindow    int      `json:"nearby_window"`
			Levels          []string `json:"levels"`
			TimeFrom        string   `json:"time_from"`
			TimeTo          string   `json:"time_to"`
			TimeRequire     bool     `json:"time_require_stamp"`
			LineStart       int64    `json:"line_start"`
			LineEnd         int64    `json:"line_end"`
			CtxBefore       int      `json:"ctx_before"`
			CtxAfter        int      `json:"ctx_after"`
			Query           string   `json:"query"`
			SecondaryQuery  string   `json:"secondary_query"`
			IsRegex         bool     `json:"is_regex"`
			CaseSensitive   bool     `json:"case_sensitive"`
			InvertMatch     bool     `json:"invert_match"`
		} `json:"filter"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0, fmt.Errorf("parse GUI presets: %w", err)
	}
	n := 0
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = "imported"
		}
		p := SearchPreset{
			Name:            name,
			ExcludePatterns: it.Filter.ExcludePatterns,
			ExcludeIsRegex:  it.Filter.ExcludeIsRegex,
			NearbyQuery:     it.Filter.NearbyQuery,
			NearbyMode:      it.Filter.NearbyMode,
			NearbyWindow:    it.Filter.NearbyWindow,
			Levels:          it.Filter.Levels,
			TimeFrom:        it.Filter.TimeFrom,
			TimeTo:          it.Filter.TimeTo,
			TimeRequire:     it.Filter.TimeRequire,
			LineStart:       it.Filter.LineStart,
			LineEnd:         it.Filter.LineEnd,
			CtxBefore:       it.Filter.CtxBefore,
			CtxAfter:        it.Filter.CtxAfter,
			Query:           it.Filter.Query,
			SecondaryQuery:  it.Filter.SecondaryQuery,
			IsRegex:         it.Filter.IsRegex,
			CaseSensitive:   it.Filter.CaseSensitive,
			InvertMatch:     it.Filter.InvertMatch,
		}
		if err := c.AddSearchPreset(p); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// NewTestConfig creates an AppConfig for testing with a temporary directory.
func NewTestConfig(t *testing.T) *AppConfig {
	t.Helper()
	dir := t.TempDir()
	cfg := &AppConfig{
		configDir:         dir,
		Theme:             "system",
		Language:          "system",
		SaveTabsExit:      true,
		SearchPaneHeight:  0,
		SearchPaneWidth:   0,
		SearchOrientation: "horizontal",
		SearchSide:        "bottom",
		ViewMode:          "split",
		WindowWidth:       1280,
		WindowHeight:      800,
		WindowMaximized:   true,
		MaxHistorySize:    100,
	}
	// Initialize secret key for encryption/decryption.
	if err := cfg.loadOrCreateSecretKey(); err != nil {
		t.Fatalf("loadOrCreateSecretKey: %v", err)
	}

	return cfg
}
