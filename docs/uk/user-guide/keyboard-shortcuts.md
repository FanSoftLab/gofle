# Клавіатурні скорочення

> **Тип контенту**: Reference — повні клавіатурні скорочення для GoFLE GUI та термінального viewer.
> **Аудиторія**: Усі користувачі
> **Verified**: 2026-10-29 — відповідає фактичним keybindings в коді

## Призначення

Повна довідка по всіх клавіатурних скороченнях для ефективного використання GoFLE.

---

## GUI Скорочення

### Файли та навігація

| Скорочення | Дія | Контекст |
|------------|-----|----------|
| `Ctrl+O` | Відкрити файл(и) | Global |
| `Ctrl+R` | Нещодавні файли | Global |
| `Ctrl+G` | Перейти до рядка | Global |
| `Ctrl+W` | Закрити поточну вкладку | Global |
| `Ctrl+Tab` | Наступна вкладка | Global |
| `Ctrl+Shift+Tab` | Попередня вкладка | Global |
| `1`–`9` | Перейти до вкладки 1–9 | Global |
| `Ctrl+Shift+T` | Відкрити закриту вкладку | Global |

### Пошук

| Скорочення | Дія | Контекст |
|------------|-----|----------|
| `/` | Фокус на пошук | Global |
| `Enter` | Виконати пошук | Поле пошуку |
| `Esc` | Очистити пошук / закрити діалог | Поле пошуку, діалоги |
| `Alt+C` | Toggle Match Case | Global |
| `Alt+R` | Toggle Regex | Global |
| `Alt+I` | Toggle Invert Match | Global |
| `\` | Фокус на AND поле | Global |
| `F3` / `Alt+↓` | Next match | Global |
| `Shift+F3` / `Alt+↑` | Previous match | Global |
| `Alt+Home` | First match | Global |
| `Alt+End` | Last match | Global |

### Follow & View

| Скорочення | Дія | Контекст |
|------------|-----|----------|
| `F` | Toggle Follow (Tail) | Global |
| `r` | Toggle Results Only view | Global |
| `s` / `m` | Split view / cycle view mode | Global |

### Help & Settings

| Скорочення | Дія | Контекст |
|------------|-----|----------|
| `F1` | Відкрити Help | Global |
| `?` | Відкрити Help | Global |
| `Ctrl+,` | Відкрити Settings | Global |

### Експорт

| Скорочення | Дія | Контекст |
|------------|-----|----------|
| `Ctrl+S` | Зберегти результати пошуку | Results pane focused |
| `Ctrl+Shift+S` | Зберегти повний лог | Log pane focused |

### Діалоги та форми

| Скорочення | Дія | Контекст |
|------------|-----|----------|
| `Esc` | Закрити діалог / зняти фокус | Будь-який діалог |
| `Enter` | Підтвердити форму / застосувати фільтр | Форми |
| `Tab` / `Shift+Tab` | Next/previous поле | Форми |

---

## Terminal Viewer Скорочення (CLI `gofle`)

### Навігація

| Клавіша | Дія |
|---------|-----|
| `↑` / `↓` | Прокрутка по рядках |
| `PgUp` / `PgDn` | Прокрутка сторінками |
| `Home` / `End` | Перехід на початок / кінець |
| `g` | Перейти до рядка (запитує номер) |

### Пошук

| Клавіша | Дія |
|---------|-----|
| `/` | Почати пошук (відкриває input знизу) |
| `Enter` (у пошуку) | Виконати пошук |
| `Esc` (у пошуку) | Скасувати введення пошуку |
| `Alt+R` | Toggle Regex mode |
| `Alt+I` | Toggle Invert Match |
| `Alt+C` | Toggle Match Case |
| `n` / `p` | Next / Previous match |
| `[` / `]` | First / Last match |
| `\` | Secondary AND filter (відкриває AND input) |

### Розширені фільтри

| Клавіша | Дія |
|---------|-----|
| `A` | Відкрити форму Advanced Filters (Nearby, Levels, Time, Exclude, Range, Top, Presets) |
| `u` | Toggle Top Unique overlay |
| `P` | Відкрити Search Presets (load/save/delete) |

### Вкладки та Групи

| Клавіша | Дія |
|---------|-----|
| `Ctrl+Tab` / `Ctrl+PgDn` | Наступна вкладка |
| `Ctrl+Shift+Tab` / `Ctrl+PgUp` | Попередня вкладка |
| `1`–`9` | Перейти до вкладки N |
| `T` | Відкрити Recent list → відкрити вибір як нову вкладку |
| `t` | У Recent picker: відкрити виділене як нову вкладку |
| `G` | Tab Group — назва/колір для активної вкладки |
| `Y` | Відновити вкладки з збереженої сесії |
| `Ctrl+W` | Закрити поточну вкладку (остання вкладка виходить) |

### View & Export

| Клавіша | Дія |
|---------|-----|
| `r` | Toggle Results Only view (filter mode) |
| `s` / `m` | Split view / cycle view mode (Log+Results / Results Only / Log Only) |
| `f` | Toggle Follow (Tail) |
| `e` | Export — відкриває діалог експорту (whole file або matches) |
| `v` | Toggle view mode |

### Settings & Help

| Клавіша | Дія |
|---------|-----|
| `F2` / `,` | Відкрити Settings (view mode, search defaults, context/nearby/top) |
| `?` | Показати повний help overlay |
| `q` / `Ctrl+C` | Вийти з viewer |

### Clipboard

| Клавіша | Дія |
|---------|-----|
| `Ctrl+C` | Копіювати виділення (якщо текст виділений) |
| `Ctrl+Shift+C` | Копіювати повні рядки (цілі видимі рядки) |

---

## Context Menu (Right-Click)

Недоступний у термінальному viewer. Використовуйте keybindings вище.

---

## Mouse Support

- **Scroll wheel**: Прокрутка log/results
- **Click**: Фокус панелі, вибір рядка
- **Drag**: Виділення тексту (для `Ctrl+C`)

Mouse support залежить від термінального емулятора (більшість сучасних підтримує).

---

## Search Input Mode

Коли `/` відкриває search input:

| Клавіша | Дія |
|---------|-----|
| `Tab` | Цикл: Primary → AND → Regex → Case → Invert → Search History |
| `Shift+Tab` | Reverse cycle |
| `Enter` | Виконати пошук |
| `Esc` | Закрити search input |
| `↑` / `↓` | Навігація по search history |

---

## Advanced Filters Form

Коли `A` відкриває форму:

| Клавіша | Дія |
|---------|-----|
| `Tab` / `Shift+Tab` | Навігація полями |
| `Enter` | Apply / save поточний фільтр |
| `Esc` | Закрити форму |
| `Space` | Toggle checkboxes (Levels, etc.) |
| `←` / `→` | Зміна radio опцій (Nearby mode, Range mode) |

---

## Presets Menu

Коли `P` відкриває пресети:

| Клавіша | Дія |
|---------|-----|
| `↑` / `↓` | Навігація пресетами |
| `Enter` | Завантажити пресет |
| `n` | Новий пресет (зберегти поточні фільтри) |
| `d` | Видалити виділений пресет |
| `Esc` | Закрити |

---

## Settings Menu

Коли `F2` / `,` відкриває settings:

| Клавіша | Дія |
|---------|-----|
| `Tab` / `Shift+Tab` | Навігація секціями |
| `Enter` / `Space` | Toggle / редагувати значення |
| `Esc` | Закрити та зберегти |

---

## Tab Indicators

Tab bar показує:
- Ім'я вкладки (скорочене)
- Індикатор кольору групи (якщо згруповано)
- Remote badge: `SSH` / `FTP` (для remote вкладок)
- Активна вкладка підсвічена

---

## Status Bar

Нижній бар показує:
- Шлях до файлу / remote URL
- Кількість рядків / кількість match
- Follow state: `Tail: ACTIVE` / `Tail: PAUSED` / `Tail: —`
- Search state: `Results Only` badge (натисніть `r` для toggle)
- Поточний час (опціонально)

---

## Колірне підсвічування

| Елемент | Колір | Toggle |
|---------|-------|--------|
| Primary match | Gold/Yellow | Settings |
| AND match | Green | Settings |
| Nearby match | Cyan | Settings |
| Exclude match | Red | Settings |
| Line numbers | Dim | Settings |

Кольори налаштовуються в Settings (спільно з GUI через `settings.json`).

---

## Пов'язані теми

- [Основи пошуку](search-basics.md)
- [Розширений пошук](search-advanced.md)
- [Вкладки та групи](tabs-groups.md)
- [Follow (Live Tail)](follow-tail.md)
- [CLI Viewer Keys](../cli-reference/cli-viewer-keys.md)