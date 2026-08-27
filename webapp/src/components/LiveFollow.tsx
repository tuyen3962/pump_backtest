import { removeWatch, type FollowRow } from "../api";
import { moneyCompact, pct, pnlClass, shortMint } from "../format";
import type { FollowStreamState } from "../hooks/useFollowStream";
import { CopyMint } from "./CopyMint";

type Props = {
  stream: FollowStreamState;
};

export function LiveFollow({ stream }: Props) {
  const { items, updatedAt, connected, error, refresh } = stream;

  async function onRemove(mint: string) {
    try {
      await removeWatch(mint);
      refresh();
    } catch (err) {
      console.error(err);
    }
  }

  return (
    <section className="panel">
      <div className="panel-head">
        <h2>Follow realtime</h2>
        <span className="muted">
          {connected ? "live SSE · 5s" : "poll 15s"}
          {updatedAt ? ` · ${new Date(updatedAt).toLocaleTimeString()}` : ""}
        </span>
      </div>
      <p className="panel-note">
        Coin đóng bằng eod_mark vẫn có thể nằm Follow để xem mcap/vol/rug khi dashboard mở.
      </p>
      {error ? <div className="err">{error}</div> : null}
      {!items.length ? (
        <div className="empty">
          Chưa có token follow — chạy backtest (giữ open / eod) hoặc Follow realtime ở Check contract.
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Coin</th>
                <th>Entry → Live</th>
                <th>Return</th>
                <th>Vol / Rug</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {items.map((c: FollowRow) => {
                const cls = pnlClass(c.returnPct);
                const rugCls = c.rugScore >= 45 ? "down" : "";
                return (
                  <tr key={c.mint}>
                    <td>
                      <div className="coin-cell">
                        <strong>{c.symbol || "?"}</strong>
                        <CopyMint mint={c.mint} />
                      </div>
                      <div className="mint" title={c.mint}>
                        {shortMint(c.mint, 6, 6)}
                      </div>
                      <div className="mint">{c.source}</div>
                    </td>
                    <td className="mono">
                      {c.entryMcap ? `$${Math.round(c.entryMcap).toLocaleString()}` : "—"}
                      <br />→ {c.liveMcap ? `$${Math.round(c.liveMcap).toLocaleString()}` : "—"}
                      {c.error ? <div className="err">{c.error}</div> : null}
                    </td>
                    <td className={`mono ${cls}`}>{pct(c.returnPct)}</td>
                    <td className="mono">
                      {c.volumeUsd1h ? moneyCompact(c.volumeUsd1h) : "—"} /1h
                      <div className={`mint ${rugCls}`}>
                        {c.rugLabel || "—"} {c.rugScore ? Math.round(c.rugScore) : ""}
                        {c.sellRatio1h ? ` · sell ${(c.sellRatio1h * 100).toFixed(0)}%` : ""}
                      </div>
                    </td>
                    <td>
                      <button type="button" className="linkish" onClick={() => void onRemove(c.mint)}>
                        Bỏ
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      <button type="button" className="run" style={{ marginTop: 12 }} onClick={() => refresh()}>
        Reconnect live
      </button>
    </section>
  );
}
