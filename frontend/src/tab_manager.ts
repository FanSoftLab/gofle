import { t } from './i18n';
import { showToast } from './toast';
import type { AppContext, TabData, TabGroup, StripEntry } from './app_context';
import { getPrefs } from './settings_manager';

// genId returns a collision-resistant unique id. crypto.randomUUID is preferred
// when available (secure context / WebView); otherwise we fall back to a
// time+random combo so group ids never collide the way Date.now() could.
const genId = (): string => {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID();
    }
    return `id_${Date.now()}_${Math.random().toString(16).slice(2)}`;
};

const GROUP_COLORS = ['#0e639c', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6'];

type DragPayload = { kind: 'tab' | 'group'; id: string };

const closeIconSvg = '<svg class="icon" viewBox="0 0 24 24"><path d="M18 6 6 18"/><path d="M6 6l12 12"/></svg>';

export class TabManager {
    private ctx: AppContext;

    /**
     * The in-flight drag. Kept in memory instead of relying solely on
     * DataTransfer, because WebKit WebViews do not expose dataTransfer payloads
     * during `dragover` (only on `drop`), which we need for live indicators.
     */
    private drag: DragPayload | null = null;

    /** Group being edited by the group modal, and what to do after saving. */
    private pendingGroupTabId: string | null = null;
    private renamingTabId: string | null = null;

    constructor(ctx: AppContext) {
        this.ctx = ctx;
        this.initGroupColorPicker();
    }

    // ---------------------------------------------------------------- structure

    /** Tab ids in visual left-to-right order, including tabs of collapsed groups. */
    public orderedTabIds = (): string[] => {
        const out: string[] = [];
        this.ctx.strip.forEach(entry => {
            if (entry.type === 'tab') {
                if (this.ctx.tabs.has(entry.id)) out.push(entry.id);
                return;
            }
            const grp = this.ctx.groups.get(entry.id);
            if (grp) grp.memberIds.forEach(id => { if (this.ctx.tabs.has(id)) out.push(id); });
        });
        return out;
    };

    /** Index in `strip` of the entry that visually contains the given tab. */
    private topLevelIndexOfTab = (tabId: string): number => {
        const tab = this.ctx.tabs.get(tabId);
        if (tab?.groupID) {
            return this.ctx.strip.findIndex(e => e.type === 'group' && e.id === tab.groupID);
        }
        return this.ctx.strip.findIndex(e => e.type === 'tab' && e.id === tabId);
    };

    /** Removes a tab from wherever it currently lives (group or strip). */
    private detachTab = (tabId: string) => {
        const tab = this.ctx.tabs.get(tabId);
        if (tab?.groupID) {
            const grp = this.ctx.groups.get(tab.groupID);
            if (grp) grp.memberIds = grp.memberIds.filter(id => id !== tabId);
        }
        this.ctx.strip = this.ctx.strip.filter(e => !(e.type === 'tab' && e.id === tabId));
    };

    /** Inserts an entry before `ref`, or appends when ref is null/gone. */
    private insertEntry = (entry: StripEntry, ref: StripEntry | null) => {
        const idx = ref ? this.ctx.strip.indexOf(ref) : -1;
        if (idx < 0) this.ctx.strip.push(entry);
        else this.ctx.strip.splice(idx, 0, entry);
    };

    /** Deletes groups that no longer hold any tab, including their strip entry. */
    public pruneEmptyGroups = () => {
        const dead: string[] = [];
        this.ctx.groups.forEach(grp => {
            grp.memberIds = grp.memberIds.filter(id => this.ctx.tabs.has(id));
            if (grp.memberIds.length === 0) dead.push(grp.id);
        });
        dead.forEach(id => this.ctx.groups.delete(id));
        this.ctx.strip = this.ctx.strip.filter(e => {
            if (e.type === 'group') return this.ctx.groups.has(e.id);
            return this.ctx.tabs.has(e.id);
        });
    };

    /** A collapsed group must expand when it holds the active tab (req. №14). */
    public autoExpandActiveGroup = () => {
        const { activeTabId, tabs, groups } = this.ctx;
        if (!activeTabId) return;
        const tab = tabs.get(activeTabId);
        if (!tab?.groupID) return;
        const grp = groups.get(tab.groupID);
        if (grp?.collapsed) grp.collapsed = false;
    };

    public createGroup = (title?: string, color?: string): TabGroup => {
        const id = `group_${genId()}`;
        const grp: TabGroup = {
            id,
            title: title || t('new_group_default_name'),
            color: color || GROUP_COLORS[this.ctx.groups.size % GROUP_COLORS.length],
            collapsed: false,
            memberIds: [],
        };
        this.ctx.groups.set(id, grp);
        return grp;
    };

    /** Moves a tab into a group, optionally before an existing member. */
    public moveTabToGroup = (tabId: string, groupId: string, beforeMemberId?: string | null) => {
        const tab = this.ctx.tabs.get(tabId);
        const grp = this.ctx.groups.get(groupId);
        if (!tab || !grp) return;

        // Remember where the group should sit so a brand-new group keeps the
        // position of the dragged tab. The anchor must be the *next* entry: the
        // tab's own entry is about to be removed by detachTab, which would make an
        // anchor pointing at it stale and push the group to the end of the strip.
        const hadEntry = this.ctx.strip.some(e => e.type === 'group' && e.id === groupId);
        const ownIdx = this.topLevelIndexOfTab(tabId);
        const ref = hadEntry ? null : (this.ctx.strip[ownIdx + 1] ?? null);

        this.detachTab(tabId);
        tab.groupID = groupId;

        const at = beforeMemberId ? grp.memberIds.indexOf(beforeMemberId) : -1;
        if (at < 0) grp.memberIds.push(tabId);
        else grp.memberIds.splice(at, 0, tabId);

        if (!hadEntry) this.insertEntry({ type: 'group', id: groupId }, ref);

        this.pruneEmptyGroups();
    };

    /** Takes a tab out of its group and places it standalone in the strip. */
    public ungroupTab = (tabId: string, beforeEntry: StripEntry | null = null) => {
        const tab = this.ctx.tabs.get(tabId);
        if (!tab) return;
        this.detachTab(tabId);
        tab.groupID = null;
        this.insertEntry({ type: 'tab', id: tabId }, beforeEntry);
        this.pruneEmptyGroups();
    };

    /** Reorders a group inside the strip. */
    public moveGroup = (groupId: string, beforeEntry: StripEntry | null) => {
        const entry = this.ctx.strip.find(e => e.type === 'group' && e.id === groupId);
        if (!entry || entry === beforeEntry) return;
        this.ctx.strip = this.ctx.strip.filter(e => e !== entry);
        this.insertEntry(entry, beforeEntry);
    };

    /** Shifts the active tab one slot left/right within its own container. */
    private shiftTab = (tabId: string, delta: number) => {
        const tab = this.ctx.tabs.get(tabId);
        if (!tab) return;

        if (tab.groupID) {
            const grp = this.ctx.groups.get(tab.groupID);
            if (!grp) return;
            const i = grp.memberIds.indexOf(tabId);
            const j = i + delta;
            if (i < 0 || j < 0 || j >= grp.memberIds.length) return;
            [grp.memberIds[i], grp.memberIds[j]] = [grp.memberIds[j], grp.memberIds[i]];
        } else {
            const i = this.ctx.strip.findIndex(e => e.type === 'tab' && e.id === tabId);
            const j = i + delta;
            if (i < 0 || j < 0 || j >= this.ctx.strip.length) return;
            [this.ctx.strip[i], this.ctx.strip[j]] = [this.ctx.strip[j], this.ctx.strip[i]];
        }

        this.renderTabs();
        this.ctx.saveSession();
    };

    // ------------------------------------------------------------------ render

    public renderTabs = () => {
        const { tabListEl } = this.ctx;

        this.pruneEmptyGroups();
        this.autoExpandActiveGroup();

        tabListEl.innerHTML = '';

        this.ctx.strip.forEach(entry => {
            if (entry.type === 'tab') {
                const el = this.renderTabEl(entry.id, null);
                if (el) tabListEl.appendChild(el);
                return;
            }

            const grp = this.ctx.groups.get(entry.id);
            if (!grp) return;

            const container = document.createElement('div');
            container.className = `tab-group${grp.collapsed ? ' collapsed' : ''}`;
            container.dataset.groupId = grp.id;
            container.style.setProperty('--group-color', grp.color);

            const header = document.createElement('div');
            header.className = 'tab-group-header';
            header.draggable = true;
            header.dataset.groupId = grp.id;
            header.style.backgroundColor = grp.color;
            header.title = grp.title;

            const chevron = document.createElement('span');
            chevron.className = 'tab-group-chevron';
            chevron.textContent = grp.collapsed ? '▸' : '▾';
            header.appendChild(chevron);

            const name = document.createElement('span');
            name.className = 'tab-group-name';
            name.textContent = grp.title;
            header.appendChild(name);

            const count = document.createElement('span');
            count.className = 'tab-group-count';
            count.textContent = String(grp.memberIds.length);
            header.appendChild(count);

            header.addEventListener('click', () => {
                grp.collapsed = !grp.collapsed;
                this.renderTabs();
                this.ctx.saveSession();
            });
            header.addEventListener('contextmenu', (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.openGroupContextMenu(grp.id, e as MouseEvent);
            });
            header.addEventListener('dragstart', (e) => this.onDragStart(e as DragEvent, { kind: 'group', id: grp.id }));
            header.addEventListener('dragend', this.onDragEnd);
            header.addEventListener('dragover', (e) => this.onDragOverGroupHeader(e as DragEvent, grp.id));
            header.addEventListener('dragleave', () => this.clearDropMarkers());
            header.addEventListener('drop', (e) => this.onDropOnGroupHeader(e as DragEvent, grp.id));

            container.appendChild(header);

            if (!grp.collapsed) {
                const body = document.createElement('div');
                body.className = 'tab-group-body';
                grp.memberIds.forEach(id => {
                    const el = this.renderTabEl(id, grp);
                    if (el) body.appendChild(el);
                });
                body.addEventListener('dragover', (e) => {
                    // Dropping on the padding of an expanded group appends to it.
                    if (!this.drag || this.drag.kind !== 'tab') return;
                    e.preventDefault();
                });
                body.addEventListener('drop', (e) => this.onDropOnGroupHeader(e as DragEvent, grp.id));
                container.appendChild(body);
            }

            tabListEl.appendChild(container);
        });
    };

    private renderTabEl = (id: string, group: TabGroup | null): HTMLElement | null => {
        const tab = this.ctx.tabs.get(id);
        if (!tab) return null;

        const tabEl = document.createElement('div');
        tabEl.className = `tab-item${id === this.ctx.activeTabId ? ' active' : ''}${group ? ' in-group' : ''}`;
        tabEl.dataset.id = id;
        tabEl.draggable = true;
        if (group) tabEl.style.setProperty('--group-color', group.color);

        const titleSpan = document.createElement('span');
        titleSpan.className = 'tab-title';
        titleSpan.textContent = tab.title || tab.fileName;
        titleSpan.title = tab.filePath;
        tabEl.appendChild(titleSpan);

        const closeBtn = document.createElement('span');
        closeBtn.className = 'tab-close';
        closeBtn.innerHTML = closeIconSvg;
        closeBtn.title = t('tab_close_title');
        closeBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.closeTab(id);
        });
        tabEl.appendChild(closeBtn);

        tabEl.addEventListener('click', () => this.switchTab(id));
        tabEl.addEventListener('contextmenu', (e) => {
            e.preventDefault();
            e.stopPropagation();
            this.openTabContextMenu(id, e as MouseEvent);
        });
        tabEl.addEventListener('dragstart', (e) => this.onDragStart(e as DragEvent, { kind: 'tab', id }));
        tabEl.addEventListener('dragend', this.onDragEnd);
        tabEl.addEventListener('dragover', (e) => this.onDragOverTab(e as DragEvent, tabEl));
        tabEl.addEventListener('dragleave', () => this.clearDropMarkers());
        tabEl.addEventListener('drop', (e) => this.onDropOnTab(e as DragEvent, id, tabEl));

        return tabEl;
    };

    // --------------------------------------------------------------------- DnD

    /** Enables reordering by dropping onto the free space of the tab strip. */
    public initDragAndDrop = () => {
        const { tabListEl } = this.ctx;
        tabListEl.addEventListener('dragover', (e) => {
            if (!this.drag) return;
            e.preventDefault();
        });
        tabListEl.addEventListener('drop', (e) => {
            if (!this.drag) return;
            const target = e.target as HTMLElement;
            // Element-level handlers already dealt with precise targets.
            if (target.closest('.tab-item') || target.closest('.tab-group')) return;
            e.preventDefault();
            const payload = this.drag;
            this.clearDropMarkers();
            if (payload.kind === 'tab') this.ungroupTab(payload.id, null);
            else this.moveGroup(payload.id, null);
            this.finishDrop();
        });
    };

    private onDragStart = (e: DragEvent, payload: DragPayload) => {
        this.drag = payload;
        if (e.dataTransfer) {
            e.dataTransfer.effectAllowed = 'move';
            try {
                e.dataTransfer.setData('text/plain', JSON.stringify(payload));
            } catch {
                /* WebView may reject custom payloads; this.drag is authoritative. */
            }
        }
        (e.currentTarget as HTMLElement).classList.add('dragging');
    };

    private onDragEnd = () => {
        this.drag = null;
        this.clearDropMarkers();
        this.ctx.tabListEl.querySelectorAll('.dragging').forEach(el => el.classList.remove('dragging'));
    };

    private clearDropMarkers = () => {
        // Scoped to the tab strip: this runs on every `dragover` event, so a
        // document-wide query would scale with the whole log DOM.
        this.ctx.tabListEl.querySelectorAll('.drop-before, .drop-after, .drop-into').forEach(el => {
            el.classList.remove('drop-before', 'drop-after', 'drop-into');
        });
    };

    private isAfter = (e: DragEvent, el: HTMLElement): boolean => {
        const rect = el.getBoundingClientRect();
        return e.clientX > rect.left + rect.width / 2;
    };

    private onDragOverTab = (e: DragEvent, tabEl: HTMLElement) => {
        if (!this.drag) return;
        e.preventDefault();
        e.stopPropagation();
        this.clearDropMarkers();
        tabEl.classList.add(this.isAfter(e, tabEl) ? 'drop-after' : 'drop-before');
    };

    private onDragOverGroupHeader = (e: DragEvent, groupId: string) => {
        if (!this.drag) return;
        if (this.drag.kind === 'group' && this.drag.id === groupId) return;
        e.preventDefault();
        e.stopPropagation();
        this.clearDropMarkers();
        const headerEl = e.currentTarget as HTMLElement;
        headerEl.classList.add(this.drag.kind === 'tab' ? 'drop-into' : (this.isAfter(e, headerEl) ? 'drop-after' : 'drop-before'));
    };

    private onDropOnTab = (e: DragEvent, targetTabId: string, tabEl: HTMLElement) => {
        if (!this.drag) return;
        e.preventDefault();
        e.stopPropagation();

        const payload = this.drag;
        const after = this.isAfter(e, tabEl);
        this.clearDropMarkers();

        if (payload.id === targetTabId && payload.kind === 'tab') {
            this.finishDrop();
            return;
        }

        const targetTab = this.ctx.tabs.get(targetTabId);
        const targetGroupId = targetTab?.groupID || null;

        if (payload.kind === 'group') {
            const idx = this.topLevelIndexOfTab(targetTabId);
            const ref = after ? (this.ctx.strip[idx + 1] ?? null) : (this.ctx.strip[idx] ?? null);
            this.moveGroup(payload.id, ref);
            this.finishDrop();
            return;
        }

        if (targetGroupId) {
            const grp = this.ctx.groups.get(targetGroupId)!;
            const at = grp.memberIds.indexOf(targetTabId);
            const beforeId = after ? (grp.memberIds[at + 1] ?? null) : targetTabId;
            this.moveTabToGroup(payload.id, targetGroupId, beforeId);
        } else {
            const idx = this.topLevelIndexOfTab(targetTabId);
            const ref = after ? (this.ctx.strip[idx + 1] ?? null) : (this.ctx.strip[idx] ?? null);
            this.ungroupTab(payload.id, ref);
        }

        this.finishDrop();
    };

    private onDropOnGroupHeader = (e: DragEvent, groupId: string) => {
        if (!this.drag) return;
        e.preventDefault();
        e.stopPropagation();

        const payload = this.drag;
        const after = this.isAfter(e, e.currentTarget as HTMLElement);
        this.clearDropMarkers();

        if (payload.kind === 'group') {
            if (payload.id !== groupId) {
                const idx = this.ctx.strip.findIndex(en => en.type === 'group' && en.id === groupId);
                const ref = after ? (this.ctx.strip[idx + 1] ?? null) : (this.ctx.strip[idx] ?? null);
                this.moveGroup(payload.id, ref);
            }
        } else {
            // Dropping a tab on a group header appends it to that group (req. №7).
            this.moveTabToGroup(payload.id, groupId, null);
        }

        this.finishDrop();
    };

    /** Common tail of every drop: no tab reload, just re-render and persist. */
    private finishDrop = () => {
        this.drag = null;
        this.renderTabs();
        this.ctx.saveSession();
    };

    // ---------------------------------------------------------------- tab ops

    /**
     * Closes a batch of tabs with a single re-render / save at the end. Closing
     * one tab at a time through closeTab would rebuild the whole strip and reset
     * the save debounce once per tab.
     */
    private closeTabs = async (ids: string[]) => {
        if (ids.length === 0) return;

        const api = this.ctx.getApi();
        const order = this.orderedTabIds();
        const firstIdx = Math.min(...ids.map(id => order.indexOf(id)).filter(i => i >= 0));
        const closingActive = !!this.ctx.activeTabId && ids.includes(this.ctx.activeTabId);

        for (const id of ids) {
            const tab = this.ctx.tabs.get(id);
            if (!tab) continue;
            if (!this.suppressReopen && !tab.isSSH && this.reopenStack[this.reopenStack.length - 1] !== tab.filePath) {
                this.reopenStack.push(tab.filePath);
                if (this.reopenStack.length > 12) this.reopenStack.shift();
            }
            if (api) await api.CloseTab(id);
            this.detachTab(id);
            this.ctx.tabs.delete(id);
        }
        this.pruneEmptyGroups();

        if (closingActive) {
            const remaining = this.orderedTabIds();
            const anchor = Number.isFinite(firstIdx) ? Math.max(firstIdx, 0) : 0;
            const next = remaining.length > 0
                ? remaining[Math.min(anchor, remaining.length - 1)]
                : undefined;
            this.ctx.activeTabId = next ?? null;
            if (this.ctx.activeTabId && api) api.SetActiveTab(this.ctx.activeTabId);
        }

        this.renderTabs();
        this.ctx.updateUIState();
        this.ctx.saveSession();
    };

    public closeTab = async (tabId: string) => {
        const tab = this.ctx.tabs.get(tabId);
        if (tab && getPrefs().confirmCloseTab && tab.isSSH) {
            if (!confirm(t('confirm_close_remote_tab') || 'Close remote tab?')) return;
        }
        await this.closeTabs([tabId]);
    };

    // Recently closed LOCAL tabs (most recent last) for Ctrl+Shift+T.
    private reopenStack: string[] = [];
    private suppressReopen = false;

    /** Reopens the most recently closed local tab, if any. */
    public reopenClosed = async () => {
        const path = this.reopenStack.pop();
        if (!path) return;
        const api = this.ctx.getApi();
        if (!api) return;
        try {
            const stats = await api.OpenFile(path);
            if (stats) this.ctx.addOrUpdateTab(stats);
        } catch (err) {
            console.error('Reopen closed tab failed:', err);
            showToast(String((err as any)?.message ?? err), 'error');
        }
    };

    public closeAllTabs = async () => {
        const ids = this.orderedTabIds();
        if (ids.length === 0) return;
        this.suppressReopen = true;
        if (!confirm(t('confirm_close_all'))) return;

        const api = this.ctx.getApi();
        for (const id of ids) {
            if (api) await api.CloseTab(id);
            this.ctx.tabs.delete(id);
        }
        this.ctx.strip = [];
        this.ctx.groups.clear();
        this.ctx.activeTabId = null;

        this.renderTabs();
        this.ctx.updateUIState();
        this.ctx.saveSession();
    };

    private closeGroup = async (groupId: string) => {
        const grp = this.ctx.groups.get(groupId);
        if (!grp) return;
        if (!confirm(t('confirm_close_group', { name: grp.title }))) return;
        // pruneEmptyGroups (inside closeTabs) drops the now-empty group.
        await this.closeTabs([...grp.memberIds]);
    };

    /**
     * Ungroups every member and removes the group, keeping the tabs open. Shared
     * by the group modal's delete button and the groups panel so the two entry
     * points can never drift apart.
     */
    public deleteGroup = (groupId: string) => {
        const grp = this.ctx.groups.get(groupId);
        if (!grp) return;

        const entry = this.ctx.strip.find(e => e.type === 'group' && e.id === groupId) || null;
        const ref = entry ? (this.ctx.strip[this.ctx.strip.indexOf(entry) + 1] ?? null) : null;
        [...grp.memberIds].forEach(tabId => this.ungroupTab(tabId, ref));

        this.ctx.groups.delete(groupId);
        this.ctx.strip = this.ctx.strip.filter(e => !(e.type === 'group' && e.id === groupId));

        this.renderTabs();
        this.ctx.saveSession();
    };

    private closeOtherTabs = async (keepId: string) => {
        await this.closeTabs(this.orderedTabIds().filter(id => id !== keepId));
    };

    private closeTabsToTheRight = async (fromId: string) => {
        const order = this.orderedTabIds();
        const idx = order.indexOf(fromId);
        if (idx < 0) return;
        await this.closeTabs(order.slice(idx + 1));
    };

    public addOrUpdateTab = (stats: any) => {
        if (!stats) return;
        const { tabs } = this.ctx;

        if (tabs.has(stats.tab_id)) {
            const tab = tabs.get(stats.tab_id)!;
            tab.totalLines = stats.total_lines;
            tab.matchedLines = stats.matched_lines;
            tab.fileSize = stats.file_size;
            if (stats.ssh) tab.ssh = stats.ssh;
            this.switchTab(stats.tab_id);
            return;
        }

        // The backend reports SSH tabs explicitly and hands back the connection
        // summary; it must be kept on the tab so the session snapshot can restore
        // it after a restart. The path prefix is only a legacy fallback.
        const isSSH = !!stats.is_ssh || (typeof stats.file_path === 'string' && stats.file_path.startsWith('ssh://'));

        tabs.set(stats.tab_id, {
            id: stats.tab_id,
            filePath: stats.file_path,
            fileName: stats.file_name,
            groupID: null,
            isSSH,
            ssh: stats.ssh || undefined,
            totalLines: stats.total_lines,
            matchedLines: stats.matched_lines,
            fileSize: stats.file_size,
            autoScroll: getPrefs().defaultFollow,
            searchQuery: '',
            secondaryQuery: '',
            isRegex: getPrefs().defaultSearchRegex,
            caseSensitive: getPrefs().defaultSearchCase,
            invertMatch: getPrefs().defaultSearchInvert,
            currentMatchIndex: -1,
            resultsOnly: false,
            mainScrollIndex: 0,
            mainScrollFrac: 0,
            filterScrollIndex: 0,
            filterScrollFrac: 0,
        });

        if (!this.ctx.strip.some(e => e.type === 'tab' && e.id === stats.tab_id)) {
            this.ctx.strip.push({ type: 'tab', id: stats.tab_id });
        }

        this.switchTab(stats.tab_id);
        this.ctx.saveSession();
    };

    public switchTab = (tabId: string) => {
        // Persist scroll positions of the tab we are leaving.
        const prevId = this.ctx.activeTabId;
        if (prevId && prevId !== tabId && this.ctx.tabs.has(prevId)) {
            const prev = this.ctx.tabs.get(prevId)!;
            const mainA = this.ctx.mainScroller.getViewAnchor();
            const filterA = this.ctx.filterScroller.getViewAnchor();
            prev.mainScrollIndex = mainA.index;
            prev.mainScrollFrac = mainA.fraction;
            prev.filterScrollIndex = filterA.index;
            prev.filterScrollFrac = filterA.fraction;
        }
        this.ctx.activeTabId = tabId;
        // Only restore scroll on an actual tab switch (not on every UI refresh).
        (this.ctx as any).requestScrollRestore?.();
        this.ctx.mainScroller.setHighlightedLine(null);
        this.ctx.filterScroller.setHighlightedLine(null);
        const api = this.ctx.getApi();
        if (api) api.SetActiveTab(tabId);
        // Restore per-tab results-only layout before updateUIState refreshes the badge.
        const tab = this.ctx.tabs.get(tabId);
        this.ctx.mainContainer.classList.toggle('results-only', !!tab?.resultsOnly);
        this.autoExpandActiveGroup();
        this.renderTabs();
        this.ctx.updateUIState();
        this.ctx.saveSession();
    };

    // ------------------------------------------------------------ context menus

    public hideTabMenus = () => {
        this.ctx.ctxMenuTab.classList.add('hidden');
        this.ctx.ctxMenuGroup.classList.add('hidden');
    };

    private showMenu = (menu: HTMLElement, e: MouseEvent) => {
        menu.classList.remove('hidden');
        // Keep the menu inside the viewport.
        const rect = menu.getBoundingClientRect();
        const left = Math.min(e.clientX, window.innerWidth - rect.width - 4);
        const top = Math.min(e.clientY, window.innerHeight - rect.height - 4);
        menu.style.left = `${Math.max(0, left)}px`;
        menu.style.top = `${Math.max(0, top)}px`;
    };

    private menuItem = (label: string, onClick: () => void): HTMLElement => {
        const el = document.createElement('div');
        el.className = 'ctx-item';
        el.textContent = label;
        el.addEventListener('click', (e) => {
            e.stopPropagation();
            this.hideTabMenus();
            onClick();
        });
        return el;
    };

    private divider = (): HTMLElement => {
        const el = document.createElement('div');
        el.className = 'ctx-divider';
        return el;
    };

    private openTabContextMenu = (tabId: string, e: MouseEvent) => {
        const menu = this.ctx.ctxMenuTab;
        const tab = this.ctx.tabs.get(tabId);
        if (!tab) return;

        menu.innerHTML = '';

        menu.appendChild(this.menuItem(t('ctx_tab_new'), () => this.ctx.openFilesAction()));
        menu.appendChild(this.divider());

        // "Move to Group" with a hover submenu of every existing group + New Group.
        const moveItem = document.createElement('div');
        moveItem.className = 'ctx-item ctx-has-submenu';
        const moveLabel = document.createElement('span');
        moveLabel.textContent = t('ctx_tab_move_to_group');
        moveItem.appendChild(moveLabel);
        const arrow = document.createElement('span');
        arrow.className = 'ctx-submenu-arrow';
        arrow.textContent = '▸';
        moveItem.appendChild(arrow);

        const submenu = document.createElement('div');
        submenu.className = 'ctx-submenu';
        this.ctx.groups.forEach(grp => {
            if (grp.id === tab.groupID) return;
            const item = document.createElement('div');
            item.className = 'ctx-submenu-item';
            const swatch = document.createElement('span');
            swatch.className = 'group-swatch';
            swatch.style.backgroundColor = grp.color;
            item.appendChild(swatch);
            const nameEl = document.createElement('span');
            nameEl.textContent = grp.title;
            item.appendChild(nameEl);
            item.addEventListener('click', (ev) => {
                ev.stopPropagation();
                this.hideTabMenus();
                this.moveTabToGroup(tabId, grp.id, null);
                this.renderTabs();
                this.ctx.saveSession();
            });
            submenu.appendChild(item);
        });
        const newGroupItem = document.createElement('div');
        newGroupItem.className = 'ctx-submenu-item';
        newGroupItem.textContent = t('ctx_tab_new_group');
        newGroupItem.addEventListener('click', (ev) => {
            ev.stopPropagation();
            this.hideTabMenus();
            this.pendingGroupTabId = tabId;
            this.openGroupModal();
        });
        submenu.appendChild(newGroupItem);
        moveItem.appendChild(submenu);
        menu.appendChild(moveItem);

        if (tab.groupID) {
            menu.appendChild(this.menuItem(t('ctx_tab_remove_from_group'), () => {
                this.ungroupTab(tabId, null);
                this.renderTabs();
                this.ctx.saveSession();
            }));
        }

        menu.appendChild(this.divider());
        menu.appendChild(this.menuItem(t('ctx_tab_rename'), () => this.openTabRenameModal(tabId)));
        menu.appendChild(this.menuItem(t('ctx_tab_move_left'), () => this.shiftTab(tabId, -1)));
        menu.appendChild(this.menuItem(t('ctx_tab_move_right'), () => this.shiftTab(tabId, 1)));
        if (tab.isSSH && tab.ssh) {
            menu.appendChild(this.divider());
            menu.appendChild(this.menuItem(t('ctx_tab_browse_server'), () => {
                void this.ctx.browseRemoteServerAction(tabId);
            }));
        }
        menu.appendChild(this.divider());
        menu.appendChild(this.menuItem(t('ctx_tab_close'), () => this.closeTab(tabId)));
        menu.appendChild(this.menuItem(t('ctx_tab_close_others'), () => this.closeOtherTabs(tabId)));
        menu.appendChild(this.menuItem(t('ctx_tab_close_right'), () => this.closeTabsToTheRight(tabId)));

        this.ctx.ctxMenuGroup.classList.add('hidden');
        this.showMenu(menu, e);
    };

    private openGroupContextMenu = (groupId: string, e: MouseEvent) => {
        const grp = this.ctx.groups.get(groupId);
        if (!grp) return;

        const menu = this.ctx.ctxMenuGroup;
        menu.innerHTML = '';

        menu.appendChild(this.menuItem(t('ctx_group_rename'), () => this.openGroupModal(groupId)));
        menu.appendChild(this.menuItem(t('ctx_group_color'), () => this.openGroupModal(groupId)));
        menu.appendChild(this.menuItem(grp.collapsed ? t('ctx_group_expand') : t('ctx_group_collapse'), () => {
            grp.collapsed = !grp.collapsed;
            this.renderTabs();
            this.ctx.saveSession();
        }));
        menu.appendChild(this.divider());
        menu.appendChild(this.menuItem(t('ctx_group_close'), () => this.closeGroup(groupId)));

        this.ctx.ctxMenuTab.classList.add('hidden');
        this.showMenu(menu, e);
    };

    // ------------------------------------------------------------------ modals

    private openTabRenameModal = (tabId: string) => {
        const tab = this.ctx.tabs.get(tabId);
        if (!tab) return;
        this.renamingTabId = tabId;
        this.ctx.tabRenameInput.value = tab.title || tab.fileName;
        this.ctx.tabRenameModal.classList.remove('hidden');
        this.ctx.tabRenameInput.focus();
    };

    public openGroupModal = (groupId?: string) => {
        const { groupModal, groupNameInput, btnDeleteGroup } = this.ctx;
        this.ctx.selectedGroupId = groupId || null;

        let color = GROUP_COLORS[0];
        if (this.ctx.selectedGroupId && this.ctx.groups.has(this.ctx.selectedGroupId)) {
            const grp = this.ctx.groups.get(this.ctx.selectedGroupId)!;
            groupNameInput.value = grp.title;
            color = grp.color;
            btnDeleteGroup.classList.remove('hidden');
        } else {
            groupNameInput.value = t('new_group_default_name');
            btnDeleteGroup.classList.add('hidden');
        }

        this.selectColorCircle(color);
        groupModal.classList.remove('hidden');
        groupNameInput.focus();
    };

    private selectColorCircle = (color: string) => {
        const circles = document.querySelectorAll('#group-color-picker .color-circle');
        let matched = false;
        circles.forEach(c => {
            const isMatch = (c as HTMLElement).dataset.color === color;
            c.classList.toggle('selected', isMatch);
            if (isMatch) matched = true;
        });
        if (!matched && circles.length > 0) circles[0].classList.add('selected');
    };

    private selectedColor = (): string => {
        const el = document.querySelector('#group-color-picker .color-circle.selected') as HTMLElement | null;
        return el?.dataset.color || GROUP_COLORS[0];
    };

    public initGroupModalHandlers = () => {
        const {
            btnCancelGroup, btnSaveGroup, btnDeleteGroup,
            btnSaveTabRename, btnCancelTabRename,
        } = this.ctx;

        btnCancelGroup.addEventListener('click', () => {
            this.pendingGroupTabId = null;
            this.ctx.groupModal.classList.add('hidden');
        });

        btnSaveGroup.addEventListener('click', () => {
            const name = this.ctx.groupNameInput.value.trim();
            if (!name) return;
            const color = this.selectedColor();

            if (this.ctx.selectedGroupId && this.ctx.groups.has(this.ctx.selectedGroupId)) {
                const grp = this.ctx.groups.get(this.ctx.selectedGroupId)!;
                grp.title = name;
                grp.color = color;
            } else {
                const grp = this.createGroup(name, color);
                // A brand-new group only survives if a tab joins it (empty groups
                // are pruned), so only attach the tab the user acted on.
                if (this.pendingGroupTabId && this.ctx.tabs.has(this.pendingGroupTabId)) {
                    this.moveTabToGroup(this.pendingGroupTabId, grp.id, null);
                } else if (!this.ctx.strip.some(e => e.type === 'group' && e.id === grp.id)) {
                    this.ctx.strip.push({ type: 'group', id: grp.id });
                }
            }

            this.pendingGroupTabId = null;
            this.ctx.groupModal.classList.add('hidden');
            this.renderTabs();
            this.ctx.saveSession();
        });

        btnDeleteGroup.addEventListener('click', () => {
            const id = this.ctx.selectedGroupId;
            this.ctx.groupModal.classList.add('hidden');
            // Deleting a group ungroups its tabs (they stay open, req. №20).
            if (id) this.deleteGroup(id);
        });

        btnCancelTabRename.addEventListener('click', () => {
            this.renamingTabId = null;
            this.ctx.tabRenameModal.classList.add('hidden');
        });

        btnSaveTabRename.addEventListener('click', () => {
            const id = this.renamingTabId;
            const name = this.ctx.tabRenameInput.value.trim();
            this.renamingTabId = null;
            this.ctx.tabRenameModal.classList.add('hidden');
            if (!id || !this.ctx.tabs.has(id)) return;
            const tab = this.ctx.tabs.get(id)!;
            tab.title = name;
            const api = this.ctx.getApi();
            if (api && name) api.RenameTab(id, name);
            this.renderTabs();
            this.ctx.saveSession();
        });

        // Any left click outside closes the tab/group context menus.
        document.addEventListener('click', () => this.hideTabMenus());

        this.initDragAndDrop();
    };

    private initGroupColorPicker = () => {
        document.querySelectorAll('#group-color-picker .color-circle').forEach(circle => {
            circle.addEventListener('click', (e) => {
                document.querySelectorAll('#group-color-picker .color-circle').forEach(c => c.classList.remove('selected'));
                (e.currentTarget as HTMLElement).classList.add('selected');
            });
        });
    };
}

export type { TabData, TabGroup };
