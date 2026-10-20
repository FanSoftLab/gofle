/**
 * Browser transport for GoFLE (HTTP + WebSocket). Same UI as desktop.
 * Method names match Wails-bound App so main.ts uses one call site.
 */
const API = "/api";
const WS_PATH = "/ws";

type WSHandler = (data: any) => void;

let ws: WebSocket | null = null;
let reconnectTimer: number | null = null;
let reconnectAttempt = 0;
const MAX_RECONNECT = 10;
const BASE_DELAY = 1000;

const searchProgressHandlers: WSHandler[] = [];
const fileUpdateHandlers: WSHandler[] = [];
const sshProgressHandlers: WSHandler[] = [];
const fileErrorHandlers: WSHandler[] = [];
const genericHandlers: WSHandler[] = [];

function dispatch(msg: { type: string; data: any }) {
  genericHandlers.forEach((h) => h(msg));
  switch (msg.type) {
    case "search_progress": searchProgressHandlers.forEach((h) => h(msg.data)); break;
    case "file_updated": fileUpdateHandlers.forEach((h) => h(msg.data)); break;
    case "ssh:progress": sshProgressHandlers.forEach((h) => h(msg.data)); break;
    case "file_error": fileErrorHandlers.forEach((h) => h(msg.data)); break;
  }
}

function connectWS(): Promise<void> {
  return new Promise((resolve, reject) => {
    if (ws?.readyState === WebSocket.OPEN) { resolve(); return; }
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    ws = new WebSocket(`${proto}//${window.location.host}${WS_PATH}`);
    ws.onopen = () => { reconnectAttempt = 0; resolve(); };
    ws.onmessage = (ev) => {
      try { dispatch(JSON.parse(ev.data)); } catch (e) { console.error("WS parse:", e); }
    };
    ws.onclose = () => { ws = null; scheduleReconnect(); };
    ws.onerror = (ev) => { console.error("WS error:", ev); if (reconnectAttempt === 0) reject(ev); };
  });
}

function scheduleReconnect() {
  if (reconnectTimer !== null) return;
  const delay = Math.min(BASE_DELAY * Math.pow(2, reconnectAttempt) + Math.random() * 1000, 30000);
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null;
    reconnectAttempt++;
    if (reconnectAttempt <= MAX_RECONNECT) connectWS().catch(() => {});
  }, delay);
}

