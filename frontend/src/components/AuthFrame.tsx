import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';

// Sensitive account pages share a self-contained frame without external media.
export function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <main className="verification-page">
      <Link to="/" className="wordmark" aria-label="SREZ — на главную">
        SREZ<span>®</span>
      </Link>
      <section className="verification-card">
        <p className="eyebrow">СВОЙ КРУГ / SREZ</p>
        {children}
      </section>
      <p className="mono verification-footer">НЕЗАВИСИМОСТЬ НАЧИНАЕТСЯ С ВЫБОРА.</p>
    </main>
  );
}
