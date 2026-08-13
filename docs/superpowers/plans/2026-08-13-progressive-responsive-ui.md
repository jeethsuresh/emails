# Progressive Responsive UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans or implement task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Progressive phone→desktop shell with bottom tabs, mail pane stack, and 2–3 line clamps so nothing overflows.

**Architecture:** `useViewport()` drives `.shell` mode; `AppChrome` swaps top/bottom nav; `MailShell` owns folders/list/reading visibility; shared CSS clamps and safe-area.

**Tech Stack:** React 19, existing CSS tokens, matchMedia (no new UI libs).

## Global Constraints

- Preserve warm paper + pine tokens and fonts
- Overflow: wrap 2–3 lines then ellipsis; no body horizontal scroll
- Narrow: bottom tabs Mail/Todos/Calendar; Mail stack with Back
- Wide (≥1024): top tabs + 3-pane mail
- Spec: `docs/superpowers/specs/2026-08-13-progressive-responsive-ui-design.md`

---

## File map

| File | Role |
| --- | --- |
| `src/lib/viewport.ts` | Breakpoints + `useViewport` |
| `src/components/AppChrome.tsx` | Top/bottom chrome |
| `src/components/MailShell.tsx` | Progressive mail panes |
| `src/App.tsx` | Compose chrome + mail shell |
| `src/styles.css` | Layout, clamps, bottom tabs, safe-area |
| Calendar/Todo minor class hooks | Clamp + wrap toolbars |

---

### Task 1: Viewport hook + CSS foundation

**Files:** create `src/lib/viewport.ts`; edit `src/styles.css`

- [ ] Add breakpoints: phone `<640`, tablet `640–1023`, desktop `≥1024`
- [ ] Export `useViewport(): "phone" | "tablet" | "desktop"`
- [ ] Add `.clamp-2`, `.clamp-3`, `.shell[data-vp=…]` layout primitives, safe-area padding
- [ ] Ensure `html/body/#root/.shell` use `overflow-x: hidden` / `min-width: 0` chain

### Task 2: AppChrome

**Files:** create `src/components/AppChrome.tsx`; edit `src/App.tsx`

- [ ] Props: `viewport`, `activeTab`, setters, search, compose/settings/sync, status, optional `back` handler/label
- [ ] Desktop: current topbar layout
- [ ] Phone/tablet: compact top + bottom tab bar
- [ ] Overflow “More” menu for Compose/Settings/Sync on narrow if needed

### Task 3: MailShell stack

**Files:** create `src/components/MailShell.tsx`; edit `src/App.tsx`

- [ ] State `mailPane` with auto rules from viewport + selection
- [ ] Phone: one pane; Back navigation
- [ ] Tablet: folders drawer or stack; list+reading when width ≥768 content
- [ ] Desktop: three panes always
- [ ] Apply clamp classes to list rows via MessageList/FolderList class updates

### Task 4: Todos + Calendar overflow

**Files:** `TodoList.tsx`, `CalendarView.tsx`, `styles.css`, message list styles

- [ ] Clamp titles/notes/chips
- [ ] Calendar toolbar wrap; phone default day or list if multi-day overflows
- [ ] Rail collapses on phone

### Task 5: Verify + commit

- [ ] Typecheck
- [ ] Manual resize sanity (or note for user)
- [ ] Commit responsive UI changes
