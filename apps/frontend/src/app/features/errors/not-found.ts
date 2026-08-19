import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterLink } from '@angular/router';

import { Seo } from '../../core/seo/seo';

/**
 * An unknown URL.
 *
 * This replaced `{ path: '**', redirectTo: '' }`, which was a **soft 404**:
 * every mistyped or dead link answered `200 OK` with the transfer feed. For a
 * visitor that silently loses the address they followed; for a crawler it is
 * worse, because a site that returns a valid page for every URL invites an
 * unbounded crawl of URLs that do not exist, and the duplicate content is
 * attributed to the home page.
 *
 * The component alone does not fix it. A wildcard route that resolves to a
 * component is still a *match*, so the render succeeds and the server answers
 * `200 OK` — the page said "not found" while the status said otherwise, which
 * is the same soft 404 wearing a better outfit. The status comes from
 * `app.routes.server.ts`, where the catch-all carries `status: 404`; that only
 * works because every real route is enumerated above it.
 */
@Component({
  selector: 'app-not-found',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <main class="page not-found">
      <p class="code">404</p>
      <h1>That page does not exist</h1>
      <p class="standfirst">
        The link may be out of date, or the record may have been removed. Nothing here is lost — try
        searching for it.
      </p>

      <nav class="ways">
        <a class="btn primary" routerLink="/">Transfer feed</a>
        <a class="btn" routerLink="/clubs">Clubs</a>
        <a class="btn" routerLink="/competitions">Competitions</a>
        <a class="btn" routerLink="/search">Search</a>
      </nav>
    </main>
  `,
  styles: `
    .not-found {
      max-width: 34rem;
      padding-block: var(--space-8);
      text-align: center;
      margin-inline: auto;
    }
    .code {
      font-family: var(--font-mono);
      font-size: var(--text-sm);
      letter-spacing: 0.2em;
      color: var(--muted);
      margin-bottom: var(--space-3);
    }
    h1 {
      font-size: var(--text-2xl);
      margin-bottom: var(--space-3);
    }
    .standfirst {
      color: var(--ink-soft);
      margin-inline: auto;
      margin-bottom: var(--space-6);
    }
    .ways {
      display: flex;
      gap: var(--space-2);
      justify-content: center;
      flex-wrap: wrap;
    }
  `,
})
export class NotFound {
  private readonly seo = inject(Seo);

  constructor() {
    this.seo.describe({
      title: 'Page not found',
      description: 'That page does not exist.',
      path: '/404',
      noindex: true,
    });
  }
}
