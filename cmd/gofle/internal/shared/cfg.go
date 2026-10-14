// Package shared holds helpers common to all gofle subcommands: loading the
// shared GoFLE configuration, recording recent entries, and resolving a target
// (local path or ssh:// URL) into an indexer + search engine.
package shared

import (
	"GoFLE/pkg/config"
)

// appConfig is the cached CLI configuration, loaded once at startup and reused
// by subcommands via Config.
var appConfig *config.AppConfig

// LoadCLIConfig loads the shared GoFLE application configuration (the same
// ~/.config/gofle store the desktop app uses) and caches it for later access.
func LoadCLIConfig() (*config.AppConfig, error) {
	c, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	appConfig = c

	return c, nil
}

// Config returns the cached CLI config, loading it on first use if necessary.
func Config() (*config.AppConfig, error) {
	if appConfig != nil {
		return appConfig, nil
	}

	return LoadCLIConfig()
}

// RecordRecent records a recently opened target (local or SSH) in the shared
// recent list used by both the CLI and the GUI. The persist error is returned;
// callers that must not abort over bookkeeping log it instead.
func RecordRecent(c *config.AppConfig, target string, isSSH bool, size int64, sshSummary *config.SSHConfigSummary) error {
	return c.AddRecentFile(target, size, isSSH, sshSummary)
}

// KnownHostsPath returns the path to the OpenSSH known_hosts file used to
// verify SSH server host keys.
func KnownHostsPath(c *config.AppConfig) string {
	return c.KnownHostsPath()
}
