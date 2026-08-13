# Progressive Responsive UI — Design Spec (2026-08-13)

## Goal

Make the Email app work as a progressive UI from phone → tablet → desktop without data overflow. Same React surface; layout adapts by viewport. Preserve the existing warm paper + pine accent visual language.

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Targets | Phone through desktop (Electron narrow windows + future mobile wrap) |
| Primary nav (narrow) | **Bottom tabs**: Mail / Todos / Calendar |
| Primary nav (wide) | Existing top tabs |
| Mail on narrow | Stack: Folders → List → Reading with back |
| Overflow | Wrap **2–3 lines**, then ellipsis |
| Approach | Progressive shell + explicit pane state (not CSS-only, not parallel mobile UI) |

## Non-goals

- Separate mobile codebase / Capacitor packaging (layout must *enable* it later)
- Visual rebrand (keep tokens: `--bg`, `--panel`, `--accent`, fonts)
- Drag-resize pane polish beyond workable responsive behavior
- Changing sync/backend APIs

---

## 1. Breakpoints

| Token | Width | Role |
| --- | --- | --- |
| `phone` | `< 640px` | Bottom tabs; single-pane stacks; compact chrome |
| `tablet` | `640px–1023px` | Bottom tabs or compact top chrome; mail may show list+reading; folders drawer |
| `desktop` | `≥ 1024px` | Top tabs; 3-pane mail; calendar rail beside grid |

Use CSS custom media / class on `.shell` from a small `useViewport()` hook (`phone` | `tablet` | `desktop`) so React can drive pane visibility and nav placement. Prefer one source of truth (matchMedia), not duplicated magic numbers in many files.

---

## 2. App chrome

### Wide (`desktop`)

- Keep topbar: brand · tabs · search (mail) · Compose / Settings / Sync · status
- Topbar actions may collapse into an overflow menu if cramped, but desktop default stays expanded
- No bottom tab bar

### Narrow (`phone` / `tablet`)

- **Top:** compact bar — back (when stacked), title/context, search icon or field, overflow (Compose / Settings / Sync)
- **Bottom:** fixed tab bar — Mail | Todos | Calendar (safe-area padding)
- Bottom bar does not scroll away; content scrolls in the main region only
- Sync badge can be a small indicator on the overflow or Sync action — must not push layout wider than the viewport

### Overflow rule for chrome

- Toolbar action groups: allow wrap once; if still overflowing, move secondary actions into a “More” menu
- Never horizontal page scroll for the shell

---

## 3. Mail progressive stack

Explicit state: `mailPane: "folders" | "list" | "reading"`.

| Viewport | Behavior |
| --- | --- |
| `phone` | One pane visible. Select folder → list; select message → reading; Back steps up. |
| `tablet` | Prefer list + reading side-by-side when width allows; folders in a slide-over / drawer. If too narrow for split, same stack as phone. |
| `desktop` | All three panes; selecting does not hide panes. |

Rules:

- Changing folder resets to list (clears reading selection on phone)
- Back from reading → list; from list → folders
- Hardware/browser back is out of scope for Electron v1; in-app Back is required
- Each pane is `min-width: 0`; lists and reading panes scroll internally — no document-level horizontal overflow

---

## 4. Todos & Calendar

- Reuse the same shell (bottom/top tabs)
- **Todos:** single column; rows use 2–3 line clamp on title/notes; actions wrap under content on phone
- **Calendar:**
  - Toolbar wraps; day-count / TZ controls stack on phone
  - Multi-day: horizontal scroll *inside* the grid only (not the whole app), or reduce to day view by default on phone
  - Month/year grids use `minmax(0, 1fr)`; chips clamp to 2 lines
  - Calendar rail becomes a bottom sheet or collapsible section on phone

---

## 5. Text overflow system

Shared utilities (CSS):

- `.clamp-2` / `.clamp-3` — `-webkit-line-clamp` + `overflow: hidden`; `word-break: break-word`; `overflow-wrap: anywhere` for long tokens
- Apply to: message subject/snippet/from, todo titles/notes, calendar event titles, folder names, sync status message
- Detail views (message body, event modal): wrap freely; body scrolls in a contained pane
- Modals: `max-height: min(90vh, …)`; body scrolls; never wider than `100vw - padding`

**No data should overflow** means:

1. No horizontal scrollbar on `body` / `.shell`
2. No text painting outside cards/rows
3. Long unbroken strings break rather than expand the layout

---

## 6. Files / structure

| Unit | Responsibility |
| --- | --- |
| `src/lib/viewport.ts` | `useViewport()` + breakpoint constants |
| `src/components/AppChrome.tsx` | Top/bottom nav, overflow menu, search placement |
| `src/components/MailShell.tsx` | Mail pane stack / 3-pane layout |
| `src/styles.css` | Breakpoint layout, clamps, safe-area, bottom tabs |
| `src/App.tsx` | Wire chrome + mail shell; keep data logic |

Keep FolderList / MessageList / MessageView mostly presentational; MailShell owns which is shown.

---

## 7. Verification

- Resize Electron from ~375px → 1280px+: no horizontal overflow; bottom tabs appear/disappear correctly
- Phone: Mail folders → list → reading → Back works; tabs switch Todos/Calendar
- Long subject / URL / calendar title clamps to 2–3 lines without expanding row width
- Calendar multi-day does not push the page wider than the viewport
- Desktop: current 3-pane mail and top tabs still feel familiar

## Risks

- Tablet split heuristics (when to show list+reading vs stack) — default: stack below 768px content width, split from 768–1023 if both panes fit
- Bottom tabs + safe area on notched devices — use `env(safe-area-inset-*)`
- Calendar density on phone — default view Day or List on `phone` if multi-day overflows
