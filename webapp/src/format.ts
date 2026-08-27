export function money(n: number | undefined | null, digits = 2): string {
  const v = Number(n ?? 0);
  const sign = v < 0 ? "-" : "";
  return `${sign}$${Math.abs(v).toLocaleString(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  })}`;
}

export function moneyCompact(n: number | undefined | null): string {
  const v = Math.abs(Number(n ?? 0));
  const sign = Number(n ?? 0) < 0 ? "-" : "";
  if (v >= 1_000_000) return `${sign}$${(v / 1_000_000).toFixed(2)}M`;
  if (v >= 1_000) return `${sign}$${(v / 1_000).toFixed(1)}k`;
  return `${sign}$${v.toFixed(0)}`;
}

export function pct(n: number | undefined | null, digits = 2): string {
  const v = Number(n ?? 0);
  return `${v >= 0 ? "+" : ""}${v.toFixed(digits)}%`;
}

export function hold(sec: number | undefined | null): string {
  const s = Number(sec ?? 0);
  if (s < 60) return `${Math.round(s)}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m${Math.round(s % 60)}s`;
  return `${Math.floor(s / 3600)}h${Math.floor((s % 3600) / 60)}m`;
}

export function pnlClass(n: number | undefined | null): string {
  const v = Number(n ?? 0);
  if (v > 0) return "up";
  if (v < 0) return "down";
  return "";
}

export function shortMint(mint: string, head = 4, tail = 4): string {
  if (!mint || mint.length <= head + tail + 1) return mint || "—";
  return `${mint.slice(0, head)}…${mint.slice(-tail)}`;
}