async function apiFetch(path: string, init: RequestInit = {}): Promise<any> {
  const res = await fetch(`${API}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined;
  return res.json();
}

async function downloadPOST(path: string, filename: string): Promise<string> {
  const res = await fetch(`${API}${path}`, { method: "POST" });
  if (!res.ok) throw new Error("export failed");
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url; a.download = filename;
  document.body.appendChild(a); a.click(); document.body.removeChild(a);
  URL.revokeObjectURL(url);
  return "downloaded";
}

/** Wails-compatible App facade. */
export class WebApp {
  GetVersion() { return apiFetch("/version"); }
  OpenFile(path: string) { return apiFetch("/files/open", { method: "POST", body: JSON.stringify({ path }) }); }
  OpenFiles(paths: string[]) { return apiFetch("/files/open-multiple", { method: "POST", body: JSON.stringify({ paths }) }); }
  OpenRemoteFile(cfg: any) { return apiFetch("/remote/open", { method: "POST", body: JSON.stringify(cfg) }); }
  OpenSSHFile(cfg: any) { return this.OpenRemoteFile({ ...cfg, protocol: cfg.protocol || "ssh" }); }
  ListRemoteFiles(cfg: any, path: string) { return apiFetch("/remote/list", { method: "POST", body: JSON.stringify({ ...cfg, path }) }); }
  ListSSHFiles(cfg: any, path: string) { return this.ListRemoteFiles({ ...cfg, protocol: "ssh" }, path); }
  async CloseTab(tabID: string) { return (await apiFetch(`/tabs/${encodeURIComponent(tabID)}`, { method: "DELETE" })).success; }
  async SetActiveTab(tabID: string) { return (await apiFetch(`/tabs/${encodeURIComponent(tabID)}/activate`, { method: "POST" })).success; }
  async RenameTab(tabID: string, newName: string) {
    return (await apiFetch(`/tabs/${encodeURIComponent(tabID)}`, { method: "PATCH", body: JSON.stringify({ new_name: newName }) })).success;
  }
  GetLogLines(tabID: string, start: number, count: number) {
    return apiFetch(`/tabs/${encodeURIComponent(tabID)}/lines?start=${start}&count=${count}`);
  }
  GetFilteredLines(tabID: string, start: number, count: number) {
    return apiFetch(`/tabs/${encodeURIComponent(tabID)}/filtered?start=${start}&count=${count}`);
  }
  async SearchLines(tabID: string, query: string, isRegex: boolean, caseSensitive: boolean, invertMatch: boolean, secondaryQuery?: string) {
    const r = await apiFetch(`/tabs/${encodeURIComponent(tabID)}/search`, {
      method: "POST",
      body: JSON.stringify({ query, is_regex: isRegex, case_sensitive: caseSensitive, invert_match: invertMatch, secondary: secondaryQuery || "" }),
    });
    return r.count;
  }
  async SearchLinesAdvanced(tabID: string, query: string, isRegex: boolean, caseSensitive: boolean, invertMatch: boolean, secondaryQuery: string, filterJSON: string) {
    const r = await apiFetch(`/tabs/${encodeURIComponent(tabID)}/search-advanced`, {
      method: "POST",
      body: JSON.stringify({ query, is_regex: isRegex, case_sensitive: caseSensitive, invert_match: invertMatch, secondary: secondaryQuery || "", filter_json: filterJSON || "" }),
    });
    return r.count;
  }
  SetAutoScroll(tabID: string, enabled: boolean) {
    return apiFetch(`/tabs/${encodeURIComponent(tabID)}/autoscroll`, { method: "POST", body: JSON.stringify({ enabled }) });
  }
  ValidateRegex(pattern: string) { return apiFetch("/validate-regex", { method: "POST", body: JSON.stringify({ pattern }) }); }
  GetLevelStats(tabID: string) { return apiFetch(`/tabs/${encodeURIComponent(tabID)}/level-stats`); }
  SaveSearchResults(tabID: string) { return downloadPOST(`/tabs/${encodeURIComponent(tabID)}/export-search`, "search_results.log"); }
  SaveFullLogAs(tabID: string) { return downloadPOST(`/tabs/${encodeURIComponent(tabID)}/export-full`, "log.txt"); }
  GetRecentFiles() { return apiFetch("/recent"); }
  async ClearRecentFiles() { return (await apiFetch("/recent", { method: "DELETE" }))?.success ?? true; }
  RemoveRecentFile(_path: string) { return Promise.resolve(true); }
  SaveSession(session: any) { return apiFetch("/session", { method: "POST", body: JSON.stringify(session) }); }
  GetSavedSession() { return apiFetch("/session"); }
  ClearSavedSession() { return apiFetch("/session", { method: "DELETE" }); }
  RestoreTab(item: any) { return apiFetch("/session/restore", { method: "POST", body: JSON.stringify(item) }); }
  GetSettings() { return apiFetch("/settings"); }
  SaveSettings(theme: string, language: string, saveTabs: boolean) {
    return apiFetch("/settings", { method: "POST", body: JSON.stringify({ settings: { theme, language, save_tabs_exit: saveTabs } }) });
  }
  SaveAppSettings(settings: Record<string, any>) {
    return apiFetch("/settings", { method: "POST", body: JSON.stringify({ settings }) });
  }
  async GetInputHistory(namespace: string) { return (await apiFetch(`/history/${encodeURIComponent(namespace)}`)).history || []; }
  AddInputHistory(namespace: string, value: string) {
    return apiFetch(`/history/${encodeURIComponent(namespace)}`, { method: "POST", body: JSON.stringify({ value }) });
  }
  ClearInputHistory(namespace: string) { return apiFetch(`/history/${encodeURIComponent(namespace)}`, { method: "DELETE" }); }
  OpenFileDialog(): Promise<any> {
    return new Promise((resolve, reject) => {
      const input = document.createElement("input");
      input.type = "file"; input.style.display = "none";
      input.onchange = async () => {
        const file = input.files?.[0];
        if (!file) { reject(new Error("No file selected")); return; }
        try {
          const fd = new FormData(); fd.append("file", file);
          const res = await fetch(`${API}/files/upload`, { method: "POST", body: fd });
          if (!res.ok) { const b = await res.json().catch(() => ({ error: res.statusText })); throw new Error(b.error || `HTTP ${res.status}`); }
          resolve(await res.json());
        } catch (e) { reject(e); }
      };
      document.body.appendChild(input); input.click(); document.body.removeChild(input);
    });
  }
  OpenMultipleFilesDialog(): Promise<any[]> {
    return new Promise((resolve, reject) => {
      const input = document.createElement("input");
      input.type = "file"; input.multiple = true; input.style.display = "none";
      input.onchange = async () => {
        const files = Array.from(input.files || []);
        if (!files.length) { reject(new Error("No files selected")); return; }
        try {
          const fd = new FormData(); files.forEach((f) => fd.append("files", f));
          const res = await fetch(`${API}/files/upload-multiple`, { method: "POST", body: fd });
          if (!res.ok) { const b = await res.json().catch(() => ({ error: res.statusText })); throw new Error(b.error || `HTTP ${res.status}`); }
          resolve(await res.json());
        } catch (e) { reject(e); }
      };
      document.body.appendChild(input); input.click(); document.body.removeChild(input);
    });
  }
  AcceptSSHHost(host: string) { return apiFetch("/ssh/accept-host", { method: "POST", body: JSON.stringify({ host }) }); }
  AckSSHHost(host: string) { return this.AcceptSSHHost(host).then(() => undefined); }
  AckFTPCert(host: string) { return apiFetch("/ftp/accept-cert", { method: "POST", body: JSON.stringify({ host }) }); }
  async SaveSearchPaneHeight(height: number) {
    const pane = await apiFetch("/ui/search-pane").catch(() => ({}));
    return apiFetch("/ui/search-pane", { method: "POST", body: JSON.stringify({ height, width: pane.width || 0, orientation: pane.orientation, side: pane.side }) });
  }
  async SaveSearchPaneWidth(width: number) {
    const pane = await apiFetch("/ui/search-pane").catch(() => ({}));
    return apiFetch("/ui/search-pane", { method: "POST", body: JSON.stringify({ height: pane.height || 0, width, orientation: pane.orientation, side: pane.side }) });
  }
  async SaveSearchPaneLayout(orientation: string, side: string) {
    const pane = await apiFetch("/ui/search-pane").catch(() => ({}));
    return apiFetch("/ui/search-pane", { method: "POST", body: JSON.stringify({ height: pane.height || 0, width: pane.width || 0, orientation, side }) });
  }
  async GetSearchPaneHeight() { return (await apiFetch("/ui/search-pane")).height || 0; }
  async GetSearchPaneWidth() { return (await apiFetch("/ui/search-pane")).width || 0; }
  async GetSearchPaneLayout() {
    const pane = await apiFetch("/ui/search-pane");
    return { orientation: pane.orientation || "horizontal", side: pane.side || "bottom" };
  }
  SaveLogFontSize(px: number) { return apiFetch("/ui/font-size", { method: "POST", body: JSON.stringify({ size: px }) }); }
  GetWindowGeometry() { return apiFetch("/ui/window"); }
}

export function installWebRuntime() {
  (window as any).runtime = {
    EventsOn(event: string, callback: (data: any) => void) {
      switch (event) {
        case "search_progress": searchProgressHandlers.push(callback); break;
        case "file_updated": fileUpdateHandlers.push(callback); break;
        case "ssh:progress": sshProgressHandlers.push(callback); break;
        case "file_error": fileErrorHandlers.push(callback); break;
        case "app:before-close":
          window.addEventListener("pagehide", () => callback(null));
          break;
        default:
          genericHandlers.push((msg) => { if (msg.type === event) callback(msg.data); });
      }
    },
    OnFileDrop(_cb: any, _use?: boolean) {},
  };
}

export async function bootstrapWebClient(): Promise<WebApp> {
  installWebRuntime();
  const app = new WebApp();
  (window as any).go = { main: { App: app } };
  try { await connectWS(); } catch (e) { console.warn("WebSocket connect failed, will retry:", e); }
  return app;
}

/** True when Wails bridge is not injected (pure browser). */
export function detectWebEnvironment(): boolean {
  return typeof (window as any).go === "undefined" || !(window as any).go?.main?.App;
}
