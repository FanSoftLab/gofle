# Go Packages API Reference

> **Content type**: Reference — public Go APIs in `pkg/`.
> **Audience**: Go developers, contributors
> **Verified**: 2026-10-29 — matches source code

## Purpose

Document public types and functions in Go backend packages for contributors and integrators.

---

## pkg/indexer

### `FileIndexer`

Indexes a file by line offsets for random access.

```go
type FileIndexer struct {
    // unexported
}

func NewFileIndexer(path string) (*FileIndexer, error)
func (i *FileIndexer) BuildIndex() error
func (i *FileIndexer) UpdateIndex() (int, error)  // returns new lines count
func (i *FileIndexer) GetLine(lineNum int) (string, error)  // 1-based
func (i *FileIndexer) GetLines(start, count int) ([]string, error)
func (i *FileIndexer) LineCount() int
func (i *FileIndexer) Size() int64
func (i *FileIndexer) Close() error
```

**Behavior**:
- Builds `[]int64` of byte offsets (one per line)
- Uses `pread` for random access — no full file load
- `UpdateIndex()` stats file, reads new bytes from last offset
- Thread-safe with `sync.RWMutex`

---

## pkg/search

### `SearchEngine`

Parallel regex/substring search over line ranges.

```go
type SearchEngine struct {
    // unexported
}

func NewSearchEngine(indexer *indexer.FileIndexer) *SearchEngine
func (e *SearchEngine) Search(req SearchRequest) (*SearchResult, error)
func (e *SearchEngine) SearchRange(req SearchRequest, start, end int) (*SearchResult, error)
```

### `SearchRequest`

```go
type SearchRequest struct {
    Query          string
    Regex          bool
    CaseSensitive  bool
    Invert         bool
    AndQuery       string
    AndLogic       string // "and" | "or"
    Range          SearchRange
    Nearby         NearbyFilter
    Levels         []string
    Time           TimeFilter
    Exclude        ExcludeFilter
    Top            bool
    ContextBefore  int
    ContextAfter   int
}
```

### `SearchRange`

```go
type SearchRange struct {
    Mode  string // "all", "cursor_down", "cursor_up", "lines", "last_n"
    Start int    // 1-based
    End   int    // 0 = EOF
    LastN int
}
```

### `NearbyFilter`

```go
type NearbyFilter struct {
    Pattern string
    Mode    string // "before", "after", "either"
    Window  int
}
```

### `TimeFilter`

```go
type TimeFilter struct {
    From            string // YYYY-MM-DD
    To              string // YYYY-MM-DD
    RequireTimestamp bool
}
```

### `ExcludeFilter`

```go
type ExcludeFilter struct {
    Patterns   []string
    AsRegex    bool
}
```

### `SearchResult`

```go
type SearchResult struct {
    Matches      []MatchLine
    TotalMatches int
    TotalLines   int
    DurationMs   int64
}
```

### `MatchLine`

```go
type MatchLine struct {
    LineNumber int
    Text       string
    Highlights []HighlightRange
}
```

---

## pkg/config

### `Config`

Manages settings, recent files, sessions, encryption.

```go
type Config struct {
    // unexported
}

func NewConfig() (*Config, error)
func (c *Config) Load() error
func (c *Config) Save() error
```

#### Settings

```go
type Settings struct {
    Theme                    string
    Language                 string
    SaveTabsExit             bool
    SearchPaneHeight         int
    WindowWidth, WindowHeight int
    WindowMaximized          bool
    FontFamily               string
    FontSize                 int
    LineDensity              string
    WordWrap                 bool
    ShowLineNumbers          bool
    ViewMode                 string
    DefaultFollow            bool
    DefaultSearchCase        bool
    DefaultSearchRegex       bool
    DefaultSearchInvert      bool
    SearchOrientation        string
    SearchSide               string
    StartupBehavior          string
    ConfirmCloseRemoteTab    bool
    MaxRecent                int
    MaxHistory               int
    SSHTimeout               int
    StallTimeout             int
    AutoAcceptHostKey        bool
    ConnMaxAttempts          int
    ConnWindow               int
    ConnLockout              int
    SecretKey                string // base64 AES-GCM key
}
```

#### Recent Files

```go
type RecentEntry struct {
    Scheme          string // "local", "ssh", "ftp"
    Path            string // local path or canonical remote key
    Host            string
    Port            int
    Username        string
    AuthType        string // "password", "key"
    KeyPath         string
    KeyPassphrase   string // encrypted
    Password        string // encrypted
    RemotePath      string
    TLSMode         string
    CanonicalKey    string // dedup key
    LastOpened      time.Time
}

func (c *Config) GetRecent() (local, ssh, ftp []RecentEntry)
func (c *Config) AddRecent(entry RecentEntry)
func (c *Config) RemoveRecent(canonicalKey string)
func (c *Config) ClearRecent()
```

