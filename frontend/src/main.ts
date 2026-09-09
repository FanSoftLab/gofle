// frontend/src/main.ts
import './style.css';
import { VirtualScroller, LogLineData } from './virtual_scroller';
import {
    translations, t, setLang, getLang, applyI18n, resolveLang, resolveTheme,
} from './i18n';
import { initToasts, showToast } from './toast';
import type { AppContext, TabData, TabGroup, StripEntry } from './app_context';
import { TabManager } from './tab_manager';
import { ModalController } from './modal_controller';
import { SSHHandler } from './ssh_handler';
import { InputHistory } from './input_history';
import { SettingsManager, applyLogFontSize } from './settings_manager';
import { ContextMenu } from './context_menu';

type FileStatsResponse = any;

let pendingStartupArgs: string[] = [];

document.addEventListener('DOMContentLoaded', async () => {
    const getApi = () => (window as any).go?.main?.App;

    const ctx: AppContext = {
        getApi,
        tabs: new Map<string, TabData>(),
        groups: new Map<string, TabGroup>(),
        strip: [],
        activeTabId: null,
        selectedGroupId: null,
        mainScroller: null as any,
        filterScroller: null as any,
        panelBottom: document.getElementById('panel-bottom')!,
        panelTop: document.getElementById('panel-top')!,
        mainContainer: document.getElementById('main-container')!,
        btnSearchLayout: document.getElementById('btn-search-layout')!,
        searchLayoutPopover: document.getElementById('search-layout-popover')!,
        searchLayoutIcon: document.getElementById('search-layout-icon') as unknown as SVGElement,
        tabListEl: document.getElementById('tab-list')!,
        btnOpen: document.getElementById('btn-open')!,
        btnOpenSSH: document.getElementById('btn-open-ssh')!,
        btnGotoLineHeader: document.getElementById('btn-goto-line')!,
        btnSettingsHeader: document.getElementById('btn-settings')!,
        btnRecent: document.getElementById('btn-recent')!,
        btnFollow: document.getElementById('btn-follow')!,
        statusTailText: document.getElementById('status-tail-text')!,
        searchInput: document.getElementById('search-input') as HTMLInputElement,
        searchBoxContainer: document.getElementById('search-box-container')!,
        regexErrorIndicator: document.getElementById('regex-error-indicator')!,
        btnOptCase: document.getElementById('btn-opt-match-case')!,
        btnOptRegex: document.getElementById('btn-opt-regex')!,
        btnOptInvert: document.getElementById('btn-opt-invert')!,
        btnNavFirst: document.getElementById('btn-search-first') as HTMLButtonElement,
        btnNavPrev: document.getElementById('btn-search-prev') as HTMLButtonElement,
        btnNavNext: document.getElementById('btn-search-next') as HTMLButtonElement,
        btnNavLast: document.getElementById('btn-search-last') as HTMLButtonElement,
        btnSaveSearch: document.getElementById('btn-save-search-results') as HTMLButtonElement,
        recentModal: document.getElementById('recent-modal')!,
        recentFilesList: document.getElementById('recent-files-list')!,
        btnCloseRecent: document.getElementById('btn-close-recent')!,
        btnClearRecent: document.getElementById('btn-clear-recent')!,
        groupModal: document.getElementById('group-modal')!,
        groupNameInput: document.getElementById('group-name-input') as HTMLInputElement,
        btnSaveGroup: document.getElementById('btn-save-group')!,
        btnCancelGroup: document.getElementById('btn-cancel-group')!,
        btnDeleteGroup: document.getElementById('btn-delete-group')!,
        tabRenameModal: document.getElementById('tab-rename-modal')!,
        tabRenameInput: document.getElementById('tab-rename-input') as HTMLInputElement,
        btnSaveTabRename: document.getElementById('btn-save-tab-rename')!,
        btnCancelTabRename: document.getElementById('btn-cancel-tab-rename')!,
        gotoModal: document.getElementById('goto-modal')!,
        gotoLineInput: document.getElementById('goto-line-input') as HTMLInputElement,
        errGotoLine: document.getElementById('err-goto-line')!,
        btnSubmitGoto: document.getElementById('btn-submit-goto')!,
        btnCancelGoto: document.getElementById('btn-cancel-goto')!,
        settingsModal: document.getElementById('settings-modal')!,
        settingsThemeSelect: document.getElementById('settings-theme') as HTMLSelectElement,
        settingsLangSelect: document.getElementById('settings-lang') as HTMLSelectElement,
        settingsSaveTabsCheck: document.getElementById('settings-save-tabs') as HTMLInputElement,
        settingsMaxHistoryInput: document.getElementById('settings-max-history') as HTMLInputElement,
        btnSaveSettings: document.getElementById('btn-save-settings')!,
        btnCancelSettings: document.getElementById('btn-cancel-settings')!,
        ctxMenuMain: document.getElementById('ctx-menu-main')!,
        ctxMenuFilter: document.getElementById('ctx-menu-filter')!,
        ctxMenuTab: document.getElementById('ctx-menu-tab')!,
        ctxMenuGroup: document.getElementById('ctx-menu-group')!,
        sshModal: document.getElementById('ssh-modal')!,
        sshAuthSelect: document.getElementById('ssh-auth-type') as HTMLSelectElement,
        sshProtocolSelect: document.getElementById('ssh-protocol') as HTMLSelectElement,
        sshTLSSelect: document.getElementById('ssh-tls-mode') as HTMLSelectElement,
        groupPassword: document.getElementById('group-ssh-password')!,
        groupKeyPath: document.getElementById('group-ssh-key-path')!,
        groupKeyPass: document.getElementById('group-ssh-key-pass')!,
        errSSH: document.getElementById('err-ssh-connection')!,
        updateUIState: () => {},
        triggerSearch: () => {},
        loadRecent: () => {},
        addOrUpdateTab: () => {},
        switchTab: () => {},
        saveSearchResults: async () => {},
        validateRegexUI: async () => true,
        navigateMatch: async () => {},
        updateSearchNavState: () => {},
        openGroupModal: () => {},
        saveSession: () => {},
        closeAllTabs: async () => {},
        openFilesAction: async () => {},
        browseRemoteServerAction: async (_tabId: string) => {},
    };

    const mainScroller = new VirtualScroller('viewport-main', (start, count) => {
        const api = getApi();
        return (api && ctx.activeTabId) ? api.GetLogLines(ctx.activeTabId, start, count) : Promise.resolve([] as LogLineData[]);
    });

    const filterScroller = new VirtualScroller('viewport-filter', (start, count) => {
        const api = getApi();
        return (api && ctx.activeTabId) ? api.GetFilteredLines(ctx.activeTabId, start, count) : Promise.resolve([] as LogLineData[]);
    });

    ctx.mainScroller = mainScroller;
    ctx.filterScroller = filterScroller;

    // Reserve space at the bottom of each log viewport for the overlaid status
    // bar (it uses the accent/blue color) so the last visible row is never
    // hidden behind it.
    const applyStatusbarInset = () => {
        // The status bar sits in normal flow below the viewport and does not
        // overlap it, so the last row should sit flush against it: no reserved
        // gap, and no overlap. Keep the inset mechanism but pass 0.
        mainScroller.setBottomInset(0);
        filterScroller.setBottomInset(0);
    };
    applyStatusbarInset();
    window.addEventListener('resize', applyStatusbarInset);

    // U2: clicking a filter (search results) row jumps the main log to that line.
    filterScroller.setOnRowClick((globalIndex) => {
        if (globalIndex >= 0) {
            mainScroller.scrollToIndex(globalIndex);
        }
    });

    const tabManager = new TabManager(ctx);
    const sshHandler = new SSHHandler(ctx);
    const modalController = new ModalController(ctx, sshHandler);
    const settingsManager = new SettingsManager(ctx);
    const contextMenu = new ContextMenu(ctx);

    tabManager.initGroupModalHandlers();

    // --- Orchestration functions (composition root) ---

    const updateSearchNavState = () => {
        const counter = document.getElementById('match-counter-text');
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) {
            ctx.btnNavFirst.disabled = true;
            ctx.btnNavPrev.disabled = true;
            ctx.btnNavNext.disabled = true;
            ctx.btnNavLast.disabled = true;
            ctx.btnSaveSearch.disabled = true;
            ctx.searchBoxContainer.classList.remove('search-active');
            counter?.classList.remove('search-active');
            return;
        }

        const tab = ctx.tabs.get(ctx.activeTabId)!;
        const count = tab.matchedLines;
        const hasQuery = !!(tab.searchQuery || tab.secondaryQuery);
        const active = hasQuery && count > 0;
        ctx.searchBoxContainer.classList.toggle('search-active', active);
        counter?.classList.toggle('search-active', active);

        if (count > 1) {
            ctx.btnNavFirst.disabled = tab.currentMatchIndex <= 0;
            ctx.btnNavPrev.disabled = tab.currentMatchIndex <= 0;
            ctx.btnNavNext.disabled = tab.currentMatchIndex >= count - 1;
            ctx.btnNavLast.disabled = tab.currentMatchIndex >= count - 1;
            ctx.btnSaveSearch.disabled = false;
        } else if (count === 1) {
            ctx.btnNavFirst.disabled = true;
            ctx.btnNavPrev.disabled = true;
            ctx.btnNavNext.disabled = true;
            ctx.btnNavLast.disabled = true;
            ctx.btnSaveSearch.disabled = false;
        } else {
            ctx.btnNavFirst.disabled = true;
            ctx.btnNavPrev.disabled = true;
            ctx.btnNavNext.disabled = true;
            ctx.btnNavLast.disabled = true;
            ctx.btnSaveSearch.disabled = true;
        }
    };

    const navigateMatch = async (targetMatchIndex: number) => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        if (targetMatchIndex < 0 || targetMatchIndex >= tab.matchedLines) return;

        tab.currentMatchIndex = targetMatchIndex;
        filterScroller.scrollToIndex(targetMatchIndex);

        const api = getApi();
        if (api) {
            const lines = await api.GetFilteredLines(ctx.activeTabId, targetMatchIndex, 1);
            if (lines && lines.length > 0) {
                mainScroller.scrollToIndex(lines[0].index);
            }
        }
        updateSearchNavState();
    };

    const saveSearchResults = async () => {
        if (!ctx.activeTabId) return;
        const api = getApi();
        if (api) {
            try {
                const savedPath = await api.SaveSearchResults(ctx.activeTabId);
                if (savedPath) {
                    showToast(t('msg_search_results_saved', { path: savedPath }), 'info');
                }
            } catch (err) {
                const msg = String((err as any)?.message ?? err);
                showToast(msg, msg.includes('unreadable') ? 'info' : 'error');
            }
        }
    };

    const validateRegexUI = async () => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) return true;
        const tab = ctx.tabs.get(ctx.activeTabId)!;

        const secQuery = tab.secondaryQuery ?? '';
        if (!tab.isRegex || (!ctx.searchInput.value && !secQuery)) {
            ctx.searchBoxContainer.classList.remove('invalid-regex');
            ctx.regexErrorIndicator.classList.add('hidden');
            return true;
        }

        if (tab.isRegex && secQuery) {
            try {
                void new RegExp(secQuery);
            } catch {
                ctx.searchBoxContainer.classList.add('invalid-regex');
                ctx.regexErrorIndicator.classList.remove('hidden');
                return false;
            }
        }

        const api = getApi();
        if (api) {
            const res = await api.ValidateRegex(ctx.searchInput.value);
            if (!res.is_valid) {
                ctx.searchBoxContainer.classList.add('invalid-regex');
                ctx.regexErrorIndicator.title = res.error_message;
                ctx.regexErrorIndicator.classList.remove('hidden');
                return false;
            }
        }

        ctx.searchBoxContainer.classList.remove('invalid-regex');
        ctx.regexErrorIndicator.classList.add('hidden');
        return true;
    };

    let searchDebounce: any;
    const triggerSearch = async () => {
        if (!ctx.activeTabId) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.searchQuery = ctx.searchInput.value;

        const isValid = await validateRegexUI();
        if (!isValid) return;

        clearTimeout(searchDebounce);
        searchDebounce = setTimeout(async () => {
            const api = getApi();
            if (!api || !ctx.activeTabId) return;
            try {
                const matches = await api.SearchLines(ctx.activeTabId, tab.searchQuery, tab.isRegex, tab.caseSensitive, tab.invertMatch, tab.secondaryQuery);
                tab.matchedLines = matches;
                tab.currentMatchIndex = matches > 0 ? 0 : -1;
                document.getElementById('match-counter-text')!.textContent = `${matches} / ${tab.totalLines}${t('match_counter_suffix')}`;
                filterScroller.setTotalLines(matches);
                filterScroller.setHighlightedLine(null);
                mainScroller.setSearchQuery(tab.searchQuery);
                updateSearchNavState();
                syncSearchClearBtn();
            } catch (err) {
                // On-change typing makes backend search errors user-triggerable
                // (invalid secondary regex): surface them, not just log.
                const msg = String((err as any)?.message ?? err);
                showToast(msg.includes('secondary') ? 'secondary: ' + msg.split(': ').pop() : msg, 'error');
            }
        }, 200);
    };

    // executeSearchNow runs the search synchronously (no debounce) — used when the
    // user presses Enter so the current query is searched immediately and only once.
    const executeSearchNow = async (raw: string) => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        ctx.searchInput.value = raw;
        tab.searchQuery = raw;

        const isValid = await validateRegexUI();
        if (!isValid) return;

        clearTimeout(searchDebounce);
        const api = getApi();
        if (!api || !ctx.activeTabId) return;
        try {
            const matches = await api.SearchLines(ctx.activeTabId, tab.searchQuery, tab.isRegex, tab.caseSensitive, tab.invertMatch, tab.secondaryQuery);
            tab.matchedLines = matches;
            tab.currentMatchIndex = matches > 0 ? 0 : -1;
            document.getElementById('match-counter-text')!.textContent = `${matches} / ${tab.totalLines}${t('match_counter_suffix')}`;
            filterScroller.setTotalLines(matches);
            filterScroller.setHighlightedLine(null);
            mainScroller.setSearchQuery(tab.searchQuery);
            updateSearchNavState();
        } catch (err) {
            console.error('Search error:', err);
            showToast(String((err as any)?.message ?? err), 'error');
        }
    };

    const searchHistory = new InputHistory(ctx.searchInput, {
        namespace: 'search',
        getApi,
        historyButton: document.getElementById('btn-search-history'),
        onExecute: (v: string) => {
            executeSearchNow(v);
            searchHistory.commit(v);
        },
    });
    searchHistory.attach();
    searchHistory.load();

    const updateUIState = () => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) {
            document.getElementById('status-file-text')!.textContent = t('status_file_none');
            document.getElementById('status-size-text')!.textContent = '0 B';
            document.getElementById('status-lines-text')!.textContent = '0';
            document.getElementById('match-counter-text')!.textContent = `0 / 0${t('match_counter_suffix')}`;
            ctx.statusTailText.textContent = t('status_tail_none');
            ctx.btnFollow.classList.remove('active');
            document.getElementById('status-levels')?.classList.add('hidden');
            document.getElementById('status-filter-badge')?.classList.add('hidden');
            document.getElementById('empty-state')?.classList.remove('hidden');
            mainScroller.setTotalLines(0);
            filterScroller.setTotalLines(0);
            updateSearchNavState();
            return;
        }

        const tab = ctx.tabs.get(ctx.activeTabId)!;
        const fileEl = document.getElementById('status-file-text')!;
        const fullPath = `${t('status_file_prefix')}${tab.filePath}`;
        fileEl.textContent = truncateMiddle(fullPath, 56);
        fileEl.title = tab.filePath;
        document.getElementById('status-size-text')!.textContent = `${t('status_size_prefix')}${(tab.fileSize / 1024 / 1024).toFixed(2)} MB`;
        document.getElementById('status-lines-text')!.textContent = `${t('status_lines_prefix')}${tab.totalLines}`;
        document.getElementById('match-counter-text')!.textContent = `${tab.matchedLines} / ${tab.totalLines}${t('match_counter_suffix')}`;
        ctx.statusTailText.textContent = tab.autoScroll ? t('status_tail_active') : t('status_tail_paused');
        ctx.btnFollow.classList.toggle('active', tab.autoScroll);
        document.getElementById('empty-state')?.classList.add('hidden');
        void refreshLevelStats();

        ctx.searchInput.value = tab.searchQuery;
        const secEl = document.getElementById('secondary-search-input') as HTMLInputElement | null;
        if (secEl && document.activeElement !== secEl) secEl.value = tab.secondaryQuery ?? '';
        ctx.btnOptCase.classList.toggle('active', tab.caseSensitive);
        ctx.btnOptRegex.classList.toggle('active', tab.isRegex);
        ctx.btnOptInvert.classList.toggle('active', tab.invertMatch);
        ctx.btnFollow.classList.toggle('active', tab.autoScroll);
        ctx.mainContainer.classList.toggle('results-only', !!tab.resultsOnly);
        syncResultsOnlyBadge();
        syncSearchClearBtn();

        mainScroller.setAutoScroll(tab.autoScroll);
        mainScroller.setTotalLines(tab.totalLines);
        mainScroller.setSearchQuery(tab.searchQuery);
        filterScroller.setTotalLines(tab.matchedLines);
        validateRegexUI();
        updateSearchNavState();
    };

    /** Truncate a path in the middle so the status bar stays readable. */
    const truncateMiddle = (s: string, max: number): string => {
        if (s.length <= max) return s;
        const keep = Math.max(4, Math.floor((max - 1) / 2));
        return s.slice(0, keep) + '…' + s.slice(s.length - keep);
    };

    /** Show/hide the status-bar "Results only" badge to match the CSS mode. */
    const syncResultsOnlyBadge = () => {
        const badge = document.getElementById('status-filter-badge');
        if (!badge) return;
        const on = ctx.mainContainer.classList.contains('results-only');
        badge.classList.toggle('hidden', !on);
    };

    const toggleResultsOnly = () => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.resultsOnly = !tab.resultsOnly;
        ctx.mainContainer.classList.toggle('results-only', !!tab.resultsOnly);
        syncResultsOnlyBadge();
    };

    /** Clear primary + secondary search for the active tab. */
    const clearSearch = () => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.searchQuery = '';
        tab.secondaryQuery = '';
        tab.matchedLines = 0;
        tab.currentMatchIndex = -1;
        ctx.searchInput.value = '';
        const secEl = document.getElementById('secondary-search-input') as HTMLInputElement | null;
        if (secEl) secEl.value = '';
        ctx.searchBoxContainer.classList.remove('invalid-regex');
        ctx.regexErrorIndicator.classList.add('hidden');
        mainScroller.setSearchQuery('');
        filterScroller.setTotalLines(0);
        document.getElementById('match-counter-text')!.textContent =
            `0 / ${tab.totalLines}${t('match_counter_suffix')}`;
        updateSearchNavState();
        syncSearchClearBtn();
        // Drop results-only when there is nothing to filter.
        if (tab.resultsOnly) {
            tab.resultsOnly = false;
            ctx.mainContainer.classList.remove('results-only');
            syncResultsOnlyBadge();
        }
        const api = getApi();
        if (api) {
            void api.SearchLines(ctx.activeTabId, '', false, false, false, '').catch(() => {});
        }
    };

    const syncSearchClearBtn = () => {
        const btn = document.getElementById('btn-clear-search');
        if (!btn) return;
        const has =
            !!ctx.searchInput.value ||
            !!(document.getElementById('secondary-search-input') as HTMLInputElement | null)?.value;
        btn.classList.toggle('hidden', !has);
    };

    // --- Session persistence (frontend is the source of truth) ---
    // The tab strip structure (order, groups, collapsed state, active tab) only
    // exists here, so Go never rebuilds it; we push the whole snapshot on every
    // structural change. Writes are debounced so a drag does not spam the disk.
    const buildSession = () => ({
        active_tab_id: ctx.activeTabId || '',
        top_level: ctx.strip.map(e => ({ type: e.type, id: e.id })),
        // An SSH tab without its connection summary could never be reopened, so it
        // is left out entirely instead of being persisted as an unrestorable entry
        // (restore tolerates ids that are missing from `tabs`).
        tabs: [...ctx.tabs.values()]
            .filter(tabItem => !tabItem.isSSH || !!tabItem.ssh)
            .map(tabItem => ({
                id: tabItem.id,
                title: tabItem.title || '',
                path: tabItem.filePath,
                is_ssh: !!tabItem.isSSH,
                ssh: tabItem.ssh || undefined,
                group_id: tabItem.groupID || '',
            })),
        groups: [...ctx.groups.values()].map(g => ({
            id: g.id,
            title: g.title,
            color: g.color,
            collapsed: g.collapsed,
            member_ids: g.memberIds,
        })),
    });

    let sessionSaveTimer: number | undefined;
    let sessionRestoring = false;

    const pushSession = () => {
        const api = getApi();
        if (!api || !api.SaveSession) return;
        // Bindings reject asynchronously, so a plain try/catch never sees the
        // failure — route the rejection through a visible toast.
        Promise.resolve(api.SaveSession(buildSession())).catch((err: unknown) => {
            console.error('SaveSession failed:', err);
            showToast(String((err as any)?.message ?? err), 'error');
        });
    };

    const saveSession = () => {
        // Restoring mutates ctx heavily; persisting mid-restore could snapshot a
        // half-built strip if the app were closed at that exact moment.
        if (sessionRestoring) return;
        if (sessionSaveTimer !== undefined) clearTimeout(sessionSaveTimer);
        sessionSaveTimer = window.setTimeout(() => {
            sessionSaveTimer = undefined;
            pushSession();
        }, 250);
    };

    // Go's Shutdown no longer snapshots the tabs, so a pending debounced save must
    // be flushed on teardown or the very last change would be lost.
    // Always persist on teardown — not only when a debounce timer is pending.
    // Previously a completed debounce left sessionSaveTimer undefined, so the
    // exit path was a no-op; combined with Wails often skipping beforeunload,
    // the last tab state could fail to reach disk.
    const flushSession = () => {
        if (sessionRestoring) return;
        if (sessionSaveTimer !== undefined) {
            clearTimeout(sessionSaveTimer);
            sessionSaveTimer = undefined;
        }
        pushSession();
    };
    window.addEventListener('beforeunload', flushSession);
    window.addEventListener('pagehide', flushSession);
    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'hidden') flushSession();
    });
    // Wails OnBeforeClose emits this so we flush even when the webview does not
    // deliver a reliable beforeunload/pagehide on window close.
    try {
        (window as any).runtime?.EventsOn?.('app:before-close', () => {
            flushSession();
        });
    } catch {
        /* runtime may be absent in plain browser tests */
    }

    // Wire cross-manager callbacks into the shared context.
    ctx.updateUIState = updateUIState;
    ctx.triggerSearch = triggerSearch;
    ctx.loadRecent = modalController.loadRecent;
    ctx.addOrUpdateTab = tabManager.addOrUpdateTab;
    ctx.switchTab = tabManager.switchTab;
    ctx.saveSearchResults = saveSearchResults;
    ctx.validateRegexUI = validateRegexUI;
    ctx.navigateMatch = navigateMatch;
    ctx.updateSearchNavState = updateSearchNavState;
    ctx.openGroupModal = tabManager.openGroupModal;
    ctx.saveSession = saveSession;
    ctx.closeAllTabs = async () => {
        await tabManager.closeAllTabs();
        // closeAllTabs must not feed the reopen stack
        (tabManager as any).suppressReopen = false;
    };

    // --- Search pane layout (orientation + side) ---
    let searchOrientation = 'horizontal';
    let searchSide = 'bottom';
    // Per-orientation thickness. 0 means "split in half" (the default) and is
    // resolved to half the container size when the layout is applied.
    let searchThicknessH = 0;
    let searchThicknessV = 0;

    const getThickness = () => searchOrientation === 'vertical' ? searchThicknessV : searchThicknessH;
    const setThickness = (v: number) => {
        if (searchOrientation === 'vertical') searchThicknessV = v;
        else searchThicknessH = v;
    };

    const applySearchPaneLayout = (orientation: string, side: string) => {
        searchOrientation = orientation;
        searchSide = side;
        const { mainContainer, panelTop, panelBottom: panel } = ctx;
        const splitterEl = document.getElementById('splitter')!;
        mainContainer.style.flexDirection = orientation === 'vertical' ? 'row' : 'column';
        panel.style.height = '';
        panel.style.width = '';
        panel.style.flex = 'none';
        const resultsFirst = side === 'top' || side === 'left';
        panelTop.style.order = resultsFirst ? '3' : '1';
        splitterEl.style.order = '2';
        panel.style.order = resultsFirst ? '1' : '3';
        splitterEl.style.cursor = orientation === 'vertical' ? 'col-resize' : 'row-resize';
        // Give the splitter a non-zero thickness on the correct axis so it is
        // always grabbable (in a row layout a 0-width splitter is invisible).
        if (orientation === 'vertical') {
            splitterEl.style.width = '6px';
            splitterEl.style.height = '100%';
        } else {
            splitterEl.style.height = '6px';
            splitterEl.style.width = '100%';
        }
        // Reflect orientation in the toolbar toggle icon.
        const icon = ctx.btnSearchLayout.querySelector('svg');
        if (icon) {
            icon.innerHTML = orientation === 'vertical'
                ? '<rect x="3" y="4" width="7" height="16" rx="1.5"/><rect x="14" y="4" width="7" height="16" rx="1.5"/>'
                : '<rect x="3" y="4" width="18" height="7" rx="1.5"/><rect x="3" y="13" width="18" height="7" rx="1.5"/>';
        }
        // Highlight the active orientation / side in the popover.
        document.querySelectorAll<HTMLElement>('.layout-opt').forEach((b) => {
            b.classList.toggle('active', b.dataset.orient === orientation);
        });
        document.querySelectorAll<HTMLElement>('.layout-side').forEach((b) => {
            const vertical = orientation === 'vertical';
            b.classList.toggle('active', b.dataset.side === side);
            b.style.display = vertical
                ? (b.dataset.side === 'left' || b.dataset.side === 'right' ? '' : 'none')
                : (b.dataset.side === 'top' || b.dataset.side === 'bottom' ? '' : 'none');
        });
        // Resolve and apply the thickness on the correct axis. 0 => half.
        const containerSize = orientation === 'vertical'
            ? mainContainer.clientWidth
            : mainContainer.clientHeight;
        let thickness = getThickness();
        if (!thickness || thickness < 60) {
            thickness = Math.floor((containerSize > 0 ? containerSize : 800) / 2);
        }
        if (orientation === 'vertical') {
            panel.style.width = `${thickness}px`;
            panel.style.height = '100%';
        } else {
            panel.style.height = `${thickness}px`;
            panel.style.width = '100%';
        }
    };

    // --- Splitter (resize the search pane on the active axis) ---
    const splitter = document.getElementById('splitter')!;
    let isDragging = false;
    let startPos = 0;
    let startSize = 250;

    splitter.addEventListener('mousedown', (e) => {
        isDragging = true;
        startPos = searchOrientation === 'vertical' ? e.clientX : e.clientY;
        startSize = searchOrientation === 'vertical' ? ctx.panelBottom.clientWidth : ctx.panelBottom.clientHeight;
        document.body.style.cursor = searchOrientation === 'vertical' ? 'col-resize' : 'row-resize';
        document.body.style.userSelect = 'none';
    });

    document.addEventListener('mousemove', (e) => {
        if (!isDragging) return;
        const container = ctx.mainContainer;
        const minSize = 60;
        let newSize: number;
        if (searchOrientation === 'vertical') {
            const containerSize = container.clientWidth;
            const delta = searchSide === 'left' ? e.clientX - startPos : startPos - e.clientX;
            newSize = startSize + delta;
            if (newSize < minSize) newSize = minSize;
            if (newSize > containerSize - 100) newSize = containerSize - 100;
            ctx.panelBottom.style.width = `${newSize}px`;
            ctx.panelBottom.style.height = '100%';
        } else {
            const containerSize = container.clientHeight;
            const delta = searchSide === 'top' ? e.clientY - startPos : startPos - e.clientY;
            newSize = startSize + delta;
            if (newSize < minSize) newSize = minSize;
            if (newSize > containerSize - 100) newSize = containerSize - 100;
            ctx.panelBottom.style.height = `${newSize}px`;
            ctx.panelBottom.style.width = '100%';
        }
        setThickness(Math.round(newSize));
        mainScroller.render();
        filterScroller.render();
    });

    document.addEventListener('mouseup', () => {
        if (isDragging) {
            isDragging = false;
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
            const api = getApi();
            const thickness = getThickness();
            if (api) {
                if (searchOrientation === 'vertical') {
                    if (api.SaveSearchPaneWidth) api.SaveSearchPaneWidth(thickness);
                } else {
                    if (api.SaveSearchPaneHeight) api.SaveSearchPaneHeight(thickness);
                }
            }
        }
    });

    // --- Search layout picker (orientation / side) ---
    const layoutPopover = ctx.searchLayoutPopover;
    ctx.btnSearchLayout.addEventListener('click', (e) => {
        e.stopPropagation();
        layoutPopover.classList.toggle('hidden');
    });
    document.addEventListener('click', (e) => {
        if (layoutPopover.classList.contains('hidden')) return;
        const t = e.target as Node;
        if (!layoutPopover.contains(t) && t !== ctx.btnSearchLayout && !ctx.btnSearchLayout.contains(t)) {
            layoutPopover.classList.add('hidden');
        }
    });
    const persistLayout = () => {
        const api = getApi();
        if (api && api.SaveSearchPaneLayout) api.SaveSearchPaneLayout(searchOrientation, searchSide);
    };
    layoutPopover.querySelectorAll<HTMLElement>('.layout-opt').forEach((b) => {
        b.addEventListener('click', () => {
            const orient = b.dataset.orient!;
            let side = searchSide;
            if (orient === 'vertical' && (side === 'top' || side === 'bottom')) side = 'right';
            if (orient === 'horizontal' && (side === 'left' || side === 'right')) side = 'bottom';
            applySearchPaneLayout(orient, side);
            mainScroller.render();
            filterScroller.render();
            persistLayout();
        });
    });
    layoutPopover.querySelectorAll<HTMLElement>('.layout-side').forEach((b) => {
        b.addEventListener('click', () => {
            applySearchPaneLayout(searchOrientation, b.dataset.side!);
            mainScroller.render();
            filterScroller.render();
            persistLayout();
        });
    });

    // --- Drag & Drop via Wails Runtime ---
    // Pass `false` to disable Wails' drop-target CSS gating (v2.10.1 never
    // injects `--wails-drop-target` as a computed style, so the callback would
    // otherwise be silently filtered out and never fire).
    if ((window as any).runtime?.OnFileDrop) {
        (window as any).runtime.OnFileDrop(async (_x: number, _y: number, paths: string[]) => {
            if (!paths || paths.length === 0) return;
            const api = getApi();
            if (!api) return;
            try {
                const statsList = await api.OpenFiles(paths);
                if (statsList && statsList.length > 0) {
                    statsList.forEach((stats: FileStatsResponse) => tabManager.addOrUpdateTab(stats));
                }
            } catch (err) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }, false);
    }

    // Visual drag indicator (highlight only — file opening is handled by the
    // Wails OnFileDrop callback above to avoid duplicate tabs).
    let dragDepth = 0;
    const toggleDrag = (on: boolean) => document.body.classList.toggle('drag-over', on);
    window.addEventListener('dragenter', (e: DragEvent) => {
        if (e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')) {
            dragDepth++;
            toggleDrag(true);
        }
    });
    window.addEventListener('dragover', (e: DragEvent) => {
        if (e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')) {
            e.preventDefault();
        }
    });
    window.addEventListener('dragleave', () => {
        if (--dragDepth <= 0) {
            dragDepth = 0;
            toggleDrag(false);
        }
    });
    window.addEventListener('drop', () => {
        dragDepth = 0;
        toggleDrag(false);
    });

    // C2: debounce Tail/Follow scroll updates.
    let followScrollTimer: number | undefined;
    const scheduleFollowScroll = () => {
        if (followScrollTimer !== undefined) {
            clearTimeout(followScrollTimer);
        }
        followScrollTimer = window.setTimeout(() => {
            followScrollTimer = undefined;
            mainScroller.scrollToBottom();
        }, 100);
    };

    initToasts();

    // --- Level stats (E/W) in the status bar ---
    const levelStatsCache = new Map<string, { sig: string; stats: Record<string, number> }>();
    let levelsTimer: ReturnType<typeof setTimeout> | undefined;
    const renderLevels = (stats: Record<string, number>) => {
        const host = document.getElementById('status-levels');
        if (!host) return;
        host.classList.remove('hidden');
        (document.getElementById('lvl-e') as HTMLElement).textContent = `E:${stats.E ?? 0}`;
        (document.getElementById('lvl-w') as HTMLElement).textContent = `W:${stats.W ?? 0}`;
    };
    const refreshLevelStats = async () => {
        if (!ctx.activeTabId || !ctx.tabs.has(ctx.activeTabId)) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        const sig = `${tab.fileSize}:${tab.totalLines}`;
        const cached = levelStatsCache.get(ctx.activeTabId);
        if (cached && cached.sig === sig) { renderLevels(cached.stats); return; }
        const api = getApi();
        if (!api?.GetLevelStats) return;
        try {
            const stats = await api.GetLevelStats(ctx.activeTabId);
            levelStatsCache.set(ctx.activeTabId, { sig, stats });
            if (ctx.activeTabId && ctx.tabs.has(ctx.activeTabId)) renderLevels(stats);
        } catch (err) {
            console.error('level stats:', err);
            showToast(String((err as any)?.message ?? err), 'error');
        }
    };
    const scheduleLevelRefresh = () => {
        if (levelsTimer) clearTimeout(levelsTimer);
        levelsTimer = setTimeout(refreshLevelStats, 1000);
    };
    document.getElementById('lvl-e')?.addEventListener('click', () => {
        ctx.searchInput.value = 'ERROR';
        ctx.triggerSearch();
    });
    document.getElementById('lvl-w')?.addEventListener('click', () => {
        ctx.searchInput.value = 'WARN';
        ctx.triggerSearch();
    });


    if ((window as any).runtime?.EventsOn) {
        (window as any).runtime.EventsOn('file_updated', (data: any) => {
            if (data && data.tab_id && ctx.tabs.has(data.tab_id)) {
                const tab = ctx.tabs.get(data.tab_id)!;
                tab.totalLines = data.total_lines;
                tab.fileSize = data.file_size;
                tab.matchedLines = data.match_count;
                if (ctx.activeTabId === data.tab_id) {
                    updateUIState();
                    if (tab.autoScroll) {
                        scheduleFollowScroll();
                    }
                    scheduleLevelRefresh();
                }
            }
        });

        // Backend-reported failures that must never be silent: a batch-opened
        // file that failed, or a tab whose tail poll keeps erroring.
        (window as any).runtime.EventsOn('file_error', (data: any) => {
            const what = data?.path ?? data?.tab_id ?? 'unknown';
            showToast(`${what}: ${data?.error ?? ''}`, 'error');
        });
    }

    // --- Open files ---
    const openFilesAction = async () => {
        const api = getApi();
        if (api) {
            try {
                const res = await api.OpenMultipleFilesDialog();
                if (res && res.length > 0) {
                    res.forEach((stats: FileStatsResponse) => tabManager.addOrUpdateTab(stats));
                }
            } catch (err) {
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
    };

    ctx.btnOpen.addEventListener('click', openFilesAction);
    ctx.openFilesAction = openFilesAction;
    ctx.browseRemoteServerAction = async (tabId: string) => {
        const tab = ctx.tabs.get(tabId);
        if (!tab?.ssh) {
            showToast(t('err_not_remote_tab'), 'error');
            return;
        }
        await sshHandler.browseOnServerFromTab(tab.ssh);
    };

    // Secondary AND-filter mirrors the primary box: applies on change
    // (triggerSearch debounces at 200ms), no Enter required.
    const secInput = document.getElementById('secondary-search-input') as HTMLInputElement | null;
    secInput?.addEventListener('input', () => {
        if (!ctx.activeTabId) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.secondaryQuery = secInput.value;
        ctx.triggerSearch();
    });

    function inTextField(e: KeyboardEvent): boolean {
        const el = e.target as HTMLElement | null;
        return !!el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.isContentEditable);
    }

    document.getElementById('btn-close-help')?.addEventListener('click', () => {
        document.getElementById('help-overlay')?.classList.add('hidden');
    });

    // Results-only badge: click exits the mode (same as pressing r).
    document.getElementById('status-filter-badge')?.addEventListener('click', () => {
        if (ctx.mainContainer.classList.contains('results-only')) {
            toggleResultsOnly();
        }
    });

    // --- Follow / search option toggles ---
    ctx.btnFollow.addEventListener('click', () => {
        if (!ctx.activeTabId) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.autoScroll = !tab.autoScroll;
        ctx.btnFollow.classList.toggle('active', tab.autoScroll);
        ctx.statusTailText.textContent = tab.autoScroll ? t('status_tail_active') : t('status_tail_paused');
        mainScroller.setAutoScroll(tab.autoScroll);
        if (tab.autoScroll) mainScroller.scrollToBottom();

        const api = getApi();
        if (api) api.SetAutoScroll(ctx.activeTabId, tab.autoScroll);
    });

    ctx.btnOptCase.addEventListener('click', () => {
        if (!ctx.activeTabId) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.caseSensitive = !tab.caseSensitive;
        ctx.btnOptCase.classList.toggle('active', tab.caseSensitive);
        triggerSearch();
    });

    ctx.btnOptRegex.addEventListener('click', () => {
        if (!ctx.activeTabId) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.isRegex = !tab.isRegex;
        ctx.btnOptRegex.classList.toggle('active', tab.isRegex);
        triggerSearch();
    });

    ctx.btnOptInvert.addEventListener('click', () => {
        if (!ctx.activeTabId) return;
        const tab = ctx.tabs.get(ctx.activeTabId)!;
        tab.invertMatch = !tab.invertMatch;
        ctx.btnOptInvert.classList.toggle('active', tab.invertMatch);
        triggerSearch();
    });

    ctx.btnGotoLineHeader.addEventListener('click', modalController.openGotoModal);

    ctx.btnNavFirst.addEventListener('click', () => navigateMatch(0));
    ctx.btnNavPrev.addEventListener('click', () => {
        if (ctx.activeTabId && ctx.tabs.has(ctx.activeTabId)) {
            navigateMatch(ctx.tabs.get(ctx.activeTabId)!.currentMatchIndex - 1);
        }
    });
    ctx.btnNavNext.addEventListener('click', () => {
        if (ctx.activeTabId && ctx.tabs.has(ctx.activeTabId)) {
            navigateMatch(ctx.tabs.get(ctx.activeTabId)!.currentMatchIndex + 1);
        }
    });
    ctx.btnNavLast.addEventListener('click', () => {
        if (ctx.activeTabId && ctx.tabs.has(ctx.activeTabId)) {
            navigateMatch(ctx.tabs.get(ctx.activeTabId)!.matchedLines - 1);
        }
    });

    ctx.btnSaveSearch.addEventListener('click', () => saveSearchResults());
    ctx.searchInput.addEventListener('input', () => {
        syncSearchClearBtn();
        triggerSearch();
    });
    document.getElementById('btn-clear-search')?.addEventListener('click', () => {
        clearSearch();
        ctx.searchInput.focus();
    });
    // Secondary input already triggers search; also keep the clear button in sync.
    document.getElementById('secondary-search-input')?.addEventListener('input', () => {
        syncSearchClearBtn();
    });

    // --- Global shortcuts ---
    // Order: modifiers first (work even inside inputs), then Esc, then plain
    // keys that must not fire while typing in a text field.
    window.addEventListener('keydown', (e) => {
        const helpEl = document.getElementById('help-overlay');

        // Ctrl/Cmd shortcuts — available even when focus is in an input.
        // Ctrl/Cmd+C: copy the current line selection from the active viewport.
        // Skip when focus is in an input/textarea (native copy should win).
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c') {
            const tag = (e.target as HTMLElement | null)?.tagName;
            if (tag !== 'INPUT' && tag !== 'TEXTAREA') {
                const scroller =
                    document.activeElement?.closest('#viewport-filter')
                        ? filterScroller
                        : mainScroller;
                const range = scroller.getSelectedRange();
                if (range) {
                    e.preventDefault();
                    void scroller.copySelection().then((ok) => {
                        if (ok) showToast(t('ctx_copy_line'), 'info', 1500);
                    });
                    return;
                }
            }
        }
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'o') {
            e.preventDefault();
            ctx.btnOpen.click();
            return;
        }
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'r') {
            e.preventDefault();
            ctx.btnRecent.click();
            return;
        }
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'g') {
            e.preventDefault();
            modalController.openGotoModal();
            return;
        }
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 's') {
            e.preventDefault();
            saveSearchResults();
            return;
        }
        if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === 't') {
            e.preventDefault();
            tabManager.reopenClosed();
            return;
        }

        // Match navigation (F3 / Alt+arrows) — useful while search is focused.
        if (e.key === 'F3' || (e.altKey && e.key === 'ArrowDown')) {
            e.preventDefault();
            ctx.btnNavNext.click();
            return;
        }
        if ((e.shiftKey && e.key === 'F3') || (e.altKey && e.key === 'ArrowUp')) {
            e.preventDefault();
            ctx.btnNavPrev.click();
            return;
        }
        if (e.altKey && e.key === 'Home') {
            e.preventDefault();
            ctx.btnNavFirst.click();
            return;
        }
        if (e.altKey && e.key === 'End') {
            e.preventDefault();
            ctx.btnNavLast.click();
            return;
        }

        // Search option toggles (tooltips promise Alt+C/R/I).
        if (e.altKey && !e.ctrlKey && !e.metaKey) {
            const k = e.key.toLowerCase();
            if (k === 'c') {
                e.preventDefault();
                ctx.btnOptCase.click();
                return;
            }
            if (k === 'r') {
                e.preventDefault();
                ctx.btnOptRegex.click();
                return;
            }
            if (k === 'i') {
                e.preventDefault();
                ctx.btnOptInvert.click();
                return;
            }
            if (/^[1-9]$/.test(e.key)) {
                e.preventDefault();
                const idx = parseInt(e.key, 10) - 1;
                const entry = ctx.strip[idx];
                if (entry) {
                    ctx.switchTab(entry.type === 'group'
                        ? (ctx.groups.get(entry.id)?.memberIds[0] ?? '')
                        : entry.id);
                }
                return;
            }
        }

        // Escape: close help → clear search if focused/has text → blur.
        if (e.key === 'Escape') {
            if (helpEl && !helpEl.classList.contains('hidden')) {
                e.preventDefault();
                helpEl.classList.add('hidden');
                return;
            }
            const active = document.activeElement as HTMLElement | null;
            const inSearch =
                active === ctx.searchInput || active?.id === 'secondary-search-input';
            const hasQuery =
                !!ctx.searchInput.value ||
                !!(document.getElementById('secondary-search-input') as HTMLInputElement | null)?.value;
            if (inSearch && hasQuery) {
                e.preventDefault();
                clearSearch();
                return;
            }
            if (inSearch) {
                e.preventDefault();
                active?.blur();
                return;
            }
            if (hasQuery && !inTextField(e)) {
                e.preventDefault();
                clearSearch();
                return;
            }
        }

        // Help overlays — available even while typing (F1 always, ? outside fields).
        if (e.key === 'F1') {
            e.preventDefault();
            helpEl?.classList.toggle('hidden');
            return;
        }

        // Plain keys that must not steal keystrokes from inputs.
        if (inTextField(e)) return;

        if (e.key === '/') {
            e.preventDefault();
            ctx.searchInput.focus();
            ctx.searchInput.select();
            return;
        }
        // Backslash focuses the secondary AND-filter (same as CLI `\`).
        if (e.key === '\\') {
            e.preventDefault();
            const sec = document.getElementById('secondary-search-input') as HTMLInputElement | null;
            if (sec) {
                sec.focus();
                sec.select();
            }
            return;
        }
        if (e.key === 'n') { ctx.btnNavNext.click(); }
        else if (e.key === 'p') { ctx.btnNavPrev.click(); }
        else if (e.key === '[') { ctx.btnNavFirst.click(); }
        else if (e.key === ']') { ctx.btnNavLast.click(); }
        else if (e.key === 'f') { ctx.btnFollow.click(); }
        else if (e.key === 'r') { toggleResultsOnly(); }
        else if (e.key === '?') { helpEl?.classList.toggle('hidden'); }
        else if (e.key === 'e' && ctx.activeTabId) {
            const tab = ctx.tabs.get(ctx.activeTabId)!;
            if (tab.matchedLines > 0) {
                ctx.saveSearchResults();
            } else {
                getApi()?.SaveFullLogAs(ctx.activeTabId)
                    .then((p: string) => showToast(t('msg_full_log_saved', { path: p }), 'info'))
                    .catch((er: unknown) => showToast(String(er), 'error'));
            }
        }
    });

    // --- Initial settings ---
    const loadSettings = async () => {
        const api = getApi();
        if (api) {
            try {
                const cfg = await api.GetSettings();
                if (cfg) {
                    setLang(resolveLang(cfg.language as string | undefined));
                    document.body.className = `theme-${resolveTheme(cfg.theme as string | undefined)}`;
                    // Restore the search-results panel orientation, side and thickness.
                    try {
                        const layout = await api.GetSearchPaneLayout();
                        let orient = (layout && layout.orientation) || 'horizontal';
                        let side = (layout && layout.side) || 'bottom';
                        if (orient !== 'vertical') orient = 'horizontal';
                        if (!['top', 'bottom', 'left', 'right'].includes(side)) side = 'bottom';
                        // Keep the side consistent with the orientation.
                        if (orient === 'vertical' && (side === 'top' || side === 'bottom')) side = 'right';
                        if (orient === 'horizontal' && (side === 'left' || side === 'right')) side = 'bottom';
                        // Restore the persisted thickness per orientation. A value
                        // of 0 (or missing) means "split in half" and is resolved
                        // inside applySearchPaneLayout.
                        if (cfg.search_pane_height && cfg.search_pane_height > 50) {
                            searchThicknessH = cfg.search_pane_height;
                        }
                        if (cfg.search_pane_width && cfg.search_pane_width > 50) {
                            searchThicknessV = cfg.search_pane_width;
                        }
                        applySearchPaneLayout(orient, side);
                    } catch (layoutErr) {
                        console.error('Failed to load search pane layout:', layoutErr);
                        showToast(String((layoutErr as any)?.message ?? layoutErr), 'error');
                        if (cfg.search_pane_height && cfg.search_pane_height > 50) {
                            searchThicknessH = cfg.search_pane_height;
                            applySearchPaneLayout(searchOrientation, searchSide);
                        }
                    }
                    const args = (cfg as any).startup_args;
                    if (Array.isArray(args) && args.length > 0) {
                        pendingStartupArgs = args as string[];
                    }
                    // Shared settings.json font size (fallback: localStorage).
                    const fontPx = (cfg as any).log_font_size
                        ? String((cfg as any).log_font_size)
                        : (localStorage.getItem('gofle.logFontSize') || '13');
                    applyLogFontSize(fontPx);
                    applyI18n();
                }
            } catch (err) {
                console.error('Failed to load settings:', err);
                showToast(String((err as any)?.message ?? err), 'error');
            }
        }
    };
    await loadSettings();

    // D1: restore the previous tab-bar structure (order, groups, collapsed,
    // active tab) when "save tabs on exit" is enabled. RestoreTab preserves the
    // persisted tab ids, so group membership and strip order stay valid.
    const restoreSavedSession = async () => {
        if (ctx.tabs.size > 0) return;
        const api = getApi();
        if (!api || !api.GetSavedSession) return;

        let sess: any;
        try {
            sess = await api.GetSavedSession();
        } catch (err) {
            console.error('GetSavedSession failed:', err);
            showToast(String((err as any)?.message ?? err), 'error');
            return;
        }
        if (!sess || !sess.tabs || sess.tabs.length === 0) return;

        sessionRestoring = true;
        try {
            const restored = new Set<string>();
            for (const item of sess.tabs) {
                try {
                    const stats = await api.RestoreTab(item);
                    if (!stats) continue;
                    tabManager.addOrUpdateTab(stats);
                    const tab = ctx.tabs.get(stats.tab_id);
                    if (tab) {
                        tab.title = item.title || '';
                        tab.groupID = item.group_id || null;
                        tab.isSSH = !!item.is_ssh;
                        if (item.ssh) tab.ssh = item.ssh;
                    }
                    restored.add(stats.tab_id);
                } catch (err) {
                    console.error('Failed to restore tab:', item.path, err);
                    // A silent skip is indistinguishable from "nothing was ever
                    // saved" — surface every failed restore to the user.
                    showToast(t('err_restore_tab_failed', {
                        path: item.path,
                        err: String((err as any)?.message ?? err),
                    }), 'error');
                }
            }

            if (restored.size === 0) return;

            // Rebuild groups, dropping members that failed to reopen.
            ctx.groups.clear();
            (sess.groups || []).forEach((g: any) => {
                const memberIds = (g.member_ids || []).filter((id: string) => restored.has(id));
                if (memberIds.length === 0) return;
                ctx.groups.set(g.id, {
                    id: g.id,
                    title: g.title,
                    color: g.color,
                    collapsed: !!g.collapsed,
                    memberIds,
                });
            });
            ctx.tabs.forEach(tab => {
                if (tab.groupID && !ctx.groups.has(tab.groupID)) tab.groupID = null;
            });

            // Rebuild the strip; anything missing from top_level is appended so no
            // restored tab can become invisible.
            const seen = new Set<string>();
            const strip: StripEntry[] = [];
            (sess.top_level || []).forEach((e: any) => {
                if (e.type === 'group') {
                    if (!ctx.groups.has(e.id) || seen.has(e.id)) return;
                    seen.add(e.id);
                    strip.push({ type: 'group', id: e.id });
                    return;
                }
                const tab = ctx.tabs.get(e.id);
                if (!tab || tab.groupID || seen.has(e.id)) return;
                seen.add(e.id);
                strip.push({ type: 'tab', id: e.id });
            });
            ctx.groups.forEach(g => {
                if (!seen.has(g.id)) {
                    seen.add(g.id);
                    strip.push({ type: 'group', id: g.id });
                }
            });
            ctx.tabs.forEach(tab => {
                if (!tab.groupID && !seen.has(tab.id)) {
                    seen.add(tab.id);
                    strip.push({ type: 'tab', id: tab.id });
                }
            });
            ctx.strip = strip;

            // Restore the ACTIVE selection through the full switch path so
            // scrollers/content/UI reflect it (a bare ctx.activeTabId write
            // left the app looking like nothing was selected). If the saved
            // active tab failed to restore (e.g. the file is gone), the first
            // available restored tab takes over.
            const activeId = sess.active_tab_id && ctx.tabs.has(sess.active_tab_id)
                ? sess.active_tab_id
                : tabManager.orderedTabIds()[0] || null;
            tabManager.autoExpandActiveGroup();
            if (activeId) {
                // A failure here must not skip the trailing renderTabs — the
                // strip is already rebuilt and must stay visible.
                try {
                    await tabManager.switchTab(activeId);
                } catch (err) {
                    console.error('Failed to activate restored tab:', activeId, err);
                    showToast(String((err as any)?.message ?? err), 'error');
                }
            }
        } finally {
            sessionRestoring = false;
        }

        tabManager.renderTabs();
        updateUIState();
        // Persist the post-restore structure immediately (no debounce) so a
        // quick quit right after launch cannot lose the session again.
        if (ctx.tabs.size > 0) {
            pushSession();
        }
    };

    await restoreSavedSession();

    if (pendingStartupArgs.length > 0) {
        const api = getApi();
        if (api) {
            for (const p of pendingStartupArgs) {
                try {
                    const stats = await api.OpenFile(p);
                    if (stats) tabManager.addOrUpdateTab(stats);
                } catch (e) {
                    console.error('Failed to open startup file:', p, e);
                    showToast(t('err_restore_tab_failed', { path: p, err: String((e as any)?.message ?? e) }), 'error');
                }
            }
        }
    }

    applyI18n();
    applyLogFontSize(localStorage.getItem('gofle.logFontSize') || '13');
});
