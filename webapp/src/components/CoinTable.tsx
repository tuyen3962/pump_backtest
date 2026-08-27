import type { CoinStat, FollowRow } from "../api";
import { hold, money, moneyCompact, pct, pnlClass, shortMint } from "../format";
import { CopyMint } from "./CopyMint";

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
            <th>Size</th>
            <th>Vol / Rug</th>
            <th>Return</th>
            <th>PnL</th>
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {coins.map((c) => {
            const live = c.open && liveByMint ? liveByMint[c.mint] : undefined;
            const ret = live?.liveMcap != null ? live.returnPct : c.returnPct;
            // remaining=1 full; after TP 2x half → 0.5, etc.
            const rem =
              c.open && c.remaining != null && c.remaining > 0 && c.remaining <= 1
                ? c.remaining
                : c.open
                  ? 1
                  : 0;
            const openSize = notionalSol * rem;
            const entrySize = notionalSol;
            const realized = c.pnlUsd || 0;
            const unreal =
              c.open && Number.isFinite(ret)
                ? openSize * (ret / 100)
                : 0;
            // Open: realized (partial TP/scale) + unrealized on size còn lại.
            // Closed: realized only.
            const pnl = c.open ? realized + unreal : realized;
            const cls = pnlClass(pnl);
            const rugScore = live?.rugScore ?? c.rugScore ?? 0;
            const rugCls = rugScore >= 45 ? "down" : rugScore >= 25 ? "" : "up";
            const vol = live?.volumeUsd24h || c.volumeUsd24h;
            return (
              <tr key={`${c.mint}-${c.entryKind}-${c.exitReason}-${c.holdSec}-${c.trades}`}>
                <td>
                  <div className="coin-cell">
                    <strong>{c.symbol || live?.symbol || "?"}</strong>
                    <CopyMint mint={c.mint} />
                  </div>
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
                  {c.open ? (
                    <>
                      {openSize.toFixed(3)} SOL
                      {rem < 0.999 ? (
                        <div className="mint">
                          còn {(rem * 100).toFixed(0)}% · entry {entrySize.toFixed(3)}
                        </div>
                      ) : (
                        <div className="mint">full entry</div>
                      )}
                    </>
                  ) : (
                    <>
                      {entrySize.toFixed(3)} SOL
                      <div className="mint">closed</div>
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
                  {c.open ? (
                    <div className="mint">
                      {Math.abs(realized) > 1e-9
                        ? `real ${pnlFmt(realized)} + unrel ${pnlFmt(unreal)}`
                        : "unrealized"}
                    </div>
                  ) : null}
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
