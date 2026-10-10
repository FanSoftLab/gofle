// frontend/src/wails.d.ts
export {};

declare global {
    interface Window {
        go: {
            main: {
                App: {
                    OpenFile(path: string): Promise<any>;
                    OpenFiles(paths: string[]): Promise<any[]>;
                    OpenSSHFile(cfg: any): Promise<any>;
                    AcceptSSHHost(host: string): Promise<any>;
                    OpenFileDialog(): Promise<any>;
                    OpenMultipleFilesDialog(): Promise<any[]>;
                    CloseTab(tabID: string): Promise<boolean>;
                    SetActiveTab(tabID: string): Promise<boolean>;
                    RenameTab(tabID: string, newName: string): Promise<boolean>;
                    GetSavedSession(): Promise<any>;
                    SaveSession(session: any): Promise<void>;
                    ClearSavedSession(): Promise<void>;
                    RestoreTab(item: any): Promise<any>;
                    GetLogLines(tabID: string, start: number, count: number): Promise<Array<{ index: number; content: string }>>;
                    SearchLines(tabID: string, query: string, isRegex: boolean, caseSensitive: boolean, invertMatch: boolean, secondaryQuery?: string): Promise<number>;
                    GetFilteredLines(tabID: string, start: number, count: number): Promise<Array<{ index: number; content: string }>>;
                    GetRecentFiles(): Promise<Array<{ path: string; file_size: number; last_opened: string }>>;
                    SetAutoScroll(tabID: string, enabled: boolean): void;
                    SaveSettings(theme: string, language: string, saveTabs: boolean): Promise<void>;
                    GetSettings(): Promise<{ theme: string; language: string; save_tabs_exit: boolean; search_pane_height: number }>;
                    SaveSearchPaneHeight(height: number): Promise<void>;
                    GetSearchPaneHeight(): Promise<number>;
                    ValidateRegex(pattern: string): Promise<{ is_valid: boolean; error_message: string }>;
                    SaveSearchResults(tabID: string): Promise<string>;
                    SaveFullLogAs(tabID: string): Promise<string>;
                    GetLevelStats(tabID: string): Promise<{ [key: string]: number }>;
                    GetVersion(): Promise<string>;
                };
            };
        };
        runtime: {
            EventsOn(event: string, callback: (data: any) => void): void;
            OnFileDrop(callback: (x: number, y: number, paths: string[]) => void, useDropTarget?: boolean): void;
        };
    }
}