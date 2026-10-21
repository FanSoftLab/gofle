package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"GoFLE/pkg/config"
	"GoFLE/pkg/remote"

	"github.com/gorilla/websocket"
)

// runWebServer starts the HTTP+WebSocket server that serves the same frontend
// as the desktop app and exposes the App API over REST.
func runWebServer(addr string) int {
	app, err := NewApp()
	if err != nil {
		slog.Error("failed to initialize app", "err", err)
		return 1
	}
	app.webMode = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.Startup(ctx)
	defer app.Shutdown(ctx)

	mux := http.NewServeMux()
	api := &webAPI{app: app}
	api.register(mux)
	mux.Handle("/", http.FileServer(webAssets()))

	srv := &http.Server{
		Addr:              addr,
		Handler:           withCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
		// IdleTimeout reaps keep-alive connections the client abandoned
		// without a close (C4). A blanket ReadTimeout/WriteTimeout is
		// deliberately NOT set: it would kill the 512 MB multipart uploads
		// and the long-lived streaming exports; ReadHeaderTimeout already
		// bounds header reads, and bodies/exports are long-lived by design.
		IdleTimeout: 120 * time.Second,
	}

	slog.Info("GoFLE web UI listening", "addr", addr)
	fmt.Fprintf(os.Stderr, "GoFLE web UI: http://%s/\n", addr)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("web server failed", "err", err)
		return 1
	}
	return 0
}

func webAssets() http.FileSystem {
	sub, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		return http.FS(assets)
	}
	return http.FS(sub)
}

type webAPI struct {
	app *App
}

func (api *webAPI) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/version", api.handleVersion)
	mux.HandleFunc("/api/files/open", api.handleOpenFile)
	mux.HandleFunc("/api/files/open-multiple", api.handleOpenFiles)
	mux.HandleFunc("/api/files/upload", api.handleUpload)
	mux.HandleFunc("/api/files/upload-multiple", api.handleUploadMultiple)
	mux.HandleFunc("/api/remote/open", api.handleRemoteOpen)
	mux.HandleFunc("/api/remote/list", api.handleRemoteList)
	mux.HandleFunc("/api/ssh/accept-host", api.handleAcceptSSHHost)
	mux.HandleFunc("/api/ftp/accept-cert", api.handleAckFTPCert)
	mux.HandleFunc("/api/validate-regex", api.handleValidateRegex)
	mux.HandleFunc("/api/recent", api.handleRecent)
	mux.HandleFunc("/api/session", api.handleSession)
	mux.HandleFunc("/api/session/restore", api.handleRestoreTab)
	mux.HandleFunc("/api/settings", api.handleSettings)
	mux.HandleFunc("/api/ui/search-pane", api.handleSearchPane)
	mux.HandleFunc("/api/ui/font-size", api.handleFontSize)
	mux.HandleFunc("/api/ui/window", api.handleWindow)
	mux.HandleFunc("/api/history/", api.handleHistory)
	mux.HandleFunc("/api/tabs/", api.handleTabs)
	mux.HandleFunc("/ws", api.handleWS)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (api *webAPI) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	ch := api.app.hub.Subscribe()
	defer api.app.hub.Unsubscribe(ch)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (api *webAPI) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, 200, api.app.GetVersion())
}

func (api *webAPI) handleOpenFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.OpenFile(body.Path)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleOpenFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.OpenFiles(body.Paths)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func saveUpload(file io.Reader, name string) (string, error) {
	tmp, err := os.CreateTemp("", "gofle-upload-*"+filepath.Ext(name))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	dest := filepath.Join(filepath.Dir(tmp.Name()), name)
	if dest != tmp.Name() {
		if err := os.Rename(tmp.Name(), dest); err != nil {
			return tmp.Name(), nil
		}
		return dest, nil
	}
	return tmp.Name(), nil
}

