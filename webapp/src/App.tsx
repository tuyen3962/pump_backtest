import { useState } from "react";
import { ContractCheck } from "./components/ContractCheck";
import { BacktestDesk } from "./components/BacktestDesk";
import { LiveFollow } from "./components/LiveFollow";
import { HistoryPanel } from "./components/HistoryPanel";
import { useFollowStream } from "./hooks/useFollowStream";

export default function App() {
  const [historyKey, setHistoryKey] = useState(0);
  const [followKey, setFollowKey] = useState(0);
  const follow = useFollowStream(followKey);

  const bumpAll = () => {
    setHistoryKey((k) => k + 1);
    setFollowKey((k) => k + 1);
  };

  return (
    <div className="wrap">
      <header className="hero">
        <p className="eyebrow">Pump Backtest Desk</p>
        <h1 className="brand">Theo dõi lối đánh memecoin</h1>
        <p className="lede">
          Chạy backtest → coin open được follow live (SSE 5s) ngay trên dashboard đang mở.
        </p>
      </header>

      <div className="top-grid">
        <ContractCheck onFollowed={() => setFollowKey((k) => k + 1)} />
        <section className="panel tip-panel">
          <h2>Workflow</h2>
          <ol className="steps">
            <li>Chạy backtest → kết quả lưu + open vào Follow</li>
            <li>Giữ dashboard mở → SSE push mcap mỗi 5s</li>
            <li>Bảng Đồng coin hiện live return cho open</li>
            <li>Coin đóng / rug vào Lịch sử</li>
          </ol>
        </section>
      </div>

      <BacktestDesk onRunComplete={bumpAll} liveByMint={follow.byMint} />

      <div className="bottom-grid">
        <LiveFollow stream={follow} />
        <HistoryPanel key={historyKey} />
      </div>
    </div>
  );
}
