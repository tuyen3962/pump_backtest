import { useCallback, useState } from "react";
import { ContractCheck } from "./components/ContractCheck";
import { BacktestDesk } from "./components/BacktestDesk";
import { LiveFollow } from "./components/LiveFollow";
import { HistoryPanel } from "./components/HistoryPanel";
import { RunsLab } from "./components/RunsLab";
import { useFollowStream } from "./hooks/useFollowStream";
import type { BacktestRequest } from "./api";

export default function App() {
  const [historyKey, setHistoryKey] = useState(0);
  const [followKey, setFollowKey] = useState(0);
  const [runsKey, setRunsKey] = useState(0);
  const [draft, setDraft] = useState<BacktestRequest | null>(null);
  const [loadRunId, setLoadRunId] = useState<string | null>(null);
  const follow = useFollowStream(followKey);

  const bumpAll = () => {
    setHistoryKey((k) => k + 1);
    setFollowKey((k) => k + 1);
    setRunsKey((k) => k + 1);
  };

  const onDraftChange = useCallback((d: BacktestRequest) => setDraft(d), []);

  return (
    <div className="wrap">
      <header className="hero">
        <p className="eyebrow">Pump Backtest Desk</p>
        <h1 className="brand">Theo dõi lối đánh memecoin</h1>
        <p className="lede">
          Chạy nhiều backtest song song, so sánh ổn định, xuất CSV để optimize trên data cũ.
        </p>
      </header>

      <div className="top-grid">
        <ContractCheck onFollowed={() => setFollowKey((k) => k + 1)} />
        <section className="panel tip-panel">
          <h2>Workflow</h2>
          <ol className="steps">
            <li>Chỉnh strategy → Chạy sync hoặc Queue async nhiều variant</li>
            <li>Lab so sánh PnL / WR / MaxDD → chọn config ổn</li>
            <li>Export CSV (trades/coins/equity/summary)</li>
            <li>Open vẫn follow live; lệnh đóng vào Lịch sử</li>
          </ol>
        </section>
      </div>

      <BacktestDesk
        onRunComplete={bumpAll}
        onDraftChange={onDraftChange}
        liveByMint={follow.byMint}
        loadRunId={loadRunId}
      />

      <RunsLab
        draft={draft}
        refreshKey={runsKey}
        onLoadRun={(id) => {
          setLoadRunId(id);
          setRunsKey((k) => k + 1);
        }}
      />

      <div className="bottom-grid">
        <LiveFollow stream={follow} />
        <HistoryPanel key={historyKey} />
      </div>
    </div>
  );
}