func (api *webAPI) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeErr(w, 400, err)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	defer file.Close()
	path, err := saveUpload(file, hdr.Filename)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	res, err := api.app.OpenFile(path)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleUploadMultiple(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeErr(w, 400, err)
		return
	}
	files := r.MultipartForm.File["files"]
	paths := make([]string, 0, len(files))
	for _, hdr := range files {
		f, err := hdr.Open()
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		path, err := saveUpload(f, hdr.Filename)
		f.Close()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		paths = append(paths, path)
	}
	res, err := api.app.OpenFiles(paths)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleRemoteOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var cfg remote.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.OpenRemoteFile(cfg)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleRemoteList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		remote.Config
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.ListRemoteFiles(body.Config, body.Path)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleAcceptSSHHost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Host string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.AcceptSSHHost(body.Host)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleAckFTPCert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Host string `json:"host"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := api.app.AckFTPCert(body.Host); err != nil {
		writeErr(w, 400, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (api *webAPI) handleValidateRegex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Pattern string `json:"pattern"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.ValidateRegex(body.Pattern)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleRecent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := api.app.GetRecentFiles()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, res)
	case http.MethodDelete:
		ok, err := api.app.ClearRecentFiles()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"success": ok})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *webAPI) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := api.app.GetSavedSession()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, res)
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		var s config.SavedSession
		if err := json.Unmarshal(body, &s); err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := api.app.SaveSession(s); err != nil {
			writeErr(w, 400, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := api.app.ClearSavedSession(); err != nil {
			writeErr(w, 500, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *webAPI) handleRestoreTab(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	var item config.SavedTabItem
	if err := json.Unmarshal(body, &item); err != nil {
		writeErr(w, 400, err)
		return
	}
	res, err := api.app.RestoreTab(item)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	writeJSON(w, 200, res)
}

func (api *webAPI) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		res, err := api.app.GetSettings()
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, res)
	case http.MethodPost:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		var wrap struct {
			Settings map[string]any `json:"settings"`
		}
		_ = json.Unmarshal(body, &wrap)
		settings := wrap.Settings
		if settings == nil {
			settings = map[string]any{}
			_ = json.Unmarshal(body, &settings)
		}
		if err := api.app.SaveAppSettings(settings); err != nil {
			writeErr(w, 400, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *webAPI) handleSearchPane(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h, _ := api.app.GetSearchPaneHeight()
		wd, _ := api.app.GetSearchPaneWidth()
		layout, _ := api.app.GetSearchPaneLayout()
		ori, side := "", ""
		if layout != nil {
			ori = layout["orientation"]
			side = layout["side"]
		}
		writeJSON(w, 200, map[string]any{
			"height": h, "width": wd, "orientation": ori, "side": side,
		})
	case http.MethodPost:
		var body struct {
			Height      int    `json:"height"`
			Width       int    `json:"width"`
			Orientation string `json:"orientation"`
			Side        string `json:"side"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err)
			return
		}
		_ = api.app.SaveSearchPaneHeight(body.Height)
		_ = api.app.SaveSearchPaneWidth(body.Width)
		_ = api.app.SaveSearchPaneLayout(body.Orientation, body.Side)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *webAPI) handleFontSize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Size int `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := api.app.SaveLogFontSize(body.Size); err != nil {
		writeErr(w, 400, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (api *webAPI) handleWindow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ww, hh, maxed := api.app.GetWindowGeometry()
	writeJSON(w, 200, map[string]any{"width": ww, "height": hh, "maximized": maxed})
}

func (api *webAPI) handleHistory(w http.ResponseWriter, r *http.Request) {
	key := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/history/"), "/")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h, err := api.app.GetInputHistory(key)
		if err != nil {
			writeErr(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]any{"history": h})
	case http.MethodPost:
		var body struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err)
			return
		}
		if err := api.app.AddInputHistory(key, body.Value); err != nil {
			writeErr(w, 400, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := api.app.ClearInputHistory(key); err != nil {
			writeErr(w, 400, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *webAPI) handleTabs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tabs/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing tab id", http.StatusBadRequest)
		return
	}
	tabID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch {
	case action == "" && r.Method == http.MethodDelete:
		ok, err := api.app.CloseTab(tabID)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"success": ok})

	case action == "" && r.Method == http.MethodPatch:
		var body struct {
			NewName string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err)
			return
		}
		ok, err := api.app.RenameTab(tabID, body.NewName)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"success": ok})

	case action == "activate" && r.Method == http.MethodPost:
		ok, err := api.app.SetActiveTab(tabID)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]bool{"success": ok})

	case action == "lines" && r.Method == http.MethodGet:
		start, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		count, _ := strconv.ParseInt(r.URL.Query().Get("count"), 10, 64)
		res, err := api.app.GetLogLines(tabID, start, count)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, res)

	case action == "filtered" && r.Method == http.MethodGet:
		start, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		count, _ := strconv.ParseInt(r.URL.Query().Get("count"), 10, 64)
		res, err := api.app.GetFilteredLines(tabID, start, count)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, res)

	case action == "search" && r.Method == http.MethodPost:
		var body struct {
			Query         string `json:"query"`
			IsRegex       bool   `json:"is_regex"`
			CaseSensitive bool   `json:"case_sensitive"`
			InvertMatch   bool   `json:"invert_match"`
			Secondary     string `json:"secondary"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err)
			return
		}
		n, err := api.app.SearchLines(tabID, body.Query, body.IsRegex, body.CaseSensitive, body.InvertMatch, body.Secondary)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]int64{"count": n})

	case action == "search-advanced" && r.Method == http.MethodPost:
		var body struct {
			Query         string `json:"query"`
			IsRegex       bool   `json:"is_regex"`
			CaseSensitive bool   `json:"case_sensitive"`
			InvertMatch   bool   `json:"invert_match"`
			Secondary     string `json:"secondary"`
			FilterJSON    string `json:"filter_json"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err)
			return
		}
		n, err := api.app.SearchLinesAdvanced(tabID, body.Query, body.IsRegex, body.CaseSensitive, body.InvertMatch, body.Secondary, body.FilterJSON)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]int64{"count": n})

	case action == "autoscroll" && r.Method == http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, 400, err)
			return
		}
		api.app.SetAutoScroll(tabID, body.Enabled)
		w.WriteHeader(http.StatusNoContent)

	case action == "level-stats" && r.Method == http.MethodGet:
		res, err := api.app.GetLevelStats(tabID)
		if err != nil {
			writeErr(w, 400, err)
			return
		}
		writeJSON(w, 200, res)

	case action == "export-search" && r.Method == http.MethodPost:
		api.handleExportSearch(w, tabID)
	case action == "export-full" && r.Method == http.MethodPost:
		api.handleExportFull(w, tabID)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

