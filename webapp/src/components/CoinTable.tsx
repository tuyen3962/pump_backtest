import type { CoinStat } from "../api";
import { hold, money, moneyCompact, pct, pnlClass, shortMint } from "../format";

type Props = {
  coins: CoinStat[];
  unit?: "USD" | "SOL";
};

export function CoinTable({ coins, unit = "SOL" }: Props) {
  if (!coins.length) {
    return (
      <div className="empty">
        Chưa có trade — cần `whale_armed` (và pass filter liq/vol nếu đang bật).
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
            const cls = pnlClass(c.pnlUsd);
            const rugCls = (c.rugScore || 0) >= 45 ? "down" : (c.rugScore || 0) >= 25 ? "" : "up";
            return (
              <tr key={`${c.mint}-${c.entryKind}-${c.exitReason}-${c.holdSec}-${c.trades}`}>
                <td>
                  <strong>{c.symbol || "?"}</strong>
                  <div className="mint" title={c.mint}>
                    {shortMint(c.mint, 6, 6)}
                  </div>
                </td>
                <td className="mono">
                  {c.entryKind} ${Math.round(c.entryMcap).toLocaleString()}
                  <br />
                  → {c.exitReason} ${Math.round(c.exitMcap).toLocaleString()}
                  <div className="mint">{hold(c.holdSec)}</div>
                </td>
                <td className="mono">
                  {c.volumeUsd24h ? moneyCompact(c.volumeUsd24h) : "—"}
                  <div className={`mint ${rugCls}`}>
                    {c.rugLabel ? `${c.rugLabel} ${Math.round(c.rugScore || 0)}` : "—"}
                    {c.sellRatio1h != null ? ` · sell ${(c.sellRatio1h * 100).toFixed(0)}%` : ""}
                  </div>
                </td>
                <td className={`mono ${cls}`}>{pct(c.returnPct)}</td>
                <td className={`mono ${cls}`}>{pnlFmt(c.pnlUsd)}</td>
                <td>
                  <span className="pill">{c.open ? "open/mtm" : "closed"}</span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
