import { Component } from 'react';
import type { ErrorInfo, ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { Button, PageTitle } from './ui';

// The common header/footer survive a page crash. Changing the route allows
// recovery without retaining an error from the previous page.
export class PageBoundary extends Component<
  { children: ReactNode; routeKey: string },
  { failed: boolean; routeKey: string }
> {
  state = { failed: false, routeKey: this.props.routeKey };
  static getDerivedStateFromProps(props: { routeKey: string }, state: { routeKey: string }) {
    return props.routeKey === state.routeKey ? null : { routeKey: props.routeKey, failed: false };
  }
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch(_error: Error, _info: ErrorInfo) {
    /* No response/credential logging. */
  }
  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div className="container page page--narrow system-page">
        <PageTitle eyebrow="SREZ" title="Не удалось открыть страницу." />
        <p className="muted">Попробуйте обновить страницу или вернитесь на главную.</p>
        <div className="button-row">
          <Button onClick={() => window.location.reload()}>Обновить страницу</Button>
          <Link className="button button--outline" to="/">
            На главную
          </Link>
        </div>
      </div>
    );
  }
}