#### Saved Session

```go
type SavedTab struct {
    Type        string // "local", "ssh", "ftp"
    Path        string
    Host        string
    Port        int
    Username    string
    AuthType    string
    KeyPath     string
    KeyPassphrase string // encrypted
    Password    string // encrypted
    RemotePath  string
    TLSMode     string
    Name        string
    Group       string
    ScrollPos   int
    SearchQuery string
    Follow      bool
}

type SavedSession struct {
    Tabs    []SavedTab
    Active  int
    Groups  []TabGroup
}

func (c *Config) GetSavedSession() *SavedSession
func (c *Config) SaveSession(session *SavedSession)
func (c *Config) ClearSession()
```

#### Encryption

```go
func (c *Config) Encrypt(plaintext string) (string, error)  // returns base64
func (c *Config) Decrypt(ciphertext string) (string, error)
```

**Algorithm**: AES-GCM, 256-bit key from `SecretKey` (32 bytes, base64 in settings)

#### Search Presets

```go
type SearchPreset struct {
    Name     string
    Nearby   NearbyFilter
    Levels   []string
    Time     TimeFilter
    Exclude  ExcludeFilter
    Range    SearchRange
}

func (c *Config) GetPresets() []SearchPreset
func (c *Config) SavePreset(preset SearchPreset)
func (c *Config) DeletePreset(name string)
```

---

## pkg/remote

### Interfaces

```go
type Streamer interface {
    Stream(ctx context.Context, remotePath, localPath string) error
    Close() error
}

type Lister interface {
    List(path string) ([]RemoteEntry, error)
}

type RemoteEntry struct {
    Name     string
    Path     string
    IsDir    bool
    Size     int64
    ModTime  time.Time
    Mode     string
}
```

### Config

```go
type Config struct {
    Host            string
    Port            int
    Username        string
    Password        string
    KeyPath         string
    KeyPassphrase   string
    RemotePath      string
    TLSMode         string // "none", "explicit", "implicit"
    Timeout         time.Duration
    StallTimeout    time.Duration
    InsecureAccept  bool
}
```

---

## pkg/ssh

### `Streamer`

Implements `remote.Streamer` for SSH.

```go
func NewStreamer(cfg *remote.Config) (*Streamer, error)
func (s *Streamer) Stream(ctx context.Context, remotePath, localPath string) error
func (s *Streamer) Close() error
```

**Behavior**:
- Initial sync: `cat remotePath > localTemp`
- Tail: `tail -c +N remotePath` every 2s
- Host key verification via `~/.config/gofle/known_hosts` (OpenSSH format)
- TOFU on first connect with fingerprint confirmation

### `Lister`

Implements `remote.Lister` for SSH.

```go
func NewLister(cfg *remote.Config) (*Lister, error)
func (l *Lister) List(path string) ([]remote.RemoteEntry, error)
```

**Behavior**: `sftp` subsystem, `ls -la` parsing

---

## pkg/ftp

### `Streamer`

Implements `remote.Streamer` for FTP/FTPS.

```go
func NewStreamer(cfg *remote.Config) (*Streamer, error)
func (s *Streamer) Stream(ctx context.Context, remotePath, localPath string) error
func (s *Streamer) Close() error
```

**Behavior**:
- Initial sync: `RETR` to local temp
- Tail: `SIZE` poll every 30s, `REST` + `RETR` for new bytes
- TLS modes: none, explicit (AUTH TLS), implicit (port 990)
- Cert trust: `~/.config/gofle/ftp_certs` (host:port + SHA256)

### `Lister`

Implements `remote.Lister` for FTP.

```go
func NewLister(cfg *remote.Config) (*Lister, error)
func (l *Lister) List(path string) ([]remote.RemoteEntry, error)
```

**Behavior**: `MLSD` or `LIST` parsing

### Certificate Trust

```go
func WriteCertTrust(hostPort, fingerprint string) error
func CheckCertTrust(hostPort, fingerprint string) (bool, error)
```

---

## pkg/logging

### `SetupLogging`

```go
func SetupLogging(appName string) *slog.Logger
```

**Behavior**:
- Outputs to stderr (JSON) + `gofle.log` in config dir
- Level: INFO default, DEBUG with `GOFLE_LOG_LEVEL=debug`
- Used by both desktop binary and `gofle` CLI

---

## pkg/version

```go
var (
    Version   = "dev"
    Commit    = "none"
    Date      = "unknown"
    BuiltBy   = "unknown"
)
```

Set at build time via `-ldflags` in `build.sh` and `.goreleaser.yaml`.

---

## Related Information

- [Wails Bridge](wails-bridge.md)
- [Frontend Modules](frontend-modules.md)
- [Source Code](https://github.com/sankam-nikolya/GoFLE/tree/main/pkg)