import { ChangeDetectionStrategy, Component, computed, inject, input, resource } from '@angular/core';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../core/api/api-error';
import { SEARCH_READER } from '../../core/api/contracts';
import { SearchKind, SearchResult } from '../../core/models/football';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';

const KINDS: { value: SearchKind | ''; label: string }[] = [
  { value: '', label: 'Everything' },
  { value: 'player', label: 'Players' },
  { value: 'team', label: 'Clubs' },
  { value: 'coach', label: 'Coaches' },
  { value: 'league', label: 'Competitions' },
];

@Component({
  selector: 'app-search-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, Loading, Empty, ErrorState],
  template: `
    <main class="page search">
      <h1>Search</h1>
      @if (q()) {
        <p class="summary">Results for <strong>{{ q() }}</strong></p>
      }

      <nav class="kinds">
        @for (option of kinds; track option.value) {
          <a
            [routerLink]="[]"
            [queryParams]="{ q: q(), kind: option.value || null }"
            [class.active]="(kind() ?? '') === option.value"
          >{{ option.label }}</a>
        }
      </nav>

      @if (!q() || q().length < 2) {
        <app-empty
          message="Type at least two characters to search."
          hint="Players, clubs, coaches and competitions are all searchable." />
      } @else if (results.isLoading()) {
        <app-loading message="Searching…" />
      } @else if (results.error()) {
        <app-error-state [message]="errorMessage()" [requestId]="errorRequestId()" />
      } @else if (!results.value()?.items?.length) {
        <app-empty
          [message]="'Nothing matches “' + q() + '”.'"
          hint="Partial words work — try fewer letters." />
      } @else {
        <ul class="results">
          @for (hit of results.value()!.items; track hit.kind + hit.id) {
            <li>
              <a [routerLink]="linkFor(hit)">
                <span class="kind">{{ hit.kind }}</span>
                <span class="name">{{ hit.name }}</span>
                @if (hit.subtitle) {
                  <span class="subtitle">{{ hit.subtitle }}</span>
                }
              </a>
            </li>
          }
        </ul>
      }
    </main>
  `,
  styles: `
    .search { padding-block: var(--space-6); }
    h1 { margin-bottom: var(--space-2); }
    .summary { color: var(--ink-soft); margin-bottom: var(--space-5); }
    .kinds {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-4);
      padding-bottom: var(--space-3);
      border-bottom: 1px solid var(--line);
      margin-bottom: var(--space-5);
    }
    .kinds a {
      font-size: var(--text-sm);
      color: var(--muted);
      text-decoration: none;
    }
    .kinds a.active { color: var(--accent); font-weight: 600; }
    .results { list-style: none; margin: 0; padding: 0; }
    .results li { border-bottom: 1px solid var(--line-soft); }
    .results a {
      display: grid;
      grid-template-columns: 6rem 1fr auto;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      text-decoration: none;
      color: var(--ink);
    }
    .results a:hover .name { color: var(--accent); }
    .kind {
      font-family: var(--font-mono);
      font-size: 10px;
      text-transform: uppercase;
      letter-spacing: 0.1em;
      color: var(--muted);
    }
    .name { font-weight: 600; }
    .subtitle { color: var(--muted); font-size: var(--text-sm); }

    @media (max-width: 34rem) {
      .results a { grid-template-columns: 1fr; gap: var(--space-1); }
    }
  `,
})
export class SearchPage {
  private readonly reader = inject(SEARCH_READER);

  /**
   * Bound from the query string by withComponentInputBinding(), so navigating
   * from ?q=messi to ?q=ronaldo re-runs the search without the component being
   * torn down and rebuilt.
   */
  readonly q = input('');
  readonly kind = input<SearchKind | undefined>(undefined);

  protected readonly kinds = KINDS;

  protected readonly results = resource({
    // Re-runs whenever either changes.
    params: () => ({ q: this.q(), kind: this.kind() }),
    loader: ({ params }) => {
      if (!params.q || params.q.length < 2) {
        return Promise.resolve({ items: [], limit: 25, offset: 0, has_more: false });
      }
      return this.reader.search(params.q, params.kind);
    },
  });

  protected readonly errorMessage = computed(() => {
    const error = this.results.error();
    return error instanceof Error ? error.message : 'Search failed.';
  });

  protected readonly errorRequestId = computed(() => {
    const error = this.results.error();
    return error instanceof ApiError ? (error.requestId ?? null) : null;
  });

  protected linkFor(hit: SearchResult): string[] {
    switch (hit.kind) {
      case 'player': return ['/players', hit.id];
      case 'team': return ['/clubs', hit.id];
      case 'coach': return ['/coaches', hit.id];
      case 'league': return ['/competitions', hit.id];
    }
  }
}
