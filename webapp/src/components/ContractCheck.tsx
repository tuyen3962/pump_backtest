import { useState } from "react";
import { fetchToken, type TokenInfo } from "../api";
import { money, moneyCompact, pct, shortMint } from "../format";

type Props = {
  onChecked?: (info: TokenInfo) => void;
};

export function ContractCheck({ onChecked }: Props) {
  const [mint, setMint] = useState("");
  const [live, setLive] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [info, setInfo] = useState<TokenInfo | null>(null);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    const value = mint.trim();
    if (!value) {
      setError("Nhập mint / contract address");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const data = await fetchToken(value, live);
      setInfo(data);
      onChecked?.(data);
    } catch (err) {
      setInfo(null);
      setError(String((err as Error).message || err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <section className="panel">
      <h2>Check contract</h2>
      <p className="panel-note">Tra volume, liquidity, sell pressure và rug score nhanh theo mint.</p>
      <form className="contract-form" onSubmit={onSubmit}>
        <label htmlFor="mint">Mint address</label>
        <input
          id="mint"
          value={mint}
          onChange={(e) => setMint(e.target.value)}
          placeholder="Base58 mint…"
          spellCheck={false}
          autoComplete="off"
        />
        <label className="check-inline">
          <input type="checkbox" checked={live} onChange={(e) => setLive(e.target.checked)} />
          Sample PumpDev WS (~8s)
        </label>
        <button type="submit" className="run" disabled={loading}>
          {loading ? "Đang check…" : "Check nhanh"}
        </button>
      </form>
      {error ? <div className="err">{error}</div> : null}

      {info ? (
        <div className="token-card">
          <div className="token-head">
            <div>
              <strong>{info.symbol || "—"}</strong>
              <span className="muted"> {info.name || ""}</span>
              <div className="mint">{shortMint(info.mint, 8, 8)}</div>
            </div>
            <span className={`pill rug-${info.rugLabel || "low"}`}>
              {(info.rugLabel || "n/a").toUpperCase()} · {Math.round(info.rugScore || 0)}
            </span>
          </div>

          <div className="metric-grid">
            <Metric label="Vol 24h" value={moneyCompact(info.volumeUsd24h)} />
            <Metric label="Vol 1h" value={moneyCompact(info.volumeUsd1h)} />
            <Metric label="Liquidity" value={moneyCompact(info.liquidityUsd)} />
            <Metric label="Mcap" value={moneyCompact(info.marketCapUsd)} />
            <Metric
              label="Sell 1h"
              value={pct((info.sellRatio1h || 0) * 100, 0)}
              className={(info.sellRatio1h || 0) >= 0.55 ? "down" : "up"}
            />
            <Metric
              label="ATH DD"
              value={`${(info.athDrawdownPct || 0).toFixed(0)}%`}
              className={(info.athDrawdownPct || 0) >= 50 ? "down" : ""}
            />
          </div>

          <div className="token-meta">
            <span>{info.complete ? "Migrated" : "Bonding"}</span>
            <span>{info.dexId || "—"}</span>
            <span>{(info.sources || []).join(" · ") || "—"}</span>
            {info.isBanned ? <span className="down">BANNED</span> : null}
          </div>

          {info.riskNotes?.length ? (
            <ul className="risk-notes">
              {info.riskNotes.map((n) => (
                <li key={n}>{n}</li>
              ))}
            </ul>
          ) : null}

          {info.liveSample ? (
            <div className="live-box">
              Live {info.liveSample.windowSec}s · {info.liveSample.trades} trades · vol{" "}
              {info.liveSample.volumeSol.toFixed(3)} SOL · sell{" "}
              {((info.liveSample.sellRatio || 0) * 100).toFixed(0)}%
            </div>
          ) : null}

          <div className="token-price mono">
            price {money(info.priceUsd, 8)} · buys/sells 1h {info.buys1h ?? 0}/{info.sells1h ?? 0}
          </div>
        </div>
      ) : null}
    </section>
  );
}

function Metric({
  label,
  value,
  className = "",
}: {
  label: string;
  value: string;
  className?: string;
}) {
  return (
    <div className="metric">
      <div className="k">{label}</div>
      <div className={`v ${className}`}>{value}</div>
    </div>
  );
}
