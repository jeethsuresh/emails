import type { AnalyzerStatus, SyncStatus } from "../../shared/types";
import { SidebarProgress } from "./SidebarProgress";

interface Folder {
  id: string;
  name: string;
  role: string;
}

export function FolderList({
  folders,
  selected,
  onSelect,
  contactsActive,
  onSelectContacts,
  syncStatus,
  analyzerStatus,
  downloadingBody,
}: {
  folders: Folder[];
  selected: string | null;
  onSelect: (id: string) => void;
  contactsActive: boolean;
  onSelectContacts: () => void;
  syncStatus: SyncStatus | null;
  analyzerStatus: AnalyzerStatus | null;
  downloadingBody: boolean;
}) {
  return (
    <aside className="folders">
      <div className="folders-body">
        <div className="folders-heading">
          <h2>Folders</h2>
          <button
            type="button"
            className={contactsActive ? "contacts-control active" : "contacts-control"}
            onClick={onSelectContacts}
          >
            Contacts
          </button>
        </div>
        <ul>
          {folders.map((f) => (
            <li key={f.id}>
              <button
                type="button"
                className={selected === f.id ? "active" : ""}
                onClick={() => onSelect(f.id)}
              >
                <span className="clamp-2">{f.name}</span>
                <em className="clamp-2">{f.role}</em>
              </button>
            </li>
          ))}
          {folders.length === 0 && <li className="empty">No folders yet — hit Sync</li>}
        </ul>
      </div>
      <SidebarProgress
        syncStatus={syncStatus}
        analyzerStatus={analyzerStatus}
        downloadingBody={downloadingBody}
      />
    </aside>
  );
}
