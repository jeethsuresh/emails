import { useEffect, useState } from "react";
import type PocketBase from "pocketbase";
import { listAliases } from "../lib/mailApi";

export function AliasFilter({
  pb,
  value,
  onChange,
}: {
  pb: PocketBase;
  value: string;
  onChange: (email: string) => void;
}) {
  const [aliases, setAliases] = useState<Array<{ email: string; count: number }>>([]);

  useEffect(() => {
    let cancelled = false;
    void listAliases(pb)
      .then((rows) => {
        if (!cancelled) setAliases(rows);
      })
      .catch((err: unknown) => console.error("AliasFilter listAliases failed", err));
    return () => {
      cancelled = true;
    };
  }, [pb]);

  if (aliases.length === 0) return null;

  return (
    <div className="alias-filter" aria-label="Filter threads by receiving address">
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
  );
}
