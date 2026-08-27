import { useEffect, useRef } from "react";
import type { EquityPoint } from "../api";

type Props = {
  points: EquityPoint[];
  startCash?: number;
};

export function EquityChart({ points, startCash }: Props) {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const dpr = window.devicePixelRatio || 1;
    const cssW = canvas.clientWidth || 900;
    const cssH = 280;
    canvas.width = Math.floor(cssW * dpr);
    canvas.height = Math.floor(cssH * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, cssW, cssH);

    if (!points || points.length < 2) {
      ctx.fillStyle = "#4d5f52";
      ctx.font = "14px IBM Plex Sans";
      ctx.fillText("Chạy backtest để xem đường balance.", 16, 40);
      return;
    }

    const vals = points.map((p) => p.equity);
    let min = Math.min(...vals);
    let max = Math.max(...vals);
    if (min === max) {
      min -= 1;
      max += 1;
    }
    const pad = { l: 52, r: 12, t: 16, b: 28 };
    const w = cssW - pad.l - pad.r;
    const h = cssH - pad.t - pad.b;
    const xAt = (i: number) => pad.l + (i / (points.length - 1)) * w;
    const yAt = (v: number) => pad.t + (1 - (v - min) / (max - min)) * h;

    ctx.strokeStyle = "#b7c7b5";
    ctx.lineWidth = 1;
    for (let i = 0; i < 4; i++) {
      const y = pad.t + (h * i) / 3;
      ctx.beginPath();
      ctx.moveTo(pad.l, y);
      ctx.lineTo(pad.l + w, y);
      ctx.stroke();
      const val = max - ((max - min) * i) / 3;
      ctx.fillStyle = "#4d5f52";
      ctx.font = "11px IBM Plex Mono";
      ctx.fillText(val.toFixed(0), 4, y + 4);
    }

    const baseline = startCash ?? points[0].equity;
    const y0 = yAt(baseline);
    ctx.setLineDash([4, 4]);
    ctx.strokeStyle = "#cf5a16";
    ctx.beginPath();
    ctx.moveTo(pad.l, y0);
    ctx.lineTo(pad.l + w, y0);
    ctx.stroke();
    ctx.setLineDash([]);

    const end = points[points.length - 1].equity;
    ctx.beginPath();
    points.forEach((p, i) => {
      const x = xAt(i);
      const y = yAt(p.equity);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.strokeStyle = end >= baseline ? "#1f7a45" : "#b42318";
    ctx.lineWidth = 2.25;
    ctx.stroke();

    ctx.lineTo(xAt(points.length - 1), pad.t + h);
    ctx.lineTo(xAt(0), pad.t + h);
    ctx.closePath();
    ctx.fillStyle = end >= baseline ? "rgba(31,122,69,0.12)" : "rgba(180,35,24,0.10)";
    ctx.fill();

    points.forEach((p, i) => {
      if (!p.event || p.event === "start" || p.event.startsWith("mark:")) return;
      ctx.beginPath();
      ctx.arc(xAt(i), yAt(p.equity), 3.2, 0, Math.PI * 2);
      ctx.fillStyle = p.event.startsWith("entry") ? "#1d4e3b" : "#cf5a16";
      ctx.fill();
    });
  }, [points, startCash]);

  return <canvas ref={ref} className="equity-canvas" width={900} height={280} />;
}
