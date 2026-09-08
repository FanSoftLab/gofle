import { main, config } from './wailsjs/go/models';
import type { VirtualScroller, LogLineData } from './virtual_scroller';

export interface TabData {
    id: string;
    filePath: string;
    fileName: string;
    /** User-editable tab title. Falls back to fileName when empty. */
    title?: string;
    groupID?: string | null;
    /** SSH connection details, kept so the tab can be restored after a restart. */
    isSSH?: boolean;
    ssh?: config.SSHConfigSummary;
    totalLines: number;
    matchedLines: number;
    fileSize: number;
    autoScroll: boolean;
    searchQuery: string;
    /** Second-level AND filter applied to the primary search results. */
    secondaryQuery: string;
    isRegex: boolean;
    caseSensitive: boolean;
    invertMatch: boolean;
    currentMatchIndex: number;
    /** When true, only the search-results pane is shown for this tab. */
    resultsOnly?: boolean;
}

export interface TabGroup {
    id: string;
    title: string;
    color: string;
    collapsed: boolean;
    /** Ordered tab ids inside the group. */
    memberIds: string[];
}

/**
 * One slot of the tab strip: either a standalone tab or a whole group. This is
 * the single source of truth for horizontal order; a group always occupies
 * exactly one strip entry, which makes it impossible to "split" a group.
 */
export type StripEntry = { type: 'tab'; id: string } | { type: 'group'; id: string };

export interface AppContext {
    getApi: () => any;

    tabs: Map<string, TabData>;
    groups: Map<string, TabGroup>;
    strip: StripEntry[];
    activeTabId: string | null;
    selectedGroupId: string | null;

    mainScroller: VirtualScroller;
    filterScroller: VirtualScroller;

    panelBottom: HTMLElement;
    panelTop: HTMLElement;
    mainContainer: HTMLElement;
    btnSearchLayout: HTMLElement;
    searchLayoutPopover: HTMLElement;
    searchLayoutIcon: SVGElement;
    tabListEl: HTMLElement;
    btnOpen: HTMLElement;
    btnOpenSSH: HTMLElement;
    btnGotoLineHeader: HTMLElement;
    btnSettingsHeader: HTMLElement;
    btnRecent: HTMLElement;
    btnFollow: HTMLElement;
    statusTailText: HTMLElement;
    searchInput: HTMLInputElement;
    searchBoxContainer: HTMLElement;
    regexErrorIndicator: HTMLElement;
    btnOptCase: HTMLElement;
    btnOptRegex: HTMLElement;
    btnOptInvert: HTMLElement;
    btnNavFirst: HTMLButtonElement;
    btnNavPrev: HTMLButtonElement;
    btnNavNext: HTMLButtonElement;
    btnNavLast: HTMLButtonElement;
    btnSaveSearch: HTMLButtonElement;
    recentModal: HTMLElement;
    recentFilesList: HTMLElement;
    btnCloseRecent: HTMLElement;
    btnClearRecent: HTMLElement;
    groupModal: HTMLElement;
    groupNameInput: HTMLInputElement;
    btnSaveGroup: HTMLElement;
    btnCancelGroup: HTMLElement;
    btnDeleteGroup: HTMLElement;
    tabRenameModal: HTMLElement;
    tabRenameInput: HTMLInputElement;
    btnSaveTabRename: HTMLElement;
    btnCancelTabRename: HTMLElement;
    gotoModal: HTMLElement;
    gotoLineInput: HTMLInputElement;
    errGotoLine: HTMLElement;
    btnSubmitGoto: HTMLElement;
    btnCancelGoto: HTMLElement;
    settingsModal: HTMLElement;
    settingsThemeSelect: HTMLSelectElement;
    settingsLangSelect: HTMLSelectElement;
    settingsSaveTabsCheck: HTMLInputElement;
    settingsMaxHistoryInput: HTMLInputElement;
    btnSaveSettings: HTMLElement;
    btnCancelSettings: HTMLElement;
    ctxMenuMain: HTMLElement;
    ctxMenuFilter: HTMLElement;
    ctxMenuTab: HTMLElement;
    ctxMenuGroup: HTMLElement;
    sshModal: HTMLElement;
    sshAuthSelect: HTMLSelectElement;
    sshProtocolSelect: HTMLSelectElement;
    sshTLSSelect: HTMLSelectElement;
    groupPassword: HTMLElement;
    groupKeyPath: HTMLElement;
    groupKeyPass: HTMLElement;
    errSSH: HTMLElement;

    updateUIState: () => void;
    triggerSearch: () => void;
    loadRecent: () => void;
    addOrUpdateTab: (stats: main.FileStatsResponse) => void;
    switchTab: (tabId: string) => void;
    saveSearchResults: () => Promise<void>;
    validateRegexUI: () => Promise<boolean>;
    navigateMatch: (target: number) => Promise<void>;
    updateSearchNavState: () => void;
    openGroupModal: (groupId?: string) => void;
    /** Persists the whole tab-bar structure (debounced internally). */
    saveSession: () => void;
    closeAllTabs: () => Promise<void>;
    openFilesAction: () => Promise<void>;
}

export type FileStatsResponse = main.FileStatsResponse;
export type RecentFileItem = config.RecentFileItem;
export type SSHConfig = main.SSHConfig;
export type RemoteConfig = main.RemoteConfig;
export type DirEntry = main.DirEntry;
