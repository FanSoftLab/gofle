# Terminology Glossary (Bilingual UK/EN)

**Single source of truth for all GoFLE documentation, UI strings, and help content.**

| Concept | UK Term | EN Term | Definition | Context | Disallowed Alternatives |
|---------|---------|---------|------------|---------|------------------------|
| Live tailing of file | **слідкувати** | **follow** | Automatic display of new lines appended to a file | UI buttons, help, procedures, settings | хвіст, tail, автопрокрутка, фолов, follow mode |
| Byte-offset line index | **індекс зміщень рядків** | **line-offset index** | In-memory array of byte positions for each line; enables random access without loading full file | Architecture, concept docs, performance | byte index, offset index, індекс байтів, індекс зміщень |
| Search results panel | **панель результатів** | **results pane** | Bottom or side panel showing matching lines from search | UI, help, keyboard shortcuts | results panel, bottom pane, панель пошуку, results view |
| Advanced filters bar | **панель розширених фільтрів** | **advanced filters bar** | Row of filter chips (Nearby, Levels, Time, Exclude, Range, Top, Presets) above results | UI, help, settings | chip row, filter chips, фільтри, advanced bar |
| Search preset | **пресет пошуку** | **search preset** | Named snapshot of advanced filter settings saved in localStorage | UI, help, settings, CLI | saved filters, filter preset, збережені фільтри |
| Tab group | **група вкладок** | **tab group** | Named, colored collection of tabs for visual organization | UI, help, context menu | group, tab collection, вкладка group |
| Configuration directory | **директорія конфігурації** | **config directory** | OS-specific folder storing settings.json, recent.json, saved_tabs.json, search_presets.json | Reference, paths, troubleshooting | settings folder, GoFLE folder, config folder, папка налаштувань |
| Terminal viewer | **термінальний переглядач** | **terminal viewer** | Interactive Bubbletea-based CLI viewer for logs (local/remote) | CLI reference, help, README | Bubbletea viewer, CLI viewer, інтерактивний переглядач |
| Host key verification | **перевірка ключа хоста** | **host key verification** | SSH server identity check against known_hosts; TOFU on first connect | Security, SSH docs, troubleshooting | key check, known_hosts check, перевірка known_hosts |
| Certificate trust | **довіра сертифіката** | **certificate trust** | FTPS server certificate pinning in ftp_certs store; explicit trust on first connect | Security, FTP docs, troubleshooting | cert pinning, ftp_certs, пінг сертифіката |
| Virtualized rendering | **віртуалізоване відображення** | **virtualized rendering** | Only visible log rows exist in DOM; enables smooth scrolling on millions of lines | Architecture, performance, concept | virtual scrolling, lazy rendering, віртуальний скрол |
| Line range | **діапазон рядків** | **line range** | Search scope: entire file, from cursor, to cursor, last N lines, specific line numbers | Search advanced filters, CLI | range, search range, діапазон пошуку |
| Nearby filter | **фільтр «Поруч»** | **Nearby filter** | Keeps primary matches that have another pattern within ±N lines (before/after/either) | Search advanced, help, examples | nearby, proximity filter, фільтр поруч |
| Level filter | **фільтр рівнів** | **Level filter** | Heuristic classification: ERROR, WARN, INFO, DEBUG, OTHER | Search advanced, help, settings | levels, log levels, рівні логування |
| Time filter | **фільтр часу** | **Time filter** | Date/time range filter with optional timestamp requirement | Search advanced, help, settings | time, date filter, фільтр дати |
| Exclude filter | **фільтр виключення** | **Exclude filter** | Patterns (one per line) that remove matching lines from results | Search advanced, help, settings | exclude, exclusion patterns, виключення |
| Top unique | **Топ унікальних** | **Top unique** | Aggregation of current results: normalized lines grouped by frequency | Search advanced, help, settings | top, frequent matches, топ |
| Context lines | **контекст: рядки до/після** | **Context lines before/after** | Number of lines shown around each match in results (currently in-row only) | Search settings, help | context, surrounding lines, контекст |
| Follow mode | **режим слідкування** | **Follow mode** | State where view stays at end of file as new lines arrive | UI, settings, help | tail mode, auto-scroll, режим хвоста |
| Search combine logic | **логіка другого поля (AND/OR)** | **Secondary field logic (AND/OR)** | Whether AND field requires both patterns (AND) or either (OR) on same line | Search UI, help, settings | combine logic, AND/OR logic, логіка пошуку |
| Stall timeout | **таймаут тиші завантаження** | **Download silence timeout** | Abort remote download if no bytes received for N seconds (slow but active transfers continue) | Remote settings, SSH/FTP, troubleshooting | stall timeout, silence timeout, таймаут стриму |
| Auto-accept host key | **автоприйняття ключа хоста** | **Auto-accept host key** | Store unknown SSH host key or FTPS certificate without prompting (marked dangerous) | Remote settings, security, troubleshooting | auto-accept, trust on first use, автоматичне прийняття |
| Recent files list | **список «Останні»** | **Recent files list** | Unified history of local/SSH/FTP files; deduped by canonical URL | UI, toolbar, CLI, settings | recents, recent files, нещодавні файли |
| Saved session | **збережена сесія** | **Saved session** | Snapshot of open tabs (local + remote) restored on next launch when enabled | Settings, session, help | session restore, tab restore, сесія |
| Search history | **історія введення** | **Search input history** | Previous queries saved per search field; configurable depth | Search UI, settings, help | search history, query history, історія пошуку |
| Word wrap | **перенос довгих рядків** | **Word wrap** | Long lines wrap fully inside window (no horizontal scroll) | Viewer settings, help, performance | wrap, line wrap, перенос рядків |
| Line density | **щільність рядків** | **Line density** | Vertical spacing between log rows: Compact / Normal / Comfortable | Viewer settings, help | density, row height, висота рядка |
| View mode | **режим перегляду** | **View mode** | Default pane layout: Split (log + results), Results only, Log only | Viewer settings, help, toolbar | pane mode, layout, режим панелей |
| Split view | **розділений перегляд** | **Split view** | Log pane and results pane shown simultaneously | Viewer settings, toolbar, help | split, split pane, розділена панель |
| Results only view | **лише результати** | **Results only** | Only the search results pane is visible | Viewer settings, toolbar, help | filter view, results view, режим результатів |
| Log only view | **лише лог** | **Log only** | Only the full log pane is visible (results hidden) | Viewer settings, toolbar, help | normal view, log view, режим логу |
| Go to line | **перейти до рядка** | **Go to line** | Jump to specific line number in full log pane (Ctrl+G) | UI, keyboard shortcuts, help | goto, line navigation, перехід до рядка |
| Export results | **зберегти результати пошуку** | **Save search results** | Write matched lines (with line numbers) to .log file (Ctrl+S) | UI, context menu, help, shortcuts | export matches, save matches, експорт збігів |
| Export full log | **зберегти лог як...** | **Save log as...** | Write entire log file to disk from context menu | UI, context menu, help | save full log, export log, експорт логу |
| SSH connection form | **форма SSH-підключення** | **SSH connection form** | Modal with host, port, user, auth type, key/password, remote path, browse button | UI, help, troubleshooting | SSH modal, SSH dialog, SSH форма |
| FTP connection form | **форма FTP-підключення** | **FTP connection form** | Modal with host, port, user, password, TLS mode, remote path, browse button | UI, help, troubleshooting | FTP modal, FTP dialog, FTP форма |
| Remote directory browser | **браузер віддаленого каталогу** | **Remote directory browser** | Interactive file picker for SSH/FTP servers (browse, filter, navigate) | UI, SSH/FTP forms, help | remote browser, directory picker, віддалений браузер |
| Primary query | **основний запит** | **Primary query** | Main search text/regex entered in left search field | Search UI, help, examples | primary, main query, головний запит |
| Secondary AND field | **AND-поле** | **AND field** | Second search field requiring match on same line (logical AND with primary) | Search UI, help, examples | AND, secondary query, друге поле |
| Invert match | **інвертувати збіг** | **Invert match** | Show lines that do NOT match the query (Alt+I) | Search UI, help, shortcuts | invert, negative match, інверсія |
| Match case | **враховувати регістр** | **Match case** | Case-sensitive search (Alt+C) | Search UI, help, shortcuts | case sensitive, case, регістр |
| Regex mode | **регулярний вираз** | **Regular expression** | Interpret query as RE2/Go regex (Alt+R) | Search UI, help, shortcuts | regex, regexp, regex mode |

---

## Usage Rules

1. **Lock terms**: Once a term is in this glossary, use it consistently across all documentation, UI strings (`i18n.ts`), in-app help (`help_content.ts`), and code comments.

2. **No synonyms in same context**: Do not vary "follow" / "слідкувати" with "tail" / "хвіст" in the same document or UI screen.

3. **Bilingual alignment**: UK and EN terms are paired. When adding a new concept, add both language terms simultaneously.

4. **UI strings derive from glossary**: The `i18n.ts` keys and values should use glossary terms. If a UI string differs, update the glossary or the string.

5. **Help content derives from glossary**: `help_content.ts` body text must use glossary terms.

6. **Disallowed column**: Terms listed here must not appear in new documentation. Existing occurrences should be migrated during documentation updates.

7. **Review process**: Before merging documentation changes, verify glossary compliance:
   - `grep -r "disallowed_term" docs/` should return no results
   - UI strings reviewed against glossary in PR