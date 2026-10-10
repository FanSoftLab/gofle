
/** Structured in-app help (bilingual). Selectable plain text, simple lists + examples. */
export type HelpLang = 'uk' | 'en';

export type HelpItem = {
  title: { uk: string; en: string };
  body: { uk: string[]; en: string[] };
  example?: { uk: string[]; en: string[] };
};

export type HelpSection = {
  id: string;
  title: { uk: string; en: string };
  items: HelpItem[];
};

export const HELP_SECTIONS: HelpSection[] = [
  {
    id: 'overview',
    title: { uk: 'Огляд', en: 'Overview' },
    items: [
      {
        title: { uk: 'Призначення GoFLE', en: 'What GoFLE is for' },
        body: {
          uk: [
            'GoFLE (Go Fast Log Explorer) — десктопний переглядач дуже великих лог-файлів (сотні МБ і більше).',
            'Файл не завантажується цілком у RAM: будується індекс зміщень рядків, а текст читається лише для видимої області екрана.',
            'Підходить для розбору production-логів, nginx/apache, application logs, journal-експортів тощо.',
            'Підтримує локальні шляхи, SSH і FTP/FTPS, кілька вкладок, live-follow і пошук із додатковими фільтрами.',
          ],
          en: [
            'GoFLE (Go Fast Log Explorer) is a desktop viewer for very large log files (hundreds of MB and more).',
            'The file is not loaded fully into RAM: a line-offset index is built, and text is read only for the visible area.',
            'Useful for production logs, nginx/apache, application logs, journal exports, and similar.',
            'Supports local paths, SSH and FTP/FTPS, multiple tabs, live follow, and search with extra filters.',
          ],
        },
      },
      {
        title: { uk: 'Інтерфейс: дві панелі', en: 'UI: two panes' },
        body: {
          uk: [
            'Верхня (або бічна) панель — повний лог з номерами рядків і підсвіткою.',
            'Нижня панель — результати пошуку. Над нею — рядок розширених фільтрів.',
            'Роздільник між панелями можна перетягувати мишею.',
            'Кнопка розкладки (біля шестерні) змінює орієнтацію (горизонтально/вертикально) і сторону results.',
          ],
          en: [
            'Top (or side) pane — full log with line numbers and highlighting.',
            'Bottom pane — search results. Above it — the advanced filters bar.',
            'Drag the splitter between panes with the mouse.',
            'Layout button (near the gear) changes orientation (horizontal/vertical) and which side holds results.',
          ],
        },
      },
    ],
  },
  {
    id: 'toolbar',
    title: { uk: 'Панель інструментів', en: 'Toolbar' },
    items: [
      {
        title: { uk: 'Відкрити файл(и)', en: 'Open file(s)' },
        body: {
          uk: [
            'Іконка теки — діалог вибору одного або кількох локальних файлів (Ctrl+O).',
            'Кожен файл відкривається окремою вкладкою. Можна також перетягнути файли в вікно.',
          ],
          en: [
            'Folder icon — dialog to pick one or more local files (Ctrl+O).',
            'Each file opens in its own tab. You can also drag and drop files onto the window.',
          ],
        },
      },
      {
        title: { uk: 'Останні', en: 'Recent' },
        body: {
          uk: [
            'Іконка годинника — список нещодавно відкритих файлів (Ctrl+R).',
            'Вкладки списку: Локальні, SSH, FTP. Подвійний клік або кнопка відкриває знову.',
            '«Очистити» прибирає історію з диску налаштувань додатку.',
          ],
          en: [
            'Clock icon — list of recently opened files (Ctrl+R).',
            'List tabs: Local, SSH, FTP. Double-click or the open action reopens the path.',
            'Clear removes history from the app settings store.',
          ],
        },
      },
      {
        title: { uk: 'Віддалений лог', en: 'Remote log' },
        body: {
          uk: [
            'Іконка хмари — відкрити лог через SSH або FTP/FTPS.',
            'Після з’єднання файл стрімиться в локальний кеш і індексується як звичайний лог; працює follow.',
            'Пароль у GUI вводиться в полі форми (не передавайте його в аргументах CLI без потреби).',
          ],
          en: [
            'Cloud icon — open a log over SSH or FTP/FTPS.',
            'After connect, the file is streamed into a local cache and indexed like a normal log; follow works.',
            'In the GUI, enter the password in the form field (avoid putting it on the CLI unless necessary).',
          ],
        },
      },
      {
        title: { uk: 'Перейти до рядка', en: 'Go to line' },
        body: {
          uk: [
            'Ctrl+G — діалог з номером рядка (1-based). Перехід у панелі повного логу.',
          ],
          en: [
            'Ctrl+G — dialog for a 1-based line number. Jumps in the full-log pane.',
          ],
        },
      },
      {
        title: { uk: 'Follow', en: 'Follow' },
        body: {
          uk: [
            'Кнопка зі стрілками вниз або клавіша F — «хвіст» файлу: нові рядки автоматично з’являються внизу.',
            'Якщо прокрутити лог вгору, follow тимчасово зупиняється, щоб не втратити контекст.',
          ],
          en: [
            'Down-arrows button or F — file tail: new lines appear at the bottom automatically.',
            'Scrolling the log up pauses follow so you do not lose context.',
          ],
        },
      },
      {
        title: { uk: 'Пошуковий рядок', en: 'Search box' },
        body: {
          uk: [
            'Ліве поле — основний запит (текст або regex).',
            'Праве поле після мітки AND — друга умова на тому ж рядку (обидва мають збігтися).',
            'Кнопки Cc — урахування регістру; .* — режим regex; ! — інверсія (рядки без збігу).',
            '× — очистити поля пошуку. Клавіша / — швидко сфокусувати пошук.',
          ],
          en: [
            'Left field — primary query (text or regex).',
            'Right field after AND — second condition on the same line (both must match).',
            'Buttons: Cc — case sensitive; .* — regex mode; ! — invert (lines that do not match).',
            '× clears search fields. Press / to focus search quickly.',
          ],
        },
        example: {
          uk: [
            'Основний: timeout',
            'AND: upstream',
            'Cc вимкнено → знайде Timeout і TIMEOUT',
          ],
          en: [
            'Primary: timeout',
            'AND: upstream',
            'Cc off → matches Timeout and TIMEOUT',
          ],
        },
      },
      {
        title: { uk: 'Навігація по збігах', en: 'Match navigation' },
        body: {
          uk: [
            'Кнопки ⇈ / ↑ / ↓ / ⇊ — перший, попередній, наступний, останній результат у панелі results.',
            'Лічильник показує кількість збігів відносно розміру файлу.',
            'Іконка збереження — експорт поточних results у файл.',
          ],
          en: [
            'Buttons ⇈ / ↑ / ↓ / ⇊ — first, previous, next, last hit in the results pane.',
            'The counter shows match count vs file size.',
            'Save icon — export current results to a file.',
          ],
        },
      },
      {
        title: { uk: 'Довідка та налаштування', en: 'Help and settings' },
        body: {
          uk: [
            'Кнопка ? (F1) — це вікно довідки.',
            'Шестерня — налаштування вигляду, перегляду, пошуку, сесії та remote.',
          ],
          en: [
            '? button (F1) — this help window.',
            'Gear — appearance, viewer, search, session, and remote settings.',
          ],
        },
      },
    ],
  },
  {
    id: 'tabs',
    title: { uk: 'Вкладки', en: 'Tabs' },
    items: [
      {
        title: { uk: 'Робота з вкладками', en: 'Working with tabs' },
        body: {
          uk: [
            'Кожен відкритий файл — окрема вкладка у верхньому рядку.',
            'Клік по вкладці активує її. Скрол у логу й у results зберігається для кожної вкладки окремо.',
            'Закриття: середня кнопка миші, контекстне меню або Ctrl+W.',
            'Ctrl+Tab / Ctrl+Shift+Tab — наступна / попередня вкладка.',
          ],
          en: [
            'Each open file is a tab in the top bar.',
            'Click a tab to activate it. Scroll in log and results is stored per tab.',
            'Close: middle mouse button, context menu, or Ctrl+W.',
            'Ctrl+Tab / Ctrl+Shift+Tab — next / previous tab.',
          ],
        },
      },
      {
        title: { uk: 'Групи вкладок', en: 'Tab groups' },
        body: {
          uk: [
            'Вкладки можна об’єднувати в кольорові групи (меню вкладки → група).',
            'Зручно для кількох логів однієї інцидентної сесії; групу можна закрити разом.',
          ],
          en: [
            'Tabs can be put into colored groups (tab menu → group).',
            'Handy for several logs in one incident session; a group can be closed together.',
          ],
        },
      },
    ],
  },
  {
    id: 'search',
    title: { uk: 'Пошук', en: 'Search' },
    items: [
      {
        title: { uk: 'Як працює пошук', en: 'How search works' },
        body: {
          uk: [
            'Пошук виконується на бекенді по індексу рядків (паралельно по чанках), без завантаження всього файлу в UI.',
            'Results — віртуалізований список: у пам’яті лише видимі рядки.',
            'Клік по рядку в results підсвічує його і переходить до відповідного номера рядка в повному логу.',
            'При увімкненому переносі рядків позиція переходу враховує різну висоту рядків.',
          ],
          en: [
            'Search runs on the backend over the line index (chunked, parallel) without loading the whole file into the UI.',
            'Results are virtualized: only visible rows stay in memory.',
            'Click a results row to highlight it and jump to that line number in the full log.',
            'With word wrap on, jump position accounts for variable row heights.',
          ],
        },
      },
      {
        title: { uk: 'Основний запит і AND', en: 'Primary query and AND' },
        body: {
          uk: [
            'Основний запит шукає підрядок або regex (залежно від .* ) у всьому рядку.',
            'AND-поле додатково вимагає другого збігу в тому ж рядку (логіка «і»).',
            'Порожній primary + активні розширені фільтри (рівні, час…) — спочатку всі кандидати в діапазоні, потім пост-фільтри.',
          ],
          en: [
            'Primary query matches a substring or regex (depending on .*) in the whole line.',
            'The AND field additionally requires a second match on the same line (logical AND).',
            'Empty primary + active advanced filters (levels, time…) — all candidates in range first, then post-filters.',
          ],
        },
        example: {
          uk: [
            'Знайти помилки з конкретним host:',
            '  primary: ERROR',
            '  AND: api-gateway',
          ],
          en: [
            'Find errors for a specific host:',
            '  primary: ERROR',
            '  AND: api-gateway',
          ],
        },
      },
      {
        title: { uk: 'Regex', en: 'Regex' },
        body: {
          uk: [
            'Увімкніть .* — запит трактується як регулярний вираз (синтаксис RE2/Go).',
            'Невалідний regex підсвічується в полі пошуку; пошук не запускається, доки вираз некоректний.',
          ],
          en: [
            'Enable .* — the query is treated as a regular expression (RE2/Go syntax).',
            'Invalid regex is highlighted in the search box; search does not run until the pattern is valid.',
          ],
        },
        example: {
          uk: ['ERROR|FATAL|panic:', 'status=5\\d{2}'],
          en: ['ERROR|FATAL|panic:', 'status=5\\d{2}'],
        },
      },
    ],
  },
  {
    id: 'advanced',
    title: { uk: 'Розширені фільтри', en: 'Advanced filters' },
    items: [
      {
        title: { uk: 'Загальна ідея', en: 'General idea' },
        body: {
          uk: [
            'Рядок чіпів над results: Поруч, Рівні, Час, Виключити, Діапазон, Топ, Пресети.',
            'Підсвічений чіп = фільтр активний. Обводка = відкрита форма налаштування.',
            'Клік по чіпу відкриває форму під рядком; повторний клік або Escape — закриває.',
            '«Застосувати» зберігає значення і перезапускає пошук; «Скинути» очищає цей фільтр.',
            'Фільтри комбінуються з primary/AND (спочатку збіги, потім пост-обробка).',
          ],
          en: [
            'Chip row above results: Nearby, Levels, Time, Exclude, Range, Top, Presets.',
            'Highlighted chip = filter active. Outline = its form is open.',
            'Click a chip to open the form under the bar; click again or Escape to close.',
            'Apply saves values and re-runs search; Clear resets that filter.',
            'Filters combine with primary/AND (matches first, then post-processing).',
          ],
        },
      },
      {
        title: { uk: 'Поруч (Nearby)', en: 'Nearby' },
        body: {
          uk: [
            'Залишає лише ті рядки primary-збігу, біля яких у вікні ±N рядків є інший патерн.',
            'Режим «перед» — патерн вище збігу; «після» — нижче; «будь-де» — з будь-якого боку.',
            'Типовий сценарій: знайти ERROR, біля якого є конкретний request id або stack frame.',
          ],
          en: [
            'Keeps only primary-match lines that have another pattern within ±N lines.',
            'Mode before — pattern above the match; after — below; either — either side.',
            'Typical case: find ERROR near a specific request id or stack frame.',
          ],
        },
        example: {
          uk: [
            'primary: Exception',
            'Поруч: UserService',
            'режим: перед, вікно: 15',
          ],
          en: [
            'primary: Exception',
            'Nearby: UserService',
            'mode: before, window: 15',
          ],
        },
      },
      {
        title: { uk: 'Рівні', en: 'Levels' },
        body: {
          uk: [
            'Евристика за текстом рядка: ERROR (та FATAL/CRITICAL), WARN, INFO, DEBUG/TRACE, OTHER.',
            'Можна обрати кілька рівнів. OTHER — усе, що не схоже на відомі маркери.',
            'Без primary запиту працює як «усі рядки обраних рівнів» (з урахуванням діапазону).',
          ],
          en: [
            'Heuristic on line text: ERROR (plus FATAL/CRITICAL), WARN, INFO, DEBUG/TRACE, OTHER.',
            'Multiple levels can be selected. OTHER is everything that does not look like known markers.',
            'Without a primary query it means “all lines of selected levels” (respecting range).',
          ],
        },
      },
      {
        title: { uk: 'Час', en: 'Time' },
        body: {
          uk: [
            'Обмеження за датою й опційно часом. Ввід з клавіатури: дата YYYY-MM-DD (можна набрати 20261007).',
            'Години і хвилини — окремі поля; після двох цифр годин фокус переходить на хвилини.',
            'Час не обов’язковий: лише дата «Від» = з 00:00:00, лише «До» = до 23:59:59 того дня.',
            '«Лише з timestamp» відкидає рядки, у яких час не розпізнано.',
          ],
          en: [
            'Limit by date and optionally time. Type the date as YYYY-MM-DD (or 20261007).',
            'Hours and minutes are separate fields; after two hour digits focus moves to minutes.',
            'Time is optional: date-only From = 00:00:00, date-only To = 23:59:59 that day.',
            'Require timestamp drops lines where time could not be parsed.',
          ],
        },
        example: {
          uk: ['Від: 2026-10-07', 'До: 2026-10-07', '→ усі події за день'],
          en: ['From: 2026-10-07', 'To: 2026-10-07', '→ all events that day'],
        },
      },
      {
        title: { uk: 'Виключити', en: 'Exclude' },
        body: {
          uk: [
            'Список патернів (один на рядок). Рядок results, що містить будь-який з них, прибирається.',
            'Опція regex — трактувати патерни як регулярні вирази.',
            'Зручно відсікати healthcheck, metrics scrape, шумні DEBUG.',
          ],
          en: [
            'List of patterns (one per line). Any results line containing one of them is removed.',
            'Regex option treats patterns as regular expressions.',
            'Handy to drop healthchecks, metrics scrapes, noisy DEBUG lines.',
          ],
        },
      },
      {
        title: { uk: 'Діапазон', en: 'Range' },
        body: {
          uk: [
            'Увесь файл — пошук без обмеження по позиції.',
            'Від курсора вниз — від видимого рядка в панелі логу (зручно після ручного скролу до місця інциденту).',
            'Останні N рядків — лише хвіст файлу; суттєво швидше на гігантських логах, якщо важливий лише кінець.',
          ],
          en: [
            'Entire file — search with no position limit.',
            'From cursor down — from the visible line in the log pane (useful after scrolling to an incident).',
            'Last N lines — file tail only; much faster on huge logs when only the end matters.',
          ],
        },
      },
      {
        title: { uk: 'Топ', en: 'Top' },
        body: {
          uk: [
            'Не окремий пошук по диску, а агрегація вже знайдених results.',
            'Рядки нормалізуються (timestamp, довгі числа) і групуються; показується кількість × шаблон.',
            'Увімкнення Топ показує список поверх results; вимкнення повертає звичайний список збігів.',
            'Ліміт унікальних рядків задається в Налаштування → Пошук.',
          ],
          en: [
            'Not a separate disk search — aggregation of current results.',
            'Lines are normalized (timestamps, long numbers) and grouped; shows count × template.',
            'Enabling Top overlays the list on results; disabling restores the normal match list.',
            'Unique-row limit is in Settings → Search.',
          ],
        },
      },
      {
        title: { uk: 'Пресети', en: 'Presets' },
        body: {
          uk: [
            'Зберігає поточний набір розширених фільтрів під ім’ям у localStorage браузера/webview.',
            'Завантаження пресета застосовує збережені значення й перезапускає пошук.',
          ],
          en: [
            'Saves the current advanced-filter set under a name in localStorage.',
            'Loading a preset applies saved values and re-runs search.',
          ],
        },
      },
    ],
  },
  {
    id: 'settings',
    title: { uk: 'Налаштування', en: 'Settings' },
    items: [
      {
        title: { uk: 'Вигляд', en: 'Appearance' },
        body: {
          uk: [
            'Тема: системна / темна / світла.',
            'Мова інтерфейсу: системна / українська / English.',
            'Шрифт і розмір тексту логу, щільність рядків (впливає на висоту рядка).',
          ],
          en: [
            'Theme: system / dark / light.',
            'UI language: system / Ukrainian / English.',
            'Log font and size, line density (affects row height).',
          ],
        },
      },
      {
        title: { uk: 'Перегляд', en: 'Viewer' },
        body: {
          uk: [
            'Режим панелей за замовчуванням (split / лише results / звичайний).',
            'Follow увімкнений при відкритті файлу.',
            'Перенос довгих рядків (word wrap) — зручно читати, але важчий скрол на дуже великих файлах.',
            'Показ номерів рядків.',
          ],
          en: [
            'Default pane mode (split / results only / normal).',
            'Follow enabled when opening a file.',
            'Word wrap — easier reading, but heavier scrolling on huge files.',
            'Show line numbers.',
          ],
        },
      },
      {
        title: { uk: 'Пошук (налаштування)', en: 'Search (settings)' },
        body: {
          uk: [
            'Контекст ±N — запас для майбутнього/сумісності; у results зараз показуються самі збіги.',
            'Режим і вікно «Поруч» за замовчуванням.',
            'Виключення як regex за замовчуванням.',
            'Лише рядки з timestamp — дефолт для фільтра Час.',
            'Ліміт Топ — скільки унікальних шаблонів показувати.',
            'Підсвітка термів: primary, AND, поруч, виключення — кольорові маркери в тексті логу.',
          ],
          en: [
            '±N context — reserved for future/compat; results currently show match lines themselves.',
            'Default Nearby mode and window.',
            'Exclude-as-regex by default.',
            'Require timestamp — default for the Time filter.',
            'Top limit — how many unique templates to show.',
            'Term highlighting: primary, AND, nearby, exclude — colored markers in log text.',
          ],
        },
      },
      {
        title: { uk: 'Сесія', en: 'Session' },
        body: {
          uk: [
            'Зберігати вкладки при виході, розмір списку нещодавніх, глибина історії вводу.',
            'Підтвердження закриття вкладок з активним SSH/FTP.',
          ],
          en: [
            'Save tabs on exit, recent-list size, input-history depth.',
            'Confirm closing tabs with active SSH/FTP.',
          ],
        },
      },
      {
        title: { uk: 'Віддалені', en: 'Remote' },
        body: {
          uk: [
            'Таймаут з’єднання SSH/FTP і stall timeout для повільних закачувань.',
            'Авто-прийняття unknown host key — зручно, але небезпечно в недовірених мережах (позначено як небезпечне).',
            'Ліміт невдалих спроб і час блокування після серії помилок.',
          ],
          en: [
            'SSH/FTP connect timeout and stall timeout for slow downloads.',
            'Auto-accept unknown host key — convenient but unsafe on untrusted networks (marked dangerous).',
            'Failed-attempt limit and lockout duration after repeated errors.',
          ],
        },
      },
    ],
  },
  {
    id: 'remote',
    title: { uk: 'SSH і FTP', en: 'SSH & FTP' },
    items: [
      {
        title: { uk: 'Підключення', en: 'Connecting' },
        body: {
          uk: [
            'Форма: хост, порт, логін, пароль або ключ, шлях до файлу на сервері.',
            'Можна спочатку підключитися й обрати файл у віддаленому браузері каталогів.',
            'Невідомий SSH host key — запит підтвердження відбитка (якщо авто-accept вимкнено).',
          ],
          en: [
            'Form: host, port, user, password or key, path to the file on the server.',
            'You can connect first and pick a file in the remote directory browser.',
            'Unknown SSH host key — fingerprint confirmation prompt (if auto-accept is off).',
          ],
        },
      },
      {
        title: { uk: 'Після відкриття', en: 'After opening' },
        body: {
          uk: [
            'Файл з’являється як вкладка; пошук і фільтри працюють як для локального файлу.',
            'Follow періодично підтягує нові дані з сервера (залежно від протоколу й налаштувань).',
          ],
          en: [
            'The file appears as a tab; search and filters work like for a local file.',
            'Follow periodically pulls new data from the server (depends on protocol and settings).',
          ],
        },
      },
    ],
  },
  {
    id: 'shortcuts',
    title: { uk: 'Клавіші', en: 'Shortcuts' },
    items: [
      {
        title: { uk: 'Список', en: 'List' },
        body: {
          uk: [
            'Ctrl+O — відкрити файли',
            'Ctrl+R — нещодавні',
            'Ctrl+W — закрити вкладку',
            'Ctrl+Tab / Ctrl+Shift+Tab — наступна / попередня вкладка',
            '/ — фокус у пошук',
            'Enter у пошуку — шукати зараз',
            'F — follow',
            'F1 — довідка',
            'F3 / Shift+F3 — наступний / попередній збіг',
            'Ctrl+G — перейти до рядка',
            'Ctrl+C — копіювати виділення',
            'Ctrl+Shift+C — копіювати цілі рядки',
            'Escape — закрити діалог / форму фільтра',
          ],
          en: [
            'Ctrl+O — open files',
            'Ctrl+R — recent',
            'Ctrl+W — close tab',
            'Ctrl+Tab / Ctrl+Shift+Tab — next / previous tab',
            '/ — focus search',
            'Enter in search — search now',
            'F — follow',
            'F1 — help',
            'F3 / Shift+F3 — next / previous match',
            'Ctrl+G — go to line',
            'Ctrl+C — copy selection',
            'Ctrl+Shift+C — copy full lines',
            'Escape — close dialog / filter form',
          ],
        },
      },
    ],
  },
  {
    id: 'cli',
    title: { uk: 'CLI', en: 'CLI' },
    items: [
      {
        title: { uk: 'Компаньйон у терміналі', en: 'Terminal companion' },
        body: {
          uk: [
            'Окремий бінарник gofle (CLI) для швидкого перегляду й пошуку без GUI.',
            'Пароль для remote запитується інтерактивно в полі вводу, а не як аргумент командного рядка.',
            'Докладні прапорці та приклади — у README (розділ CLI Companion).',
          ],
          en: [
            'Separate gofle CLI binary for quick view and search without the GUI.',
            'Remote password is prompted interactively, not as a command-line argument.',
            'Full flags and examples — see README (CLI Companion section).',
          ],
        },
      },
    ],
  },
  {
    id: 'about',
    title: { uk: 'Про додаток', en: 'About' },
    items: [
      {
        title: { uk: 'Go Fast Log Explorer', en: 'Go Fast Log Explorer' },
        body: {
          uk: [
            'Швидкий інтерактивний переглядач дуже великих лог-файлів.',
            'Локальні шляхи, SSH і FTP/FTPS, кілька вкладок, live-follow, пошук і фільтри.',
            'Ліцензія додатку: MIT.',
            'Повний список сторонніх бібліотек — вкладка «Ліцензії» та файл LICENSES.md у репозиторії.',
          ],
          en: [
            'Fast interactive viewer for very large log files.',
            'Local paths, SSH and FTP/FTPS, multiple tabs, live follow, search and filters.',
            'Application license: MIT.',
            'Full third-party list — the Licenses tab and LICENSES.md in the repository.',
          ],
        },
      },
    ],
  },
  {
    id: 'licenses',
    title: { uk: 'Ліцензії', en: 'Licenses' },
    items: [
      {
        title: { uk: 'Сторонні бібліотеки', en: 'Third-party libraries' },
        body: {
          uk: [
            'Нижче перелічені основні залежності (назва, посилання, ліцензія).',
            'Повний опис — у файлі LICENSES.md у корені проєкту.',
          ],
          en: [
            'Main dependencies are listed below (name, link, license).',
            'Full descriptions are in LICENSES.md at the project root.',
          ],
        },
      },
    ],
  },
];

