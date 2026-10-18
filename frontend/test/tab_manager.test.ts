import { describe, it, expect, beforeEach, vi } from 'vitest';
import { TabManager } from '../src/tab_manager';
import type { AppContext, TabData, TabGroup, StripEntry } from '../src/app_context';

function createTab(id: string, overrides: Partial<TabData> = {}): TabData {
  return {
    id,
    filePath: `/path/${id}`,
    fileName: id,
    groupID: null,
    isSSH: false,
    totalLines: 100,
    matchedLines: 0,
    fileSize: 1024,
    autoScroll: true,
    searchQuery: '',
    secondaryQuery: '',
    secondaryMode: 'or',
    isRegex: false,
    caseSensitive: false,
    invertMatch: false,
    currentMatchIndex: -1,
    resultsOnly: false,
    mainScrollIndex: 0,
    mainScrollFrac: 0,
    filterScrollIndex: 0,
    filterScrollFrac: 0,
    ...overrides,
  };
}

function createGroup(id: string, overrides: Partial<TabGroup> = {}): TabGroup {
  return {
    id,
    title: `Group ${id}`,
    color: '#0e639c',
    collapsed: false,
    memberIds: [],
    ...overrides,
  };
}

function createMockContext(overrides: Record<string, unknown> = {}): AppContext {
  const tabs = new Map<string, TabData>();
  const groups = new Map<string, TabGroup>();
  const strip: StripEntry[] = [];

  const ctx: AppContext = {
    getApi: () => null,
    tabs,
    groups,
    strip,
    activeTabId: null,
    selectedGroupId: null,
    mainScroller: null as unknown as never,
    filterScroller: null as unknown as never,
    panelBottom: null as unknown as never,
    panelTop: null as unknown as never,
    mainContainer: { classList: { toggle: vi.fn() } } as unknown as never,
    btnSearchLayout: null as unknown as never,
    searchLayoutPopover: null as unknown as never,
    searchLayoutIcon: null as unknown as never,
    tabListEl: {
      innerHTML: '',
      appendChild: vi.fn(),
      querySelectorAll: vi.fn(() => []),
    } as unknown as never,
    btnOpen: null as unknown as never,
    btnOpenSSH: null as unknown as never,
    btnGotoLineHeader: null as unknown as never,
    btnSettingsHeader: null as unknown as never,
    btnRecent: null as unknown as never,
    btnFollow: null as unknown as never,
    statusTailText: null as unknown as never,
    searchInput: null as unknown as never,
    searchBoxContainer: null as unknown as never,
    regexErrorIndicator: null as unknown as never,
    btnOptCase: null as unknown as never,
    btnOptRegex: null as unknown as never,
    btnOptInvert: null as unknown as never,
    btnNavFirst: null as unknown as never,
    btnNavPrev: null as unknown as never,
    btnNavNext: null as unknown as never,
    btnNavLast: null as unknown as never,
    btnSaveSearch: null as unknown as never,
    recentModal: null as unknown as never,
    recentFilesList: null as unknown as never,
    btnCloseRecent: null as unknown as never,
    btnClearRecent: null as unknown as never,
    groupModal: null as unknown as never,
    groupNameInput: null as unknown as never,
    btnSaveGroup: null as unknown as never,
    btnCancelGroup: null as unknown as never,
    btnDeleteGroup: null as unknown as never,
    tabRenameModal: null as unknown as never,
    tabRenameInput: null as unknown as never,
    btnSaveTabRename: null as unknown as never,
    btnCancelTabRename: null as unknown as never,
    gotoModal: null as unknown as never,
    gotoLineInput: null as unknown as never,
    errGotoLine: null as unknown as never,
    btnSubmitGoto: null as unknown as never,
    btnCancelGoto: null as unknown as never,
    settingsModal: null as unknown as never,
    settingsThemeSelect: null as unknown as never,
    settingsLangSelect: null as unknown as never,
    settingsSaveTabsCheck: null as unknown as never,
    settingsMaxHistoryInput: null as unknown as never,
    btnSaveSettings: null as unknown as never,
    btnCancelSettings: null as unknown as never,
    ctxMenuMain: null as unknown as never,
    ctxMenuFilter: null as unknown as never,
    ctxMenuTab: { classList: { add: vi.fn() } } as unknown as never,
    ctxMenuGroup: { classList: { add: vi.fn() } } as unknown as never,
    sshModal: null as unknown as never,
    sshAuthSelect: null as unknown as never,
    sshProtocolSelect: null as unknown as never,
    sshTLSSelect: null as unknown as never,
    groupPassword: null as unknown as never,
    groupKeyPath: null as unknown as never,
    groupKeyPass: null as unknown as never,
    errSSH: null as unknown as never,
    updateUIState: vi.fn(),
    triggerSearch: vi.fn(),
    syncSearchCombineUI: vi.fn(),
    loadRecent: vi.fn(),
    addOrUpdateTab: vi.fn(),
    switchTab: vi.fn(),
    saveSearchResults: vi.fn(),
    validateRegexUI: vi.fn(),
    navigateMatch: vi.fn(),
    updateSearchNavState: vi.fn(),
    openGroupModal: vi.fn(),
    saveSession: vi.fn(),
    closeAllTabs: vi.fn(),
    openFilesAction: vi.fn(),
    browseRemoteServerAction: vi.fn(),
    ...(overrides as Record<string, unknown>),
  };
  return ctx;
}

