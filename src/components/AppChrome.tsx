import type { ReactNode } from "react";
import type { SyncStatus } from "../../shared/types";
import type { Viewport } from "../lib/viewport";
import { SyncBadge } from "./SyncBadge";

export type AppTab = "mail" | "todos" | "calendar";

export function AppChrome({
  viewport,
  activeTab,
  onTabChange,
  title,
  showBack,
  onBack,
  search,
  onSearchChange,
  showSearch,
  onCompose,
  onSettings,
  onSync,
  status,
  children,
}: {
  viewport: Viewport;
  activeTab: AppTab;
  onTabChange: (tab: AppTab) => void;
  title?: string;
  showBack?: boolean;
  onBack?: () => void;
  search: string;
  onSearchChange: (q: string) => void;
  showSearch: boolean;
  onCompose: () => void;
  onSettings: () => void;
  onSync: () => void;
  status: SyncStatus | null;
  children: ReactNode;
}) {
  const narrow = viewport !== "desktop";

  return (
    <div className="shell" data-vp={viewport}>
      <header className={narrow ? "topbar topbar-compact" : "topbar"}>
        {showBack ? (
          <button type="button" className="chrome-back" onClick={onBack} aria-label="Back">
            ‹ Back
          </button>
        ) : (
          <div className="brand">Email</div>
        )}
        {narrow ? <div className="chrome-title clamp-2">{title ?? tabLabel(activeTab)}</div> : null}
        {!narrow ? (
          <nav className="app-tabs" aria-label="Primary">
            <TabButton active={activeTab === "mail"} onClick={() => onTabChange("mail")}>
              Mail
            </TabButton>
            <TabButton active={activeTab === "todos"} onClick={() => onTabChange("todos")}>
              Todos
            </TabButton>
            <TabButton active={activeTab === "calendar"} onClick={() => onTabChange("calendar")}>
              Calendar
            </TabButton>
          </nav>
        ) : null}
        {showSearch && !narrow ? (
          <input
            className="search"
            placeholder="Search subject, from, or body…"
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
            aria-label="Search mail"
          />
        ) : !narrow ? (
          <div className="search-spacer" />
        ) : null}
        <div className="chrome-actions">
          {showSearch && narrow ? (
            <input
              className="search search-compact"
              placeholder="Search…"
              value={search}
              onChange={(e) => onSearchChange(e.target.value)}
              aria-label="Search mail"
            />
          ) : null}
          {!narrow ? (
            <>
              <button type="button" onClick={onCompose}>
                Compose
              </button>
              <button type="button" onClick={onSettings}>
                Settings
              </button>
              <button type="button" onClick={onSync}>
                Sync
              </button>
              <SyncBadge status={status} />
            </>
          ) : (
            <details className="chrome-more">
              <summary aria-label="More actions">More</summary>
              <div className="chrome-more-menu" role="menu">
                <button type="button" role="menuitem" onClick={onCompose}>
                  Compose
                </button>
                <button type="button" role="menuitem" onClick={onSettings}>
                  Settings
                </button>
                <button type="button" role="menuitem" onClick={onSync}>
                  Sync
                </button>
                <div className="chrome-more-status">
                  <SyncBadge status={status} />
                </div>
              </div>
            </details>
          )}
        </div>
      </header>

      <div className="shell-main">{children}</div>

      {narrow ? (
        <nav className="bottom-tabs" aria-label="Primary">
          <TabButton active={activeTab === "mail"} onClick={() => onTabChange("mail")}>
            Mail
          </TabButton>
          <TabButton active={activeTab === "todos"} onClick={() => onTabChange("todos")}>
            Todos
          </TabButton>
          <TabButton active={activeTab === "calendar"} onClick={() => onTabChange("calendar")}>
            Calendar
          </TabButton>
        </nav>
      ) : null}
    </div>
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button type="button" className={active ? "tab active" : "tab"} onClick={onClick}>
      {children}
    </button>
  );
}

function tabLabel(tab: AppTab): string {
  switch (tab) {
    case "mail":
      return "Mail";
    case "todos":
      return "Todos";
    case "calendar":
      return "Calendar";
    default: {
      const _exhaustive: never = tab;
      return _exhaustive;
    }
  }
}
