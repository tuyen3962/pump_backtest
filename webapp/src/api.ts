export type Source = {
  id: string;
  label: string;
  path: string;
};

export type EquityPoint = {
  time: string;
  equity: number;
  realizedPnl: number;
  unrealizedPnl: number;
  openPositions: number;
  event: string;
  symbol?: string;
};

export type CoinStat = {
  mint: string;
  symbol: string;
  trades: number;
  open: boolean;
  pnlUsd: number;
  returnPct: number;
  entryMcap: number;
  exitMcap: number;
  entryKind: string;
  exitReason: string;
  holdSec: number;
  volumeUsd24h?: number;
  volumeUsd1h?: number;
  liquidityUsd?: number;
  sellRatio1h?: number;
  rugScore?: number;
  rugLabel?: string;
  marketCapUsd?: number;
  athDrawdownPct?: number;
  complete?: boolean;
};

export type BacktestResult = {
  signals: number;
  trades: unknown[];
  coins: CoinStat[];
  openCount: number;
  closedCount: number;
  wins: number;
  losses: number;
  totalPnl: number;
  totalFees: number;
  winRate: number;
  avgReturn: number;
  endEquity: number;
  maxEquity: number;
  minEquity: number;
  maxDrawdownPct: number;
  skippedEntries?: number;
  config: {
    startCash: number;
    notionalUsd: number;
    feeBps: number;
    entryKinds: string[];
    latencySec?: number;
    stopLossPct?: number;
    minLiquidityUsd?: number;
    minVolumeUsd1h?: number;
  };
};

export type BacktestResponse = {
  source: string;
  loaded: number;
  result: BacktestResult;
  equity: EquityPoint[];
  updated: string;
};

export type TokenInfo = {
  mint: string;
  symbol?: string;
  name?: string;
  creator?: string;
  complete?: boolean;
  isBanned?: boolean;
  marketCapUsd?: number;
  athMarketCapUsd?: number;
  athDrawdownPct?: number;
  volumeUsd5m?: number;
  volumeUsd1h?: number;
  volumeUsd6h?: number;
  volumeUsd24h?: number;
  liquidityUsd?: number;
  priceUsd?: number;
  dexId?: string;
  buys1h?: number;
  sells1h?: number;
  buys24h?: number;
  sells24h?: number;
  sellRatio1h?: number;
  sellRatio24h?: number;
  rugScore?: number;
  rugLabel?: string;
  riskNotes?: string[];
  sources?: string[];
  errors?: string[];
  fetchedAt?: string;
};

export type BacktestRequest = {
  source: string;
  entryKinds: string[];
  startCash: number;
  notionalUsd: number;
  feeBps: number;
  exitMustOut?: boolean;
  exitWatchOut?: boolean;
  maxPositions: number;
  closeOpenAtEnd: boolean;
  enrichTokens: boolean;
  sampleLive: boolean;
  alsoExitMustOut?: boolean;
  minLiquidityUsd?: number;
  minVolumeUsd1h?: number;
  latencySec?: number;
  stopLossPct?: number;
  scaleTriggerPct?: number;
  disableFilters?: boolean;
};

export type HistoryEntry = {
  id: string;
  mint: string;
  symbol: string;
  status: "closed" | "rugged" | "open_saved" | string;
  exitReason: string;
  entryKind: string;
  entryMcap: number;
  exitMcap: number;
  returnPct: number;
  pnlSol: number;
  rugScore?: number;
  rugLabel?: string;
  holdSec: number;
  closedAt: string;
  runId?: string;
};

export type FollowRow = {
  mint: string;
  symbol: string;
  entryMcap: number;
  liveMcap: number;
  returnPct: number;
  volumeUsd1h: number;
  volumeUsd24h: number;
  liquidityUsd: number;
  sellRatio1h: number;
  rugScore: number;
  rugLabel: string;
  athDrawdownPct: number;
  source: string;
  error?: string;
};

async function readError(res: Response): Promise<string> {
  const text = await res.text();
  return text || res.statusText;
}

export async function fetchSources(): Promise<Source[]> {
  const res = await fetch("/api/sources");
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function fetchToken(mint: string, live = false): Promise<TokenInfo> {
  const q = new URLSearchParams({ mint });
  if (live) q.set("live", "1");
  const res = await fetch(`/api/token?${q}`);
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function runBacktest(body: BacktestRequest): Promise<BacktestResponse> {
  const res = await fetch("/api/backtest", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function fetchLastBacktest(): Promise<{
  found: boolean;
  id?: string;
  savedAt?: string;
  run?: BacktestResponse;
}> {
  const res = await fetch("/api/backtest");
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function fetchHistory(status?: string): Promise<{ items: HistoryEntry[]; count: number }> {
  const q = status ? `?status=${encodeURIComponent(status)}` : "";
  const res = await fetch(`/api/history${q}`);
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function addWatch(mint: string, symbol?: string, entryMcap?: number) {
  const res = await fetch("/api/watchlist", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ mint, symbol, entryMcap, source: "manual" }),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function removeWatch(mint: string) {
  const res = await fetch(`/api/watchlist?mint=${encodeURIComponent(mint)}`, { method: "DELETE" });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function fetchFollow(): Promise<{ items: FollowRow[]; updatedAt: string }> {
  const res = await fetch("/api/follow");
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}
