import { ConnectionForm } from "./ConnectionForm";

export function AccountSetup({ onSaved }: { onSaved: () => Promise<void> }) {
  return (
    <ConnectionForm
      title="Email"
      subtitle="Connect an IMAP account. Mail stays local in PocketBase; sync runs in the Go backend."
      submitLabel="Save & sync"
      onSaved={onSaved}
    />
  );
}
