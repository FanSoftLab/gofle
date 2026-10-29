# Глосарій термінології (Двомовний UK/EN)

**Єдине джерело істини для всякої документації GoFLE, UI рядків та help контенту.**

| Концепція | UK термін | EN термін | Визначення | Контекст | Заборонені альтернативи |
|-----------|-----------|-----------|------------|----------|------------------------|
| Live tailing файлу | **слідкувати** | **follow** | Автоматичне відображення нових рядків, доданих до файлу | UI кнопки, help, процедури, settings | хвіст, tail, автопрокрутка, фолов, follow mode |
| Byte-offset індекс рядків | **індекс зміщень рядків** | **line-offset index** | In-memory масив байтових позицій для кожного рядка; дозволяє випадковий доступ без завантаження всього файлу | Архітектура, concept docs, продуктивність | byte index, offset index, індекс байтів, індекс зміщень |
| Панель результатів пошуку | **панель результатів** | **results pane** | Нижня або бічна панель, що показує рядки, що відповідають пошуку | UI, help, клавіатурні скорочення | results panel, bottom pane, панель пошуку, results view |
| Панель розширених фільтрів | **панель розширених фільтрів** | **advanced filters bar** | Рядок фільтр-чіпів (Nearby, Levels, Time, Exclude, Range, Top, Presets) над результатами | UI, help, settings | chip row, filter chips, фільтри, advanced bar |
| Пресет пошуку | **пресет пошуку** | **search preset** | Пменована знімка налаштувань розширених фільтрів, збережена в localStorage | UI, help, settings, CLI | saved filters, filter preset, збережені фільтри |
| Група вкладок | **група вкладок** | **tab group** | Пменована, кольорова колекція вкладок для візуальної організації | UI, help, context menu | group, tab collection, вкладка group |
| Директорія конфігурації | **директорія конфігурації** | **config directory** | OS-специфічна папка, що зберігає settings.json, recent.json, saved_tabs.json, search_presets.json | Reference, paths, troubleshooting | settings folder, GoFLE folder, config folder, папка налаштувань |
| Термінальний переглядач | **термінальний переглядач** | **terminal viewer** | Інтерактивний Bubbletea-based CLI viewer для логів (local/remote) | CLI reference, help, README | Bubbletea viewer, CLI viewer, інтерактивний переглядач |
| Перевірка ключа хоста | **перевірка ключа хоста** | **host key verification** | Перевірка ідентичності SSH сервера проти known_hosts; TOFU при першому connect | Security, SSH docs, troubleshooting | key check, known_hosts check, перевірка known_hosts |
| Довіра сертифіката | **довіра сертифіката** | **certificate trust** | Пінгування FTPS серверного сертифіката в ftp_certs store; явна довіра при першому connect | Security, FTP docs, troubleshooting | cert pinning, ftp_certs, пінг сертифіката |
| Віртуалізоване рендеринг | **віртуалізоване відображення** | **virtualized rendering** | Тільки видимі рядки логу існують в DOM; дозволяє плавний скрол на мільйонах рядків | Архітектура, продуктивність, concept | virtual scrolling, lazy rendering, віртуальний скрол |
| Діапазон рядків | **діапазон рядків** | **line range** | Область пошуку: увесь файл, від курсора, до курсора, останні N рядків, конкретні номери рядків | Search advanced filters, CLI | range, search range, діапазон пошуку |
| Nearby фільтр | **фільтр «Поруч»** | **Nearby filter** | Залишає тільки primary-match рядки, біля яких у вікні ±N рядків є інший патерн | Search advanced, help, examples | nearby, proximity filter, фільтр поруч |
| Level фільтр | **фільтр рівнів** | **Level filter** | Евристична класифікація: ERROR, WARN, INFO, DEBUG, OTHER | Search advanced, help, settings | levels, log levels, рівні логування |
| Time фільтр | **фільтр часу** | **Time filter** | Фільтр за діапазоном дати/часу з опціональною вимогою timestamp | Search advanced, help, settings | time, date filter, фільтр дати |
| Exclude фільтр | **фільтр виключення** | **Exclude filter** | Патерни (один на рядок), що видаляють відповідні рядки з результатів | Search advanced, help, settings | exclude, exclusion patterns, виключення |
| Top unique | **Топ унікальних** | **Top unique** | Агрегація поточних результатів: нормалізовані рядки згруповані за частотою | Search advanced, help, settings | top, frequent matches, топ |
| Контекстні рядки | **контекст: рядки до/після** | **Context lines before/after** | Кількість рядків, показаних навколо кожного match в results (зараз in-row only) | Search settings, help | context, surrounding lines, контекст |
| Follow режим | **режим слідкування** | **Follow mode** | Стан, коли view залишається в кінці файлу, коли приходять нові рядки | UI, settings, help | tail mode, auto-scroll, режим хвоста |
| Search combine logic | **логіка другого поля (AND/OR)** | **Secondary field logic (AND/OR)** | Чи AND поле вимагає обидва патерни (AND) або будь-який (OR) в тому ж рядку | Search UI, help, settings | combine logic, AND/OR logic, логіка пошуку |
| Stall timeout | **таймаут тиші завантаження** | **Download silence timeout** | Прервати remote download, якщо N секунд не отримано байтів (повільні але активні передачі не чіпаються) | Remote settings, SSH/FTP, troubleshooting | stall timeout, silence timeout, таймаут стриму |
| Auto-accept host key | **автоприйняття ключа хоста** | **Auto-accept host key** | Зберегти невідомий SSH host key / FTPS cert без запиту (позначено як небезпечне) | Remote settings, security, troubleshooting | auto-accept, trust on first use, автоматичне прийняття |
| Recent files list | **список «Останні»** | **Recent files list** | Об'єднана історія local/SSH/FTP файлів; дедуплена за canonical URL | UI, toolbar, CLI, settings | recents, recent files, нещодавні файли |
| Saved session | **збережена сесія** | **Saved session** | Знімок відкритих вкладок (local + remote), відновлений при наступному запуску, якщо увімкнено | Settings, session, help | session restore, tab restore, сесія |
| Search history | **історія введення** | **Search input history** | Попередні запити, збережені на поле пошуку; налаштовувана глибина | Search UI, settings, help | search history, query history, історія пошуку |
| Word wrap | **перенос довгих рядків** | **Word wrap** | Довгі рядки повністю переносяться в межах вікна (без горизонтального скролу) | Viewer settings, help, performance | wrap, line wrap, перенос рядків |
| Line density | **щільність рядків** | **Line density** | Вертикальний інтервал між рядками логу: Compact / Normal / Comfortable | Viewer settings, help | density, row height, висота рядка |
| View mode | **режим перегляду** | **View mode** | Дефолтний layout панелей: Split (log + results), Results only, Log only | Viewer settings, help, toolbar | pane mode, layout, режим панелей |
| Split view | **розділений перегляд** | **Split view** | Log pane і results pane показані одночасно | Viewer settings, toolbar, help | split, split pane, розділена панель |
| Results only view | **лише результати** | **Results only** | Видна тільки панель результатів пошуку | Viewer settings, toolbar, help | filter view, results view, режим результатів |
| Log only view | **лише лог** | **Log only** | Видна тільки повна log pane (results приховані) | Viewer settings, toolbar, help | normal view, log view, режим логу |
| Go to line | **перейти до рядка** | **Go to line** | Перехід до конкретного номера рядка в повній log pane (Ctrl+G) | UI, keyboard shortcuts, help | goto, line navigation, перехід до рядка |
| Export results | **зберегти результати пошуку** | **Save search results** | Записати matched рядки (з номерами рядків) у .log файл (Ctrl+S) | UI, context menu, help, shortcuts | export matches, save matches, експорт збігів |
| Export full log | **зберегти лог як...** | **Save log as...** | Записати повний лог файл на диск з context menu | UI, context menu, help | save full log, export log, експорт логу |
| SSH connection form | **форма SSH-підключення** | **SSH connection form** | Модалка з host, port, user, auth type, key/password, remote path, browse button | UI, help, troubleshooting | SSH modal, SSH dialog, SSH форма |
| FTP connection form | **форма FTP-підключення** | **FTP connection form** | Модалка з host, port, user, password, TLS mode, remote path, browse button | UI, help, troubleshooting | FTP modal, FTP dialog, FTP форма |
| Remote directory browser | **браузер віддаленого каталогу** | **Remote directory browser** | Інтерактивний file picker для SSH/FTP серверів (browse, filter, navigate) | UI, SSH/FTP forms, help | remote browser, directory picker, віддалений браузер |
| Primary query | **основний запит** | **Primary query** | Головний пошуковий текст/regex, введений в ліве поле пошуку | Search UI, help, examples | primary, main query, головний запит |
| Secondary AND field | **AND-поле** | **AND field** | Друге поле пошуку, яке вимагає match в тому ж рядку (логічне AND з primary) | Search UI, help, examples | AND, secondary query, друге поле |
| Invert match | **інвертувати збіг** | **Invert match** | Показати рядки, що НЕ відповідають запиту (Alt+I) | Search UI, help, shortcuts | invert, negative match, інверсія |
| Match case | **враховувати регістр** | **Match case** | Case-sensitive пошук (Alt+C) | Search UI, help, shortcuts | case sensitive, case, регістр |
| Regex mode | **регулярний вираз** | **Regular expression** | Інтерпретувати запит як RE2/Go regex (Alt+R) | Search UI, help, shortcuts | regex, regexp, regex mode |

---

## Правила використання

1. **Фіксуйте терміни**: Якщо термін є в цьому глосарії, використовуйте його узгоджено у всей документації, UI рядках (`i18n.ts`), in-app help (`help_content.ts`) та кодових коментарях.

2. **Немає синонімів в одному контексті**: Не змінюйте "follow" / "слідкувати" на "tail" / "хвіст" в одному документі або UI екрані.

3. **Двомовне узгодження**: UK та EN терміни парні. При додаванні нового концепту, додавайте обидві мовні терміни одночасно.

4. **UI рядки виводяться з глосарію**: Ключі та значення `i18n.ts` мають використовувати терміни глосарію. Якщо UI рядок відрізняється — оновіть глосарій або рядок.

5. **Help контент виводиться з глосарію**: Текст `help_content.ts` body має використовувати терміни глосарію.

6. **Заборонена колонка**: Терміни, перелічені тут, не мають з'являтися в новій документації. Існуючі входження мають бути мігровані при оновленні документації.

7. **Процес рев'ю**: Перед мерджем змі documentación, перевірте відповідність глосарію:
   - `grep -r "заборонений_термін" docs/` не має повертати результатів
   - UI рядки рев'ються проти глосарію в PR