describe('TabManager', () => {
  let ctx: AppContext;
  let tm: TabManager;

  beforeEach(() => {
    ctx = createMockContext();
    tm = new TabManager(ctx);
  });

  describe('orderedTabIds', () => {
    it('returns empty array for empty strip', () => {
      expect(tm.orderedTabIds()).toEqual([]);
    });

    it('returns standalone tab ids in strip order', () => {
      const t1 = createTab('t1');
      const t2 = createTab('t2');
      ctx.tabs.set('t1', t1);
      ctx.tabs.set('t2', t2);
      ctx.strip = [{ type: 'tab', id: 't1' }, { type: 'tab', id: 't2' }];
      expect(tm.orderedTabIds()).toEqual(['t1', 't2']);
    });

    it('skips tabs not in ctx.tabs', () => {
      ctx.tabs.set('t1', createTab('t1'));
      ctx.strip = [{ type: 'tab', id: 't1' }, { type: 'tab', id: 'missing' }];
      expect(tm.orderedTabIds()).toEqual(['t1']);
    });

    it('returns group members in order for group entries', () => {
      const t1 = createTab('t1');
      const t2 = createTab('t2');
      const t3 = createTab('t3');
      ctx.tabs.set('t1', t1);
      ctx.tabs.set('t2', t2);
      ctx.tabs.set('t3', t3);
      ctx.groups.set('g1', createGroup('g1', { memberIds: ['t2', 't1'] }));
      ctx.strip = [{ type: 'group', id: 'g1' }, { type: 'tab', id: 't3' }];
      expect(tm.orderedTabIds()).toEqual(['t2', 't1', 't3']);
    });

    it('skips group members not in ctx.tabs', () => {
      ctx.tabs.set('t1', createTab('t1'));
      ctx.groups.set('g1', createGroup('g1', { memberIds: ['t1', 'missing'] }));
      ctx.strip = [{ type: 'group', id: 'g1' }];
      expect(tm.orderedTabIds()).toEqual(['t1']);
    });
  });

  describe('pruneEmptyGroups', () => {
    it('removes groups with no members', () => {
      ctx.groups.set('g1', createGroup('g1', { memberIds: [] }));
      ctx.groups.set('g2', createGroup('g2', { memberIds: ['t1'] }));
      ctx.tabs.set('t1', createTab('t1'));
      ctx.strip = [
        { type: 'group', id: 'g1' },
        { type: 'group', id: 'g2' },
        { type: 'tab', id: 't1' },
      ];
      tm.pruneEmptyGroups();
      expect(ctx.groups.has('g1')).toBe(false);
      expect(ctx.groups.has('g2')).toBe(true);
      expect(ctx.strip.length).toBe(2);
      expect(ctx.strip[0]).toEqual({ type: 'group', id: 'g2' });
      expect(ctx.strip[1]).toEqual({ type: 'tab', id: 't1' });
    });

    it('removes strip entries for deleted groups', () => {
      ctx.groups.set('g1', createGroup('g1', { memberIds: [] }));
      ctx.strip = [{ type: 'group', id: 'g1' }];
      tm.pruneEmptyGroups();
      expect(ctx.strip.length).toBe(0);
    });

    it('removes strip entries for deleted tabs', () => {
      ctx.tabs.set('t1', createTab('t1'));
      ctx.strip = [{ type: 'tab', id: 't1' }, { type: 'tab', id: 'gone' }];
      tm.pruneEmptyGroups();
      expect(ctx.strip.length).toBe(1);
      expect(ctx.strip[0]).toEqual({ type: 'tab', id: 't1' });
    });

    it('filters group memberIds to existing tabs', () => {
      ctx.tabs.set('t1', createTab('t1'));
      ctx.groups.set('g1', createGroup('g1', { memberIds: ['t1', 'gone'] }));
      ctx.strip = [{ type: 'group', id: 'g1' }];
      tm.pruneEmptyGroups();
      const grp = ctx.groups.get('g1');
      expect(grp?.memberIds).toEqual(['t1']);
    });
  });

  describe('autoExpandActiveGroup', () => {
    it('does nothing when no active tab', () => {
      ctx.activeTabId = null;
      ctx.groups.set('g1', createGroup('g1', { collapsed: true }));
      tm.autoExpandActiveGroup();
      expect(ctx.groups.get('g1')?.collapsed).toBe(true);
    });

    it('does nothing when active tab has no group', () => {
      ctx.activeTabId = 't1';
      ctx.tabs.set('t1', createTab('t1'));
      ctx.groups.set('g1', createGroup('g1', { collapsed: true }));
      tm.autoExpandActiveGroup();
      expect(ctx.groups.get('g1')?.collapsed).toBe(true);
    });

    it('expands collapsed group holding active tab', () => {
      ctx.activeTabId = 't1';
      ctx.tabs.set('t1', createTab('t1', { groupID: 'g1' }));
      ctx.groups.set('g1', createGroup('g1', { collapsed: true }));
      tm.autoExpandActiveGroup();
      expect(ctx.groups.get('g1')?.collapsed).toBe(false);
    });

    it('does nothing when group is already expanded', () => {
      ctx.activeTabId = 't1';
      ctx.tabs.set('t1', createTab('t1', { groupID: 'g1' }));
      ctx.groups.set('g1', createGroup('g1', { collapsed: false }));
      tm.autoExpandActiveGroup();
      expect(ctx.groups.get('g1')?.collapsed).toBe(false);
    });
  });

  describe('createGroup', () => {
    it('creates a group with defaults', () => {
      const grp = tm.createGroup();
      expect(grp.id).toMatch(/^group_/);
      expect(grp.title).toBe('Нова група');
      expect(grp.color).toBe('#0e639c');
      expect(grp.collapsed).toBe(false);
      expect(grp.memberIds).toEqual([]);
      expect(ctx.groups.has(grp.id)).toBe(true);
    });

    it('uses provided title and color', () => {
      const grp = tm.createGroup('My Group', '#ff0000');
      expect(grp.title).toBe('My Group');
      expect(grp.color).toBe('#ff0000');
    });

    it('cycles through GROUP_COLORS', () => {
      const g1 = tm.createGroup();
      const g2 = tm.createGroup();
      expect(g1.color).toBe('#0e639c');
      expect(g2.color).toBe('#10b981');
    });
  });

  describe('moveTabToGroup', () => {
    it('moves standalone tab into group', () => {
      const t1 = createTab('t1');
      ctx.tabs.set('t1', t1);
      ctx.strip = [{ type: 'tab', id: 't1' }];
      const grp = tm.createGroup('G', '#000');
      ctx.groups.set('g1', grp);

      tm.moveTabToGroup('t1', 'g1');

      expect(t1.groupID).toBe('g1');
      expect(grp.memberIds).toEqual(['t1']);
      expect(ctx.strip.some(e => e.type === 'tab' && e.id === 't1')).toBe(false);
      expect(ctx.strip.some(e => e.type === 'group' && e.id === 'g1')).toBe(true);
    });

    it('moves tab from one group to another', () => {
      const t1 = createTab('t1', { groupID: 'g1' });
      ctx.tabs.set('t1', t1);
      const g1 = createGroup('g1', { memberIds: ['t1'] });
      const g2 = createGroup('g2', { memberIds: [] });
      ctx.groups.set('g1', g1);
      ctx.groups.set('g2', g2);
      ctx.strip = [{ type: 'group', id: 'g1' }, { type: 'group', id: 'g2' }];

      tm.moveTabToGroup('t1', 'g2');

      expect(t1.groupID).toBe('g2');
      expect(g1.memberIds).toEqual([]);
      expect(g2.memberIds).toEqual(['t1']);
      // g1 should be pruned from strip since empty
      expect(ctx.strip.some(e => e.type === 'group' && e.id === 'g1')).toBe(false);
    });

    it('inserts before a specific member', () => {
      const t1 = createTab('t1');
      const t2 = createTab('t2');
      const t3 = createTab('t3');
      ctx.tabs.set('t1', t1);
      ctx.tabs.set('t2', t2);
      ctx.tabs.set('t3', t3);
      const grp = createGroup('g1', { memberIds: ['t2', 't3'] });
      ctx.groups.set('g1', grp);
      ctx.strip = [{ type: 'group', id: 'g1' }];

      tm.moveTabToGroup('t1', 'g1', 't2');

      expect(grp.memberIds).toEqual(['t1', 't2', 't3']);
    });

    it('does nothing when tab or group missing', () => {
      tm.moveTabToGroup('missing', 'g1');
      expect(ctx.groups.size).toBe(0);
    });
  });

  describe('ungroupTab', () => {
    it('takes a tab out of its group into the strip', () => {
      const t1 = createTab('t1', { groupID: 'g1' });
      ctx.tabs.set('t1', t1);
      const grp = createGroup('g1', { memberIds: ['t1'] });
      ctx.groups.set('g1', grp);
      ctx.strip = [{ type: 'group', id: 'g1' }];

      tm.ungroupTab('t1');

      expect(t1.groupID).toBe(null);
      expect(grp.memberIds).toEqual([]);
      expect(ctx.strip.some(e => e.type === 'tab' && e.id === 't1')).toBe(true);
      // empty group pruned
      expect(ctx.groups.has('g1')).toBe(false);
    });

    it('does nothing for tab not in a group', () => {
      const t1 = createTab('t1');
      ctx.tabs.set('t1', t1);
      ctx.strip = [{ type: 'tab', id: 't1' }];
      tm.ungroupTab('t1');
      expect(t1.groupID).toBe(null);
    });

    it('inserts before a given strip entry', () => {
      const t1 = createTab('t1', { groupID: 'g1' });
      const t2 = createTab('t2');
      ctx.tabs.set('t1', t1);
      ctx.tabs.set('t2', t2);
      const grp = createGroup('g1', { memberIds: ['t1'] });
      ctx.groups.set('g1', grp);
      ctx.strip = [{ type: 'group', id: 'g1' }, { type: 'tab', id: 't2' }];

      tm.ungroupTab('t1', { type: 'tab', id: 't2' });

      const tabEntry = ctx.strip.find(e => e.type === 'tab' && e.id === 't1');
      expect(tabEntry).toEqual({ type: 'tab', id: 't1' });
      expect(ctx.strip.indexOf(tabEntry!)).toBeLessThan(ctx.strip.findIndex(e => e.type === 'tab' && e.id === 't2'));
    });
  });

  describe('moveGroup', () => {
    it('reorders a group within the strip', () => {
      const t1 = createTab('t1');
      ctx.tabs.set('t1', t1);
      const g1 = createGroup('g1');
      const g2 = createGroup('g2');
      ctx.groups.set('g1', g1);
      ctx.groups.set('g2', g2);
      ctx.strip = [{ type: 'group', id: 'g1' }, { type: 'tab', id: 't1' }, { type: 'group', id: 'g2' }];

      tm.moveGroup('g2', { type: 'group', id: 'g1' });

      expect(ctx.strip[0]).toEqual({ type: 'group', id: 'g2' });
      expect(ctx.strip[1]).toEqual({ type: 'group', id: 'g1' });
      expect(ctx.strip[2]).toEqual({ type: 'tab', id: 't1' });
    });

    it('does nothing when group not in strip', () => {
      const g1 = createGroup('g1');
      ctx.groups.set('g1', g1);
      ctx.strip = [];
      tm.moveGroup('g1', null);
      expect(ctx.strip.length).toBe(0);
    });

    it('does nothing when beforeEntry is the same entry', () => {
      const g1 = createGroup('g1');
      ctx.groups.set('g1', g1);
      ctx.strip = [{ type: 'group', id: 'g1' }];
      tm.moveGroup('g1', { type: 'group', id: 'g1' });
      expect(ctx.strip.length).toBe(1);
    });
  });

  describe('addOrUpdateTab', () => {
    it('adds a new tab', () => {
      const stats = {
        tab_id: 'new-tab',
        file_path: '/var/log/app.log',
        file_name: 'app.log',
        total_lines: 500,
        matched_lines: 10,
        file_size: 4096,
      };
      tm.addOrUpdateTab(stats);

      const tab = ctx.tabs.get('new-tab');
      expect(tab).toBeDefined();
      expect(tab?.filePath).toBe('/var/log/app.log');
      expect(tab?.fileName).toBe('app.log');
      expect(tab?.groupID).toBe(null);
      expect(tab?.totalLines).toBe(500);
      expect(tab?.autoScroll).toBe(true);
      expect(ctx.strip.some(e => e.type === 'tab' && e.id === 'new-tab')).toBe(true);
      expect(ctx.activeTabId).toBe('new-tab');
    });

    it('updates existing tab fields instead of duplicating', () => {
      const existing = createTab('t1');
      existing.totalLines = 100;
      ctx.tabs.set('t1', existing);
      ctx.strip = [{ type: 'tab', id: 't1' }];
      ctx.activeTabId = 'other';

      tm.addOrUpdateTab({ tab_id: 't1', file_path: '/var/log/app.log', file_name: 'app.log', total_lines: 999, matched_lines: 5, file_size: 8192 });

      const tab = ctx.tabs.get('t1');
      expect(tab?.totalLines).toBe(999);
      expect(tab?.matchedLines).toBe(5);
      expect(ctx.strip.filter(e => e.type === 'tab' && e.id === 't1').length).toBe(1);
    });

    it('does nothing for null stats', () => {
      tm.addOrUpdateTab(null as any);
      expect(ctx.tabs.size).toBe(0);
    });

    it('detects SSH tabs from is_ssh flag', () => {
      tm.addOrUpdateTab({ tab_id: 'ssh1', file_path: '/var/log/remote.log', file_name: 'remote.log', is_ssh: true, total_lines: 10, matched_lines: 0, file_size: 100, ssh: { host: 'example.com' } as any });
      const tab = ctx.tabs.get('ssh1');
      expect(tab?.isSSH).toBe(true);
      expect(tab?.ssh?.host).toBe('example.com');
    });

    it('detects SSH tabs from ssh:// path prefix', () => {
      tm.addOrUpdateTab({ tab_id: 'ssh2', file_path: 'ssh://host/var/log/remote.log', file_name: 'remote.log', total_lines: 10, matched_lines: 0, file_size: 100 });
      const tab = ctx.tabs.get('ssh2');
      expect(tab?.isSSH).toBe(true);
    });
  });

  describe('switchTab', () => {
    it('sets active tab id', () => {
      const t1 = createTab('t1');
      const t2 = createTab('t2');
      ctx.tabs.set('t1', t1);
      ctx.tabs.set('t2', t2);
      ctx.activeTabId = 't1';

      tm.switchTab('t2');

      expect(ctx.activeTabId).toBe('t2');
    });

    it('calls api.SetActiveTab when api available', () => {
      const setActiveTab = vi.fn();
      ctx.getApi = () => ({ SetActiveTab: setActiveTab });
      const t1 = createTab('t1');
      ctx.tabs.set('t1', t1);

      tm.switchTab('t1');

      expect(setActiveTab).toHaveBeenCalledWith('t1');
    });
  });

  describe('deleteGroup', () => {
    it('ungroups all members and removes the group', () => {
      const t1 = createTab('t1');
      const t2 = createTab('t2');
      ctx.tabs.set('t1', t1);
      ctx.tabs.set('t2', t2);
      const grp = createGroup('g1', { memberIds: ['t1', 't2'] });
      ctx.groups.set('g1', grp);
      ctx.strip = [{ type: 'group', id: 'g1' }];

      tm.deleteGroup('g1');

      expect(ctx.groups.has('g1')).toBe(false);
      expect(t1.groupID).toBe(null);
      expect(t2.groupID).toBe(null);
      expect(ctx.strip.some(e => e.type === 'group' && e.id === 'g1')).toBe(false);
      // tabs remain in strip
      expect(ctx.strip.some(e => e.type === 'tab' && e.id === 't1')).toBe(true);
      expect(ctx.strip.some(e => e.type === 'tab' && e.id === 't2')).toBe(true);
    });

    it('does nothing for missing group', () => {
      tm.deleteGroup('missing');
      expect(ctx.strip.length).toBe(0);
    });
  });

  describe('closeAllTabs', () => {
    it('clears all tabs, groups, strip, and active tab', async () => {
      const t1 = createTab('t1');
      ctx.tabs.set('t1', t1);
      ctx.groups.set('g1', createGroup('g1', { memberIds: ['t1'] }));
      ctx.strip = [{ type: 'group', id: 'g1' }];
      ctx.activeTabId = 't1';

      await tm.closeAllTabs();

      expect(ctx.tabs.size).toBe(0);
      expect(ctx.groups.size).toBe(0);
      expect(ctx.strip.length).toBe(0);
      expect(ctx.activeTabId).toBe(null);
    });
  });
});
