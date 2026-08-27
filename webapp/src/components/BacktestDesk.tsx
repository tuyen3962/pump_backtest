import { useEffect, useMemo, useState } from "react";
import {
  enqueueBacktest,
  fetchJob,
  fetchLastBacktest,
  fetchRun,
  fetchSources,
  runBacktest,
  type BacktestRequest,
  type BacktestResponse,
  type FollowRow,
  type Source,
} from "../api";
import { pnlClass } from "../format";
import { CoinTable } from "./CoinTable";
import { EquityChart } from "./EquityChart";

const ENTRY_OPTIONS = ["pump", "whale_armed", "arm", "whale", "milestone"] as const;

function sol(n: number | undefined | null, digits = 4): string {
  const v = Number(n ?? 0);
  const sign = v < 0 ? "-" : "";
  return `${sign}${Math.abs(v).toFixed(digits)} SOL`;
}

/** datetime-local value in local TZ */
function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

type Props = {
  onRunComplete?: () => void;
  onDraftChange?: (draft: BacktestRequest) => void;
  liveByMint?: Record<string, FollowRow>;
  loadRunId?: string | null;
};

export function BacktestDesk({ onRunComplete, onDraftChange, liveByMint, loadRunId }: Props) {
  const [sources, setSources] = useState<Source[]>([]);
  const [source, setSource] = useState("live");
  const [runLabel, setRunLabel] = useState("");
  const [entryKinds, setEntryKinds] = useState<string[]>(["pump"]);
  const [startCash, setStartCash] = useState(1);
  const [notional, setNotional] = useState(0.05);
  const [feeBps, setFeeBps] = useState(100);
  const [latency, setLatency] = useState(5);
  const [minLiq, setMinLiq] = useState(5000);
  const [minVol1h, setMinVol1h] = useState(2000);
  const [maxPos, setMaxPos] = useState(0);
  const [closeEod, setCloseEod] = useState(false);
  const [stopLoss, setStopLoss] = useState(60);
  const [takeProfit, setTakeProfit] = useState(2);
  const [enrich, setEnrich] = useState(true);
  const [sampleLive, setSampleLive] = useState(false);
  const [disableFilters, setDisableFilters] = useState(false);
  const [fromTime, setFromTime] = useState("");
  const [toTime, setToTime] = useState("");
  const [sessionEndAt, setSessionEndAt] = useState("");
  const [sessionRefreshSec, setSessionRefreshSec] = useState(60);
  const [sessionJobId, setSessionJobId] = useState<string | null>(null);
  const [sessionProgress, setSessionProgress] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [savedAt, setSavedAt] = useState("");
  const [data, setData] = useState<BacktestResponse | null>(null);

  const draft: BacktestRequest = useMemo(
    () => ({
      source,
      label: runLabel.trim() || undefined,
      entryKinds,
      startCash,
      notionalUsd: notional,
      feeBps,
      maxPositions: maxPos,
      closeOpenAtEnd: closeEod,
      enrichTokens: enrich,
      sampleLive,
      alsoExitMustOut: true,
      minLiquidityUsd: minLiq,
      minVolumeUsd1h: minVol1h,
      latencySec: latency,
      stopLossPct: stopLoss,
      scaleTriggerPct: 15,
      takeProfit2x: takeProfit,
      disableFilters,
      exitMustOut: true,
      exitWatchOut: false,
      fromTime: fromTime || undefined,
      toTime: toTime || undefined,
      sessionEndAt: sessionEndAt || undefined,
      sessionRefreshSec: sessionRefreshSec || undefined,
    }),
    [
      source,
      runLabel,
      entryKinds,
      startCash,
      notional,
      feeBps,
      maxPos,
      closeEod,
      enrich,
      sampleLive,
      minLiq,
      minVol1h,
      latency,
      stopLoss,
      takeProfit,
      disableFilters,
      fromTime,
      toTime,
      sessionEndAt,
      sessionRefreshSec,
    ],
  );

  useEffect(() => {
    onDraftChange?.(draft);
  }, [draft, onDraftChange]);

  useEffect(() => {
    fetchSources()
      .then((list) => {
        setSources(list);
        if (list.some((s) => s.id === "live")) setSource("live");
        else if (list[0]) setSource(list[0].id);
      })
      .catch((err) => setError(String(err.message || err)));

    fetchLastBacktest()
      .then((last) => {
        if (last.found && last.run) {
          setData(last.run);
          setSavedAt(last.savedAt || last.run.updated || "");
          const cfg = last.run.result?.config;
          if (cfg?.entryKinds?.length) setEntryKinds(cfg.entryKinds);
          if (cfg?.startCash) setStartCash(cfg.startCash);
          if (cfg?.notionalUsd) setNotional(cfg.notionalUsd);
          if (cfg?.stopLossPct) setStopLoss(cfg.stopLossPct);
        }
      })
      .catch(() => {
        /* no prior run is fine */
      });
  }, []);

  useEffect(() => {
    if (!loadRunId) return;
    void (async () => {
      try {
        const packed = await fetchRun(loadRunId);
        setData(packed.run);
        setSavedAt(packed.savedAt || packed.run.updated || "");
        setRunLabel(packed.label || "");
        const cfg = packed.run.result?.config;
        if (cfg?.entryKinds?.length) setEntryKinds(cfg.entryKinds);
        if (cfg?.startCash) setStartCash(cfg.startCash);
        if (cfg?.notionalUsd) setNotional(cfg.notionalUsd);
        if (cfg?.feeBps != null) setFeeBps(cfg.feeBps);
        if (cfg?.latencySec != null) setLatency(cfg.latencySec);
        if (cfg?.stopLossPct) setStopLoss(cfg.stopLossPct);
      } catch (err) {
        setError(String((err as Error).message || err));
      }
    })();
  }, [loadRunId]);

  // Poll stability session job + refresh run snapshot
  useEffect(() => {
    if (!sessionJobId) return;
    const t = setInterval(() => {
      void (async () => {
        try {
          const job = await fetchJob(sessionJobId);
          setSessionProgress(job.progress || job.status);
          if (job.runId) {
            const packed = await fetchRun(job.runId);
            setData(packed.run);
            setSavedAt(packed.savedAt || packed.run.updated || "");
          }
          if (job.status === "done" || job.status === "failed" || job.status === "cancelled") {
            setSessionJobId(null);
            setLoading(false);
            if (job.status === "failed") setError(job.error || "session failed");
            else onRunComplete?.();
          }
        } catch (err) {
          setError(String((err as Error).message || err));
        }
      })();
    }, 3000);
    return () => clearInterval(t);
  }, [sessionJobId, onRunComplete]);

  const result = data?.result;
  const liveBook = useMemo(() => {
    if (!result) return null;
    const start = result.config?.startCash ?? 1;
    const notional = result.config?.notionalUsd ?? 0.05;
    const realized = result.totalPnl || 0;
    let openUnreal = 0;
    let liveMarks = 0;
    for (const c of result.coins || []) {
      if (!c.open) continue;
      const rem =
        c.remaining != null && c.remaining > 0 && c.remaining <= 1 ? c.remaining : 1;
      const live = liveByMint?.[c.mint];
      if (live?.liveMcap != null && c.entryMcap > 0) {
        openUnreal += notional * rem * ((live.liveMcap - c.entryMcap) / c.entryMcap);
        liveMarks++;
      } else {
        openUnreal += notional * rem * ((c.returnPct || 0) / 100);
      }
    }
    const equity = start + realized + openUnreal;
    return {
      start,
      realized,
      openUnreal,
      equity,
      pnl: equity - start,
      liveMarks,
    };
  }, [result, liveByMint]);

  const stats = useMemo(() => {
    if (!result || !liveBook) return [];
    return [
      { k: "Balance", v: sol(liveBook.equity), cls: pnlClass(liveBook.pnl) },
      { k: "Lợi nhuận", v: sol(liveBook.pnl), cls: pnlClass(liveBook.pnl) },
      { k: "Win rate", v: `${(result.winRate || 0).toFixed(1)}%`, cls: "" },
      {
        k: "Max DD",
        v: `${(result.maxDrawdownPct || 0).toFixed(2)}%`,
        cls: (result.maxDrawdownPct || 0) > 0 ? "down" : "",
      },
    ];
  }, [result, liveBook]);

  function toggleKind(kind: string) {
    setEntryKinds((prev) =>
      prev.includes(kind) ? prev.filter((k) => k !== kind) : [...prev, kind],
    );
  }

  async function run() {
    if (!entryKinds.length) {
      setError("Chọn ít nhất 1 entry kind");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const res = await runBacktest({
        ...draft,
        sessionEndAt: undefined,
        async: false,
        updateWatchlist: true,
      });
      setData(res);
      setSavedAt(res.updated || new Date().toISOString());
      onRunComplete?.();
    } catch (err) {
      setError(String((err as Error).message || err));
    } finally {
      setLoading(false);
    }
  }

  async function startSession() {
    if (!entryKinds.length) {
      setError("Chọn ít nhất 1 entry kind");
      return;
    }
    if (!sessionEndAt) {
      setError("Chọn giờ kết thúc session");
      return;
    }
    const end = new Date(sessionEndAt);
    if (Number.isNaN(end.getTime()) || end.getTime() <= Date.now()) {
      setError("Giờ kết thúc phải sau hiện tại");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const start = fromTime || toLocalInput(new Date());
      if (!fromTime) setFromTime(start);
      const { job } = await enqueueBacktest({
        ...draft,
        fromTime: start,
        toTime: undefined,
        sessionEndAt,
        sessionRefreshSec: sessionRefreshSec || 60,
        label: runLabel.trim() || `session → ${end.toLocaleTimeString()}`,
        updateWatchlist: true,
      });
      setSessionJobId(job.id);
      setSessionProgress(job.progress || job.status);
    } catch (err) {
      setError(String((err as Error).message || err));
      setLoading(false);
    }
  }

  return (
    <section className="backtest-block">
      <div className="layout">
        <aside className="panel sticky">
          <h2>Strategy v1</h2>
          <p className="panel-note">
            Entry <code>pump</code> (fire fill) · multi-hold theo bankroll · dùng Lab bên dưới để
            queue nhiều variant song song + CSV.
          </p>

          <label htmlFor="runLabel">Label (lưu run)</label>
          <input
            id="runLabel"
            value={runLabel}
            placeholder="vd: pump baseline"
            onChange={(e) => setRunLabel(e.target.value)}
          />

          <label htmlFor="source">Nguồn data</label>
          <select id="source" value={source} onChange={(e) => setSource(e.target.value)}>
            {sources.map((s) => (
              <option key={s.id} value={s.id}>
                {s.label}
              </option>
            ))}
          </select>

          <label>Entry kinds</label>
          <div className="checks">
            {ENTRY_OPTIONS.map((kind) => (
              <label key={kind}>
                <input
                  type="checkbox"
                  checked={entryKinds.includes(kind)}
                  onChange={() => toggleKind(kind)}
                />
                {kind}
              </label>
            ))}
          </div>

          <label htmlFor="fromTime">Từ lúc (signal window)</label>
          <input
            id="fromTime"
            type="datetime-local"
            value={fromTime}
            onChange={(e) => setFromTime(e.target.value)}
          />
          <label htmlFor="toTime">Đến lúc (để trống = hết file)</label>
          <input
            id="toTime"
            type="datetime-local"
            value={toTime}
            onChange={(e) => setToTime(e.target.value)}
          />
          <p className="panel-note" style={{ marginTop: 4 }}>
            Sync: cắt slice lịch sử theo cửa sổ. Session: bắt đầu từ lúc chạy → giờ kết thúc,
            reload tín hiệu định kỳ để theo dõi ổn định.
          </p>

          <label htmlFor="sessionEnd">Kết thúc session</label>
          <input
            id="sessionEnd"
            type="datetime-local"
            value={sessionEndAt}
            onChange={(e) => setSessionEndAt(e.target.value)}
          />
          <label htmlFor="sessionRefresh">Refresh session (giây)</label>
          <input
            id="sessionRefresh"
            type="number"
            value={sessionRefreshSec}
            min={30}
            step={30}
            onChange={(e) => setSessionRefreshSec(Number(e.target.value))}
          />

          <label htmlFor="startCash">Bankroll (SOL)</label>
          <input
            id="startCash"
            type="number"
            value={startCash}
            min={0.01}
            step={0.1}
            onChange={(e) => setStartCash(Number(e.target.value))}
          />

          <label htmlFor="notional">Size / lệnh (SOL)</label>
          <input
            id="notional"
            type="number"
            value={notional}
            min={0.001}
            step={0.01}
            onChange={(e) => setNotional(Number(e.target.value))}
          />

          <label htmlFor="stopLoss">Stop loss (%)</label>
          <input
            id="stopLoss"
            type="number"
            value={stopLoss}
            min={5}
            step={5}
            onChange={(e) => setStopLoss(Number(e.target.value))}
          />

          <label htmlFor="takeProfit">Take profit (×)</label>
          <input
            id="takeProfit"
            type="number"
            value={takeProfit}
            min={1.1}
            step={0.1}
            onChange={(e) => setTakeProfit(Number(e.target.value))}
          />

          <label htmlFor="latency">Latency entry (giây)</label>
          <input
            id="latency"
            type="number"
            value={latency}
            min={0}
            step={1}
            onChange={(e) => setLatency(Number(e.target.value))}
          />

          <label htmlFor="feeBps">Fee / slippage (bps)</label>
          <input
            id="feeBps"
            type="number"
            value={feeBps}
            min={0}
            step={10}
            onChange={(e) => setFeeBps(Number(e.target.value))}
          />

          <div className="checks" style={{ marginTop: 14 }}>
            <label>
              <input type="checkbox" checked={closeEod} onChange={(e) => setCloseEod(e.target.checked)} />
              Đóng / MTM hết lệnh mở khi hết data
            </label>
            <label>
              <input type="checkbox" checked={enrich} onChange={(e) => setEnrich(e.target.checked)} />
              Enrich + filter volume/liq
            </label>
            <label>
              <input
                type="checkbox"
                checked={disableFilters}
                onChange={(e) => setDisableFilters(e.target.checked)}
              />
              Tắt filter (chỉ test signal)
            </label>
            <label>
              <input
                type="checkbox"
                checked={sampleLive}
                onChange={(e) => setSampleLive(e.target.checked)}
              />
              Sample PumpDev WS
            </label>
          </div>

          <button className="run" onClick={() => void run()} disabled={loading || !!sessionJobId}>
            {loading && !sessionJobId ? "Đang chạy…" : "Chạy backtest (sync)"}
          </button>
          <button
            className="run"
            style={{ marginTop: 8 }}
            onClick={() => void startSession()}
            disabled={loading || !!sessionJobId}
          >
            {sessionJobId ? "Session đang chạy…" : "Bắt đầu session → giờ kết thúc"}
          </button>
          {sessionJobId ? (
            <p className="panel-note" style={{ marginTop: 8 }}>
              {sessionProgress || "session…"}
            </p>
          ) : null}
          {error ? <div className="err">{error}</div> : null}
        </aside>

        <div className="main-col">
          <div className="stats">
            {stats.length
              ? stats.map((s) => (
                  <div className="stat" key={s.k}>
                    <div className="k">{s.k}</div>
                    <div className={`v ${s.cls}`}>{s.v}</div>
                  </div>
                ))
              : ["Balance", "Lợi nhuận", "Win rate", "Max DD"].map((k) => (
                  <div className="stat" key={k}>
                    <div className="k">{k}</div>
                    <div className="v">—</div>
                  </div>
                ))}
          </div>
          {liveBook && (result?.openCount || 0) > 0 ? (
            <p className="panel-note" style={{ margin: "0 0 10px" }}>
              Balance = bankroll + realized ({sol(liveBook.realized)}) + open unrealized (
              {sol(liveBook.openUnreal)}
              {liveBook.liveMarks ? ` · ${liveBook.liveMarks} live marks` : ""}). Chart bên dưới
              là equity lúc hết replay — TP/SL chỉ chạy trên signal lịch sử, không tự cắt theo
              giá live.
            </p>
          ) : null}

          <div className="chart-wrap">
            <div className="cap">
              <strong>Balance (SOL)</strong>
              <span>
                {data
                  ? `${data.loaded} signals · ${result?.coins?.length || 0} coins · skip ${result?.skippedEntries ?? 0}`
                  : "—"}
                {savedAt ? ` · saved ${new Date(savedAt).toLocaleString()}` : ""}
              </span>
            </div>
            <EquityChart points={data?.equity || []} startCash={result?.config?.startCash} />
          </div>

          <div className="panel">
            <div className="panel-head">
              <h2>Đồng coin</h2>
              {result ? (
                <span className="muted">
                  {result.closedCount} legs closed · {result.openCount} open · fees{" "}
                  {sol(result.totalFees)}
                </span>
              ) : null}
            </div>
            <CoinTable
              coins={result?.coins || []}
              unit="SOL"
              liveByMint={liveByMint}
              notionalSol={result?.config?.notionalUsd ?? 0.05}
            />
          </div>
        </div>
      </div>
    </section>
  );
}