// handleExportSearch streams the current search matches to the client, one
// output line per match in the desktop export's format ("%d: %s\n" with the
// 1-based line number).
//
// It ports the desktop export's bulk approach (exports.go SaveSearchResults)
// (C3): the (ascending) match list is walked in contiguous runs, each run is
// read with one bulk ScanLinesRetry pass instead of one pread+string per
// match, and output is buffered through a bufio.Writer instead of fmt.Fprintf
// straight into the ResponseWriter (one unbuffered syscall per line). A run
// whose bulk read fails (e.g. it raced a rotation) falls back to per-line
// reads for exactly that run, so lines unreadable by ScanLines are skipped the
// same way the previous per-line path always did. Write errors (the client
// went away mid-stream) stop the export; the status cannot be changed after
// the first byte is written, so the failure is only logged.
func (api *webAPI) handleExportSearch(w http.ResponseWriter, tabID string) {
	api.app.mu.RLock()
	tab, exists := api.app.tabs[tabID]
	api.app.mu.RUnlock()
	if !exists {
		writeErr(w, 404, ErrTabNotFound)
		return
	}
	tab.mu.RLock()
	matched := append([]int64(nil), tab.MatchedLines...)
	fileName := tab.FileName
	tab.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"search_results_%s.log\"", fileName))

	// Buffer through bufio: an unbuffered Fprintf per match issued one syscall
	// per line, which dominated the export on large match sets.
	writer := bufio.NewWriter(w)

	var writeErr error

	// Export via bulk ScanLines over contiguous runs of the (ascending) match
	// list instead of one pread+string per match — the same measured ~15x path
	// as the desktop export. Matches beyond the live total (snapshot raced a
	// rotation) are skipped, matching what GetLine would have reported.
	total := tab.Indexer.TotalLines()

	i := 0
	for i < len(matched) && writeErr == nil {
		if matched[i] >= total {
			i++

			continue
		}

		runEnd := i + 1
		for runEnd < len(matched) && matched[runEnd] == matched[runEnd-1]+1 && matched[runEnd] < total {
			runEnd++
		}

		start, count := matched[i], int64(runEnd-i)

		serr := tab.Indexer.ScanLinesRetry(start, count, func(idx int64, line []byte) bool {
			if _, e := fmt.Fprintf(writer, "%d: %s\n", idx+1, line); e != nil {
				writeErr = e
				return false
			}

			return true
		})

		if writeErr != nil {
			break
		}

		if serr != nil {
			// Chunk read failed (e.g. raced a rotation): fall back to per-line
			// reads for exactly this run; lines that still fail to read are
			// skipped, matching the previous per-line export.
			for k := i; k < runEnd; k++ {
				content, gerr := tab.Indexer.GetLine(matched[k])
				if gerr != nil {
					continue
				}

				if _, e := fmt.Fprintf(writer, "%d: %s\n", matched[k]+1, content); e != nil {
					writeErr = e
					break
				}
			}

			if writeErr != nil {
				break
			}
		}

		i = runEnd
	}

	if ferr := writer.Flush(); ferr != nil && writeErr == nil {
		writeErr = ferr
	}

	if writeErr != nil {
		// The client disconnected or the connection broke mid-stream; the
		// status is already committed, so there is nothing to report to it.
		slog.Warn("web export-search ended with a write error", "tab", tabID, "err", writeErr)
	}
}

func (api *webAPI) handleExportFull(w http.ResponseWriter, tabID string) {
	api.app.mu.RLock()
	tab, exists := api.app.tabs[tabID]
	api.app.mu.RUnlock()
	if !exists {
		writeErr(w, 404, ErrTabNotFound)
		return
	}
	tab.mu.RLock()
	fileName := tab.FileName
	path := tab.FilePath
	tab.mu.RUnlock()

	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
	_, _ = io.Copy(w, f)
}
