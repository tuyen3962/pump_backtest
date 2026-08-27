import { ContractCheck } from "./components/ContractCheck";
import { BacktestDesk } from "./components/BacktestDesk";

export default function App() {
  return (
    <div className="wrap">
      <header className="hero">
        <p className="eyebrow">Pump Backtest Desk</p>
        <h1 className="brand">Theo dõi lối đánh memecoin</h1>
        <p className="lede">
          Strategy v1: entry <code>whale_armed</code>, bankroll 1 SOL / 0.05 SOL mỗi lệnh — check
          contract, theo dõi balance và tinh chỉnh filter trước khi paper realtime.
        </p>
      </header>

      <div className="top-grid">
        <ContractCheck />
        <section className="panel tip-panel">
          <h2>Workflow</h2>
          <ol className="steps">
            <li>Paste mint → check volume & rug score</li>
            <li>Chọn nguồn signal đã ghi / Demo</li>
            <li>Chỉnh entry / fee / notional</li>
            <li>Đọc balance + PnL + bảng coin để tinh chỉnh lối đánh</li>
          </ol>
          <p className="panel-note">
            Dev: <code>npm run dev</code> trong <code>webapp/</code> (proxy /api → :8080). Prod: build
            embed vào Go dashboard.
          </p>
        </section>
      </div>

      <BacktestDesk />
    </div>
  );
}
