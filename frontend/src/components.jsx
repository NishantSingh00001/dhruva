import React from "react";

export function Card({ title, action, children, className = "", style }) {
  return (
    <div className={`card fade-in ${className}`} style={style}>
      {title && (
        <div className="card-head">
          <h3>{title}</h3>
          {action !== undefined ? action : <span className="more">⋯</span>}
        </div>
      )}
      {children}
    </div>
  );
}

export function StatPair({ a, b }) {
  return (
    <div className="stat-row">
      <Stat {...a} />
      <Stat {...b} />
    </div>
  );
}

export function Stat({ value, label, trend, unit = "%" }) {
  return (
    <div className="stat">
      <span className={`arrow ${trend}`}>{trend === "up" ? "▲" : "▼"}</span>
      <div>
        <div className="value">
          {value}
          {unit}
        </div>
        <span className="label">{label}</span>
      </div>
    </div>
  );
}

// Data-driven dot census: one dot per unit (one per vehicle, one per event),
// grouped and colored by `segments`. Deterministic — no Math.random — because
// each dot stands for something real, not decoration.
export function PulseDots({ segments }) {
  const cells = segments.flatMap((seg) => Array.from({ length: seg.count }, () => seg.color));
  return (
    <div className="pulse-dots">
      {cells.map((c, i) => (
        <span key={i} className={c} />
      ))}
    </div>
  );
}

export function TimelineFeed({ rows, colorMap }) {
  return (
    <div>
      {rows.map((r, i) => (
        <div className="timeline-row" key={i}>
          <span className="tdate">{r.date}</span>
          <div className="timeline-track">
            <div
              className="timeline-bar"
              style={{
                width: `${Math.min(100, r.value * 2.4)}%`,
                background: colorMap[r.type] || "#333",
                color: ["#ffffff", "var(--green)", "var(--green-dim)"].includes(colorMap[r.type]) ? "#0a0a0a" : "#0a0a0a",
              }}
            >
              <span className="pip">{r.pip}</span>
              <span className="label-text">{r.label}</span>
              <span className="count">{r.value}</span>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

export function CapsuleChart({ data }) {
  const max = Math.max(...data.map((d) => d.value), 1);
  return (
    <div className="capsule-row">
      {data.map((d, i) => (
        <div className="capsule-col" key={i}>
          <div
            className="capsule"
            style={{ height: `${Math.max(16, (d.value / max) * 180)}px`, background: d.color }}
          >
            {d.value}
          </div>
          <span className="capsule-label">{d.label}</span>
        </div>
      ))}
    </div>
  );
}

export function ScoreList({ rows }) {
  return (
    <div className="score-list">
      {rows.map((r, i) => (
        <div className="score-row" key={i}>
          <span className="name">{r.vehicle}</span>
          <div className="bar-bg">
            <div
              className="bar-fill"
              style={{
                width: `${r.score}%`,
                background: r.score >= 80 ? "var(--green)" : r.score >= 60 ? "var(--orange)" : "#ff5c5c",
              }}
            />
          </div>
          <span className="num">{r.score}</span>
        </div>
      ))}
    </div>
  );
}

export function EventChip({ severity, children }) {
  return <span className={`event-chip ${severity}`}>{children}</span>;
}

export function Legend({ items, total }) {
  return (
    <div className="legend">
      {items.map((it, i) => (
        <span className="legend-item" key={i}>
          <span className="legend-dot" style={{ background: it.color }} />
          {it.label}
        </span>
      ))}
      {total != null && (
        <span className="total">
          Total: <b>{total}</b>
        </span>
      )}
    </div>
  );
}
