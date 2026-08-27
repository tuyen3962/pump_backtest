import { useCallback, useEffect, useState } from "react";
import { fetchHistory, type HistoryEntry } from "../api";
import { hold, pct, pnlClass, shortMint } from "../format";

export function HistoryPanel() {
  const [items, setItems] = useState<HistoryEntry[]>([]);
  const [filter, setFilter] = useState<"" | "closed" | "rugged">("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await fetchHistory(filter || undefined);
      setItems(data.items || []);
    } catch (err) {
      setError(String((err as Error).message || err));
    } finally {
      setLoading(false);
    }
  }, [filter]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return (
    <section className="panel">
      <div className="panel-head">
        <h2>Lịch sử đóng / rug</h2>
        <span className="muted">
          {loading ? "…" : `${items.length} records`}
          {" · "}
          <button type="button" className="linkish" onClick={() => void refresh()} disabled={loading}>
            Refresh
          </button>
        </span>
      </div>
      <div className="checks" style={{ gridTemplateColumns: "repeat(3, auto)", marginBottom: 10 }}>
        <label>
          <input type="radio" checked={filter === ""} onChange={() => setFilter("")} /> All
        </label>
        <label>
          <input type="radio" checked={filter === "closed"} onChange={() => setFilter("closed")} /> Closed
        </label>
        <label>
          <input type="radio" checked={filter === "rugged"} onChange={() => setFilter("rugged")} /> Rugged
        </label>
      </div>
      {error ? <div className="err">{error}</div> : null}
      {!items.length ? (
        <div className="empty">
          Chưa có lịch sử đóng. Sau backtest, mỗi exit leg (stop / TP / out / eod_mark) được ghi
          vào đây. Lệnh vẫn <strong>open</strong> chỉ nằm ở Follow — bật “Đóng / MTM hết lệnh mở”
          nếu muốn chốt toàn bộ vào lịch sử khi hết data.
        </div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>Coin</th>
                <th>Status</th>
                <th>Exit</th>
                <th>Return</th>
                <th>PnL</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {items.map((c) => {
                const cls = pnlClass(c.returnPct);
                const statusCls = c.status === "rugged" ? "down" : "";
                return (
                  <tr key={c.id || `${c.mint}-${c.closedAt}-${c.exitReason}-${c.holdSec}`}>
                    <td>
                      <strong>{c.symbol || "?"}</strong>
                      <div className="mint">{shortMint(c.mint, 6, 6)}</div>
                    </td>
                    <td>
                      <span className={`pill ${statusCls}`}>{c.status}</span>
                      {c.rugLabel ? <div className="mint">{c.rugLabel}</div> : null}
                    </td>
                    <td className="mono">
                      {c.exitReason}
                      <div className="mint">{hold(c.holdSec)}</div>
                    </td>
                    <td className={`mono ${cls}`}>{pct(c.returnPct)}</td>
                    <td className={`mono ${cls}`}>
                      {c.pnlSol < 0 ? "-" : ""}
                      {Math.abs(c.pnlSol ?? 0).toFixed(4)} SOL
                    </td>
                    <td className="mono">{c.closedAt ? new Date(c.closedAt).toLocaleString() : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
