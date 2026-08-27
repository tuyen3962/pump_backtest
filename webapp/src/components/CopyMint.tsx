import { useState, type MouseEvent } from "react";

type Props = {
  mint: string;
  className?: string;
};

export function CopyMint({ mint, className }: Props) {
  const [ok, setOk] = useState(false);
  if (!mint) return null;

  async function copy(e: MouseEvent) {
    e.preventDefault();
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(mint);
      setOk(true);
      window.setTimeout(() => setOk(false), 1200);
    } catch {
      /* ignore */
    }
  }

  return (
    <button
      type="button"
      className={`copy-mint ${className || ""}`}
      title={ok ? "Đã copy" : "Copy contract"}
      aria-label="Copy contract"
      onClick={(e) => void copy(e)}
    >
      {ok ? (
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden>
          <path
            d="M5 13l4 4L19 7"
            stroke="currentColor"
            strokeWidth="2.2"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      ) : (
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden>
          <rect x="9" y="9" width="11" height="11" rx="2" stroke="currentColor" strokeWidth="2" />
          <path
            d="M5 15V5a2 2 0 0 1 2-2h10"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
          />
        </svg>
      )}
    </button>
  );
}
