import { useState } from "react";

export function AliasFilter({
  aliases,
  value,
  onChange,
}: {
  aliases: Array<{ email: string; count: number }>;
  value: string;
  onChange: (email: string) => void;
}) {
  const [open, setOpen] = useState(false);

  if (aliases.length === 0) return null;

  return (
    <section className="alias-filter" aria-label="Filter threads by receiving address">
      <button
        type="button"
        className="alias-filter-toggle"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        <span className="alias-filter-title">Received as</span>
        <span className="alias-filter-summary">
          {open ? "Hide" : value || "All"}
        </span>
      </button>
      {open ? (
        <div className="alias-filter-chips">
          <button
            type="button"
            className={value === "" ? "chip active" : "chip"}
            aria-pressed={value === ""}
            onClick={() => onChange("")}
          >
            All mail
          </button>
          {aliases.map((alias) => (
            <button
              type="button"
              className={value === alias.email ? "chip active" : "chip"}
              aria-pressed={value === alias.email}
              key={alias.email}
              onClick={() => onChange(alias.email)}
            >
              <span>{alias.email}</span>
              <span className="chip-count">{alias.count}</span>
            </button>
          ))}
        </div>
      ) : null}
    </section>
  );
}
