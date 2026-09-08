// Package remote defines the shared, protocol-agnostic contract used by the
// remote backends (ssh and ftp). It contains only common types — directory
// entries, the browse-only Lister, the streaming Streamer, and the unified
// Config — so that the application layer and indexer can depend on a single
// abstraction regardless of transport. It must not import pkg/ssh or pkg/ftp;
// those packages import remote and implement its interfaces, avoiding an
// import cycle.
package remote

import (
	"context"
	"time"
)

// Protocol identifies the remote transport.
type Protocol = string

// Protocol identifiers for the supported remote backends.
const (
	ProtocolSSH = "ssh"
	ProtocolFTP = "ftp"
)

// DirEntry describes one entry of a remote directory listing.
type DirEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Kind    string `json:"kind"` // "dir" | "file" | "symlink"
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"` // unix seconds
}

// Lister is a browse-only remote connection (no file streaming).
type Lister interface {
	ListDir(ctx context.Context, path string) ([]DirEntry, error)
	HomeDir(ctx context.Context) (string, error)
	Close() error
}

// Streamer produces a local temp file that an indexer can read.
type Streamer interface {
	LocalPath() string
	Downloaded() int64
	RemoteTotal() int64
	KeepTempFile()
	Close() error
}

// Config is the unified remote connection config sent from the UI/CLI.
type Config struct {
	Protocol       string `json:"protocol"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Username       string `json:"username"`
	AuthType       string `json:"auth_type"` // "password" | "key"
	Password       string `json:"password,omitempty"`
	KeyPath        string `json:"key_path,omitempty"`
	KeyPassphrase  string `json:"key_passphrase,omitempty"`
	RemoteFilePath string `json:"remote_file_path"`
	TLSMode        string `json:"tls_mode,omitempty"` // ftp: "" | "explicit" | "implicit"
	// KnownHostsPath points at the OpenSSH known_hosts file (ssh) or ftp cert
	// trust store (ftp). Empty => default location.
	KnownHostsPath string `json:"known_hosts_path,omitempty"`
	// Progress, when set, receives open-stage updates.
	Progress func(stage string, done, total int64) `json:"-"`
	// StallTimeout bounds a silent initial download.
	StallTimeout time.Duration `json:"-"`
}
