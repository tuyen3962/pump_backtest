import { useEffect, useMemo, useState } from "react";
import {
  fetchLastBacktest,
  fetchSources,
  runBacktest,
  type BacktestResponse,
  type FollowRow,
  type Source,
} from "../api";
import { pnlClass } from "../format";
import { CoinTable } from "./CoinTable";
import { EquityChart } from "./EquityChart";

const ENTRY_OPTIONS = ["whale_armed", "pump", "arm", "whale", "milestone"] as const;

function sol(n: number | undefined | null, digits = 4): string {
  const v = Number(n ?? 0);
  const sign = v < 0 ? "-" : "";
  return `${sign}${Math.abs(v).toFixed(digits)} SOL`;
}

type Props = {
  onRunComplete?: () => void;
  liveByMint?: Record<string, FollowRow>;
};

export function BacktestDesk({ onRunComplete, liveByMint }: Props) {
  const [sources, setSources] = useState<Source[]>([]);
  const [source, setSource] = useState("live");
  const [entryKinds, setEntryKinds] = useState<string[]>(["whale_armed"]);
  const [startCash, setStartCash] = useState(1);
  const [notional, setNotional] = useState(0.05);
  const [feeBps, setFeeBps] = useState(100);
  const [latency, setLatency] = useState(5);
  const [minLiq, setMinLiq] = useState(5000);
  const [minVol1h, setMinVol1h] = useState(2000);
  const [maxPos, setMaxPos] = useState(0);
  const [closeEod, setCloseEod] = useState(true);
  const [enrich, setEnrich] = useState(true);
  const [sampleLive, setSampleLive] = useState(false);
  const [disableFilters, setDisableFilters] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [savedAt, setSavedAt] = useState("");
  const [data, setData] = useState<BacktestResponse | null>(null);

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
        }
      })
      .catch(() => {
        /* no prior run is fine */
      });
  }, []);

  const result = data?.result;
  const stats = useMemo(() => {
    if (!result) return [];
    const pnl = result.totalPnl || 0;
    return [
      { k: "Balance", v: sol(result.endEquity), cls: pnlClass(pnl) },
      { k: "Lợi nhuận", v: sol(pnl), cls: pnlClass(pnl) },
      { k: "Win rate", v: `${(result.winRate || 0).toFixed(1)}%`, cls: "" },
      {
        k: "Max DD",
        v: `${(result.maxDrawdownPct || 0).toFixed(2)}%`,
        cls: (result.maxDrawdownPct || 0) > 0 ? "down" : "",
      },
    ];
  }, [result]);

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
        source,
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
        stopLossPct: 60,
        scaleTriggerPct: 15,
        disableFilters,
        // legacy unused
        exitMustOut: true,
        exitWatchOut: false,
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

  return (
    <section className="backtest-block">
      <div className="layout">
        <aside className="panel sticky">
          <h2>Strategy v1</h2>
          <p className="panel-note">
            Entry <code>whale_armed</code> · stop -60% · TP 2x bán ½ · scale +15% · out stale/dev_sold/whale_dump
          </p>

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

          <label htmlFor="minLiq">Min liquidity ($)</label>
          <input
            id="minLiq"
            type="number"
            value={minLiq}
            min={0}
            step={500}
            onChange={(e) => setMinLiq(Number(e.target.value))}
          />

          <label htmlFor="minVol1h">Min vol 1h ($)</label>
          <input
            id="minVol1h"
            type="number"
            value={minVol1h}
            min={0}
            step={500}
            onChange={(e) => setMinVol1h(Number(e.target.value))}
          />

          <label htmlFor="maxPos">Max concurrent (0 = ∞)</label>
          <input
            id="maxPos"
            type="number"
            value={maxPos}
            min={0}
            step={1}
            onChange={(e) => setMaxPos(Number(e.target.value))}
          />

          <div className="checks" style={{ marginTop: 14 }}>
            <label>
              <input type="checkbox" checked={closeEod} onChange={(e) => setCloseEod(e.target.checked)} />
              Mark-to-market open at end
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

          <button className="run" onClick={() => void run()} disabled={loading}>
            {loading ? "Đang chạy…" : "Chạy backtest"}
          </button>
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
            <CoinTable coins={result?.coins || []} unit="SOL" liveByMint={liveByMint} />
          </div>
        </div>
      </div>
    </section>
  );
}