/** Libraries shown on the Licenses help tab (name, url, license). */
export type LicenseEntry = {
  name: string;
  url: string;
  license: string;
};

export const LICENSE_ENTRIES: LicenseEntry[] = [
  { name: 'wails/v2', url: 'https://github.com/wailsapp/wails', license: 'MIT' },
  { name: 'charmbracelet/bubbletea', url: 'https://github.com/charmbracelet/bubbletea', license: 'MIT' },
  { name: 'charmbracelet/bubbles', url: 'https://github.com/charmbracelet/bubbles', license: 'MIT' },
  { name: 'charmbracelet/lipgloss', url: 'https://github.com/charmbracelet/lipgloss', license: 'MIT' },
  { name: 'spf13/cobra', url: 'https://github.com/spf13/cobra', license: 'Apache-2.0' },
  { name: 'jlaffaye/ftp', url: 'https://github.com/jlaffaye/ftp', license: 'ISC' },
  { name: 'golang.org/x/crypto', url: 'https://pkg.go.dev/golang.org/x/crypto', license: 'BSD-3-Clause' },
  { name: 'golang.org/x/term', url: 'https://pkg.go.dev/golang.org/x/term', license: 'BSD-3-Clause' },
  { name: 'golang.org/x/sync', url: 'https://pkg.go.dev/golang.org/x/sync', license: 'BSD-3-Clause' },
  { name: 'mattn/go-runewidth', url: 'https://github.com/mattn/go-runewidth', license: 'MIT' },
  { name: 'muesli/termenv', url: 'https://github.com/muesli/termenv', license: 'MIT' },
  { name: 'JetBrains Mono', url: 'https://www.jetbrains.com/lp/mono/', license: 'OFL-1.1' },
  { name: 'Fira Code', url: 'https://github.com/tonsky/FiraCode', license: 'OFL-1.1' },
  { name: 'Cascadia Code', url: 'https://github.com/microsoft/cascadia-code', license: 'OFL-1.1' },
  { name: 'Vite', url: 'https://vitejs.dev/', license: 'MIT' },
  { name: 'TypeScript', url: 'https://www.typescriptlang.org/', license: 'Apache-2.0' },
];

export function helpLangFromApp(lang: string): HelpLang {
  if (lang === 'en' || (lang && lang.toLowerCase().startsWith('en'))) return 'en';
  return 'uk';
}
