import type { CoinStat, FollowRow } from "../api";
import { hold, money, moneyCompact, pct, pnlClass, shortMint } from "../format";

type Props = {
  coins: CoinStat[];
  unit?: "USD" | "SOL";
  liveByMint?: Record<string, FollowRow>;
  /** Size per entry in SOL — used for live unrealized PnL. */
  notionalSol?: number;
};

export function CoinTable({ coins, unit = "SOL", liveByMint, notionalSol = 0.05 }: Props) {
  if (!coins.length) {
    return (
      <div className="empty">
        Chưa có trade — cần signal <code>pump</code> (hoặc kind đã chọn) và đủ free
        bankroll; nếu bật enrich thì phải pass filter liq/vol.
      </div>
    );
  }

  const pnlFmt = (n: number) =>
    unit === "SOL" ? `${n < 0 ? "-" : ""}${Math.abs(n).toFixed(4)} SOL` : money(n);

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Coin</th>
            <th>Entry → Exit</th>
            <th>Vol / Rug</th>
            <th>Return</th>
            <th>PnL</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {coins.map((c) => {
            const live = c.open && liveByMint ? liveByMint[c.mint] : undefined;
            const ret = live?.liveMcap ? live.returnPct : c.returnPct;
            // Open + live: unrealized = size × remaining × live return.
            // Closed: realized pnl from backtest legs.
            const rem = c.remaining && c.remaining > 0 ? c.remaining : 1;
            const livePnl =
              live?.liveMcap != null && Number.isFinite(ret)
                ? notionalSol * rem * (ret / 100)
                : null;
            const pnl = livePnl != null ? livePnl : c.pnlUsd;
            const cls = pnlClass(pnl);
            const rugScore = live?.rugScore ?? c.rugScore ?? 0;
            const rugCls = rugScore >= 45 ? "down" : rugScore >= 25 ? "" : "up";
            const vol = live?.volumeUsd24h || c.volumeUsd24h;
            return (
              <tr key={`${c.mint}-${c.entryKind}-${c.exitReason}-${c.holdSec}-${c.trades}`}>
                <td>
                  <strong>{c.symbol || live?.symbol || "?"}</strong>
                  <div className="mint" title={c.mint}>
                    {shortMint(c.mint, 6, 6)}
                  </div>
                </td>
                <td className="mono">
                  {c.entryKind} ${Math.round(c.entryMcap).toLocaleString()}
                  <br />
                  {live?.liveMcap ? (
                    <>
                      → live ${Math.round(live.liveMcap).toLocaleString()}
                      <div className="mint live-tag">realtime</div>
                    </>
                  ) : (
                    <>
                      → {c.exitReason} ${Math.round(c.exitMcap).toLocaleString()}
                      <div className="mint">{hold(c.holdSec)}</div>
                    </>
                  )}
                </td>
                <td className="mono">
                  {vol ? moneyCompact(vol) : "—"}
                  <div className={`mint ${rugCls}`}>
                    {(live?.rugLabel || c.rugLabel)
                      ? `${live?.rugLabel || c.rugLabel} ${Math.round(rugScore)}`
                      : "—"}
                    {(live?.sellRatio1h ?? c.sellRatio1h) != null
                      ? ` · sell ${(((live?.sellRatio1h ?? c.sellRatio1h) || 0) * 100).toFixed(0)}%`
                      : ""}
                  </div>
                </td>
                <td className={`mono ${cls}`}>{pct(ret)}</td>
                <td className={`mono ${cls}`}>
                  {pnlFmt(pnl)}
                  {livePnl != null ? <div className="mint live-tag">unrealized</div> : null}
                </td>
                <td>
                  <span className={`pill ${c.open ? "pill-live" : ""}`}>
                    {c.open ? (live?.liveMcap ? "open/live" : "open/mtm") : "closed"}
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
