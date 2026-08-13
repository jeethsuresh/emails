import { useCallback, useEffect, useRef, useState } from "react";
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
  const [contactsTotal, setContactsTotal] = useState(0);
  const [contactsPage, setContactsPage] = useState(1);
  const [selectedContact, setSelectedContact] = useState<Contact | null>(null);
  const [messages, setMessages] = useState<ThreadMessage[]>([]);
  const [messagesTotal, setMessagesTotal] = useState(0);
  const [messagesPage, setMessagesPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const contactsRequestSeq = useRef(0);
  const messagesRequestSeq = useRef(0);

  const loadContacts = useCallback(async (nextPage: number, append: boolean) => {
    const seq = ++contactsRequestSeq.current;
    setLoading(true);
    setError(null);
    try {
      const result = await listContacts(pb, undefined, nextPage);
      if (seq !== contactsRequestSeq.current) return;
      setContacts((current) => (append ? [...current, ...result.items] : result.items));
      setContactsTotal(result.totalItems);
      setContactsPage(result.page);
    } catch (err) {
      if (seq === contactsRequestSeq.current) {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (seq === contactsRequestSeq.current) setLoading(false);
    }
  }, [pb]);

  useEffect(() => {
    void loadContacts(1, false);
    return () => {
      contactsRequestSeq.current += 1;
      messagesRequestSeq.current += 1;
    };
  }, [loadContacts]);

  const loadContactMessages = async (
    contact: Contact,
    nextPage: number,
    append: boolean,
  ) => {
    const seq = ++messagesRequestSeq.current;
    setLoading(true);
    setError(null);
    try {
      const result = await contactMessages(pb, contact.email, nextPage);
      if (messagesRequestSeq.current !== seq) return;
      setMessages((current) => (append ? [...current, ...result.items] : result.items));
      setMessagesTotal(result.totalItems);
      setMessagesPage(result.page);
    } catch (err) {
      if (messagesRequestSeq.current === seq) {
        setError(err instanceof Error ? err.message : String(err));
      }
    } finally {
      if (messagesRequestSeq.current === seq) setLoading(false);
    }
  };

  const selectContact = (contact: Contact) => {
    setSelectedContact(contact);
    setMessages([]);
    setMessagesTotal(0);
    setMessagesPage(1);
    void loadContactMessages(contact, 1, false);
  };

  return (
    <section className="messages contacts-view" aria-busy={loading}>
      <h2>
        {selectedContact ? (
          <button
            type="button"
            className="contacts-back"
            onClick={() => {
              messagesRequestSeq.current += 1;
              setSelectedContact(null);
              setMessages([]);
              setMessagesTotal(0);
              setMessagesPage(1);
              setLoading(false);
              setError(null);
            }}
          >
            ‹ Contacts
          </button>
        ) : (
          "Contacts"
        )}
        {!selectedContact && contactsTotal > 0 ? (
          <span className="count">{contactsTotal}</span>
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
                  <button type="button" className="row" onClick={() => selectContact(contact)}>
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
        {!selectedContact && contacts.length < contactsTotal ? (
          <button
            type="button"
            className="load-more"
            disabled={loading}
            onClick={() => void loadContacts(contactsPage + 1, true)}
          >
            {loading ? "Loading…" : "Load more contacts"}
          </button>
        ) : null}
        {selectedContact && messages.length < messagesTotal ? (
          <button
            type="button"
            className="load-more"
            disabled={loading}
            onClick={() =>
              void loadContactMessages(selectedContact, messagesPage + 1, true)
            }
          >
            {loading ? "Loading…" : "Load more messages"}
          </button>
        ) : null}
      </div>
    </section>
  );
}
