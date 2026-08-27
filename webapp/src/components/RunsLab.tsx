import { useCallback, useEffect, useMemo, useState } from "react";
import {
  compareRuns,
  enqueueBacktest,
  exportRunCsvUrl,
  fetchJobs,
  fetchRun,
  fetchRuns,
  type BacktestRequest,
  type CompareRow,
  type JobStatus,
  type RunSummary,
} from "../api";
import { pnlClass } from "../format";

type Props = {
  draft: BacktestRequest | null;
  onLoadRun?: (runId: string) => void;
  refreshKey?: number;
};

function sol(n: number | undefined | null, digits = 4): string {
  const v = Number(n ?? 0);
  const sign = v < 0 ? "-" : "";
  return `${sign}${Math.abs(v).toFixed(digits)} SOL`;
}

export function RunsLab({ draft, onLoadRun, refreshKey = 0 }: Props) {
  const [runs, setRuns] = useState<RunSummary[]>([]);
  const [jobs, setJobs] = useState<JobStatus[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [compare, setCompare] = useState<CompareRow[]>([]);
  const [label, setLabel] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    try {
      const [r, j] = await Promise.all([fetchRuns(), fetchJobs()]);
      setRuns(r.items || []);
      setJobs(j.items || []);
    } catch (err) {
      setError(String((err as Error).message || err));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh, refreshKey]);

  // Poll active jobs
  useEffect(() => {
    const active = jobs.some((j) => j.status === "queued" || j.status === "running");
    if (!active) return;
    const t = setInterval(() => {
      void (async () => {
        try {
          const j = await fetchJobs();
          setJobs(j.items || []);
          const done = (j.items || []).filter((x) => x.status === "done");
          if (done.length) await refresh();
        } catch {
          /* ignore poll errors */
        }
      })();
    }, 2000);
    return () => clearInterval(t);
  }, [jobs, refresh]);

  const toggle = (id: string) => {
    setSelected((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id].slice(-6)));
  };

  async function queueFromDraft() {
    if (!draft) {
      setError("Chưa có cấu hình từ Strategy panel — chỉnh bên trái rồi queue.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const name = label.trim() || `variant ${new Date().toLocaleTimeString()}`;
      await enqueueBacktest({ ...draft, label: name, async: true, updateWatchlist: false });
      setLabel("");
      await refresh();
    } catch (err) {
      setError(String((err as Error).message || err));
    } finally {
      setBusy(false);
    }
  }

  async function doCompare() {
    if (selected.length < 2) {
      setError("Chọn ≥2 run để so sánh");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const data = await compareRuns(selected);
      setCompare(data.items || []);
    } catch (err) {
      setError(String((err as Error).message || err));
    } finally {
      setBusy(false);
    }
  }

  async function openRun(id: string) {
    try {
      await fetchRun(id);
      onLoadRun?.(id);
    } catch (err) {
      setError(String((err as Error).message || err));
    }
  }

  const best = useMemo(() => {
    if (!compare.length) return null;
    const byPnl = [...compare].sort((a, b) => b.totalPnl - a.totalPnl)[0];
    const byDD = [...compare].sort((a, b) => a.maxDrawdownPct - b.maxDrawdownPct)[0];
    const byWR = [...compare].sort((a, b) => b.winRate - a.winRate)[0];
    return { byPnl, byDD, byWR };
  }, [compare]);

  return (
    <section className="panel runs-lab">
      <div className="panel-head">
        <h2>Lab: multi-run & CSV</h2>
        <button type="button" className="linkish" onClick={() => void refresh()}>
          Refresh
        </button>
      </div>
      <p className="panel-note">
        Queue nhiều config chạy song song (tối đa 3 worker). So sánh PnL / winrate / max DD, rồi export CSV
        để optimize offline.
      </p>

      <div className="runs-queue">
        <label htmlFor="runLabel">Label run</label>
        <div className="runs-queue-row">
          <input
            id="runLabel"
            value={label}
            placeholder="vd: pump SL40 TP2.5"
            onChange={(e) => setLabel(e.target.value)}
          />
          <button type="button" className="run" disabled={busy || !draft} onClick={() => void queueFromDraft()}>
            {busy ? "…" : "Queue async"}
          </button>
        </div>
      </div>

      {jobs.some((j) => j.status === "queued" || j.status === "running") ? (
        <div className="jobs-live">
          {jobs
            .filter((j) => j.status === "queued" || j.status === "running" || j.status === "failed")
            .slice(0, 8)
            .map((j) => (
              <div key={j.id} className="job-row">
                <span className={`pill ${j.status === "failed" ? "down" : ""}`}>{j.status}</span>
                <strong>{j.label || j.id}</strong>
                <span className="muted">{j.progress || j.error || j.runId || ""}</span>
              </div>
            ))}
        </div>
      ) : null}

      {error ? <div className="err">{error}</div> : null}

      <div className="panel-head" style={{ marginTop: 14 }}>
        <h3 style={{ margin: 0, fontSize: "1rem" }}>Saved runs</h3>
        <button type="button" className="run" disabled={selected.length < 2 || busy} onClick={() => void doCompare()}>
          So sánh ({selected.length})
        </button>
      </div>

      {!runs.length ? (
        <div className="empty">Chưa có run lưu — queue hoặc chạy backtest sync.</div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th></th>
                <th>Label</th>
                <th>PnL</th>
                <th>WR</th>
                <th>MaxDD</th>
                <th>Coins</th>
                <th>CSV</th>
              </tr>
            </thead>
            <tbody>
              {runs.slice(0, 40).map((r) => (
                <tr key={r.id}>
                  <td>
                    <input type="checkbox" checked={selected.includes(r.id)} onChange={() => toggle(r.id)} />
                  </td>
                  <td>
                    <button type="button" className="linkish" onClick={() => void openRun(r.id)}>
                      {r.label || r.id}
                    </button>
                    <div className="mint">
                      {(r.entryKinds || []).join(",") || "—"} · {r.savedAt ? new Date(r.savedAt).toLocaleString() : ""}
                    </div>
                  </td>
                  <td className={`mono ${pnlClass(r.totalPnl)}`}>{sol(r.totalPnl)}</td>
                  <td className="mono">{(r.winRate || 0).toFixed(1)}%</td>
                  <td className="mono">{(r.maxDrawdownPct || 0).toFixed(1)}%</td>
                  <td className="mono">
                    {r.closedCount}/{r.coinCount}
                  </td>
                  <td className="mono csv-links">
                    <a href={exportRunCsvUrl(r.id, "trades")}>trades</a>
                    {" · "}
                    <a href={exportRunCsvUrl(r.id, "coins")}>coins</a>
                    {" · "}
                    <a href={exportRunCsvUrl(r.id, "equity")}>equity</a>
                    {" · "}
                    <a href={exportRunCsvUrl(r.id, "summary")}>summary</a>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {compare.length ? (
        <>
          <div className="panel-head" style={{ marginTop: 16 }}>
            <h3 style={{ margin: 0, fontSize: "1rem" }}>Compare</h3>
          </div>
          {best ? (
            <p className="panel-note">
              Cao PnL: <strong>{best.byPnl.label || best.byPnl.id}</strong> ({sol(best.byPnl.totalPnl)}) · Thấp DD:{" "}
              <strong>{best.byDD.label || best.byDD.id}</strong> ({best.byDD.maxDrawdownPct.toFixed(1)}%) · Cao WR:{" "}
              <strong>{best.byWR.label || best.byWR.id}</strong> ({best.byWR.winRate.toFixed(1)}%)
            </p>
          ) : null}
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Label</th>
                  <th>Entry</th>
                  <th>PnL</th>
                  <th>End</th>
                  <th>WR</th>
                  <th>AvgRet</th>
                  <th>MaxDD</th>
                  <th>SL/TP</th>
                  <th>Fee/Lat</th>
                </tr>
              </thead>
              <tbody>
                {compare.map((c) => (
                  <tr key={c.id}>
                    <td>
                      <strong>{c.label || c.id}</strong>
                      <div className="mint">{c.id}</div>
                    </td>
                    <td className="mono">{(c.entryKinds || []).join(",")}</td>
                    <td className={`mono ${pnlClass(c.totalPnl)}`}>{sol(c.totalPnl)}</td>
                    <td className="mono">{sol(c.endEquity)}</td>
                    <td className="mono">{c.winRate.toFixed(1)}%</td>
                    <td className={`mono ${pnlClass(c.avgReturn)}`}>{c.avgReturn.toFixed(1)}%</td>
                    <td className="mono">{c.maxDrawdownPct.toFixed(1)}%</td>
                    <td className="mono">
                      {c.stopLossPct}/{c.takeProfit2x}x
                    </td>
                    <td className="mono">
                      {c.feeBps}bps/{c.latencySec}s
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : null}
    </section>
  );
}
