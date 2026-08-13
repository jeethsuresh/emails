import { useEffect, useRef, useState } from "react";
import type PocketBase from "pocketbase";
import { decodeMIMEWords } from "../lib/mimeWords";
import {
  contactMessages,
  listContacts,
  type Contact,
  type ThreadMessage,
} from "../lib/mailApi";

export function ContactsView({
  pb,
  selectedMessageId,
  onSelectMessage,
}: {
  pb: PocketBase;
  selectedMessageId: string | null;
  onSelectMessage: (message: ThreadMessage) => void;
}) {
  const [contacts, setContacts] = useState<Contact[]>([]);
  const [selectedContact, setSelectedContact] = useState<Contact | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const requestSeq = useRef(0);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    void listContacts(pb)
      .then((result) => {
        if (!cancelled) setContacts(result.items);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [pb]);

  const selectContact = async (contact: Contact) => {
    const seq = ++requestSeq.current;
    setSelectedContact(contact);
    setMessages([]);
    setLoading(true);
    setError(null);
    try {
      const result = await contactMessages(pb, contact.email);
      if (requestSeq.current === seq) setMessages(result.items);
    } catch (err) {
      if (requestSeq.current === seq) {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (requestSeq.current === seq) setLoading(false);
    }
  };

  return (
    <section className="messages contacts-view" aria-busy={loading}>
      <h2>
        {selectedContact ? (
          <button
            type="button"
            className="contacts-back"
            onClick={() => {
              requestSeq.current += 1;
              setSelectedContact(null);
              setMessages([]);
              setError(null);
            }}
          >
            ‹ Contacts
          </button>
        ) : (
          "Contacts"
        )}
        {!selectedContact && contacts.length > 0 ? (
          <span className="count">{contacts.length}</span>
        ) : null}
        {loading ? <span className="count">Loading…</span> : null}
      </h2>
      {selectedContact ? (
        <div className="contact-heading">
          <strong>{selectedContact.display_name || selectedContact.email}</strong>
          <span>{selectedContact.email}</span>
        </div>
      ) : null}
      {error ? <p className="error">{error}</p> : null}
      <div className="messages-scroll">
        {!selectedContact ? (
          contacts.length === 0 && !loading ? (
            <p className="empty">No contacts yet</p>
          ) : (
            <ul className="contact-list">
              {contacts.map((contact) => (
                <li key={contact.id}>
                  <button type="button" className="row" onClick={() => void selectContact(contact)}>
                    <div className="meta">
                      <strong className="clamp-2">{contact.display_name || contact.email}</strong>
                      <span>{contact.message_count}</span>
                    </div>
                    {contact.display_name ? <div className="snippet">{contact.email}</div> : null}
                    <time>
                      {contact.last_message_at
                        ? new Date(contact.last_message_at).toLocaleString()
                        : ""}
                    </time>
                  </button>
                </li>
              ))}
            </ul>
          )
        ) : messages.length === 0 && !loading ? (
          <p className="empty">No messages for this contact</p>
        ) : (
          <ul>
            {messages.map((message) => (
              <li key={message.id} className={selectedMessageId === message.id ? "active" : ""}>
                <button type="button" className="row" onClick={() => onSelectMessage(message)}>
                  <div className="meta">
                    <strong className={message.seen ? "" : "unread"}>
                      {message.from_addr || "(unknown)"}
                    </strong>
                    <time>{message.date ? new Date(message.date).toLocaleString() : ""}</time>
                  </div>
                  <div className="subject clamp-2">
                    {decodeMIMEWords(message.subject) || "(no subject)"}
                  </div>
                  <div className="snippet">{decodeMIMEWords(message.snippet)}</div>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
