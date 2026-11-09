import { ChangeDetectionStrategy, Component, inject, resource, signal } from '@angular/core';
import { DatePipe } from '@angular/common';

import { NEWS_ADMIN } from '../../core/api/news';
import { AdminNewsStory, NewsRun } from '../../core/models/news';
import { messageFor } from '../../shared/forms/submit';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';
import { categoryLabel } from '../news/news-meta';

const PAGE = 50;

/**
 * Every story the pipeline scored, published or not, with why it got its
 * score. Hide takes a story off both public pages; pin puts it first on both,
 * and pinning one that was never published publishes it.
 */
@Component({
  selector: 'app-news-admin',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe, Loading, Empty, ErrorState],
  template: `
    <main class="page">
      <header class="masthead">
        <div>
          <h1>News</h1>
          <p class="standfirst">Every scored story, its reason and its sources.</p>
        </div>
        <button class="btn" type="button" [disabled]="running()" (click)="runNow()">
          {{ running() ? 'Running…' : 'Run now' }}
        </button>
      </header>

      @if (lastRun(); as r) {
        <p class="notice" role="status">
          @if (r.skipped) {
            Run skipped: {{ r.skipped }}
          } @else {
            Run {{ r.status }}: {{ r.fetched }} fetched, {{ r.new_items }} new, {{ r.published }}
            published, {{ r.ai_neurons }} Neurons.
          }
        </p>
      }
      @if (actionError()) {
        <app-error-state [message]="actionError()!" />
      }

      @if (data.hasValue() ? data.value() : null; as d) {
        <section aria-labelledby="sources-h" class="block">
          <h2 id="sources-h">Sources</h2>
          <ul class="sources">
            @for (s of d.sources; track s.key) {
              <li>
                <strong>{{ s.name }}</strong>
                @if (s.enabled && s.commercial_use_verified) {
                  <span class="ok">fetched</span>
                } @else {
                  <span class="off">off — terms not verified</span>
                }
                <a [href]="s.terms_url" target="_blank" rel="noopener noreferrer">Terms</a>
              </li>
            }
          </ul>
        </section>

        <section aria-labelledby="runs-h" class="block">
          <h2 id="runs-h">Recent runs</h2>
          @if (d.runs.length) {
            <ul class="runs">
              @for (r of d.runs; track r.id) {
                <li>
                  <span class="status" [attr.data-status]="r.status">{{ r.status }}</span>
                  {{ r.started_at | date: 'd MMM, HH:mm' }} — {{ r.fetched }} fetched,
                  {{ r.new_items }} new, {{ r.published }} published, {{ r.pruned }} pruned,
                  {{ r.ai_neurons }} Neurons
                  @if (r.errors.length) {
                    <span class="errors">· {{ r.errors.join('; ') }}</span>
                  }
                </li>
              }
            </ul>
          } @else {
            <p class="muted">No runs yet.</p>
          }
        </section>

        <section aria-labelledby="stories-h" class="block">
          <h2 id="stories-h">Stories</h2>
          @if (d.items.length) {
            <div class="scroll-x">
              <table>
                <thead>
                  <tr>
                    <th scope="col">Score</th>
                    <th scope="col">Story</th>
                    <th scope="col">Category</th>
                    <th scope="col">Sources</th>
                    <th scope="col">State</th>
                    <th scope="col"><span class="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  @for (s of d.items; track s.id) {
                    <tr [class.hidden-row]="s.hidden">
                      <td class="tabular score">{{ s.score }}</td>
                      <td>
                        <a [href]="s.url" target="_blank" rel="noopener noreferrer">{{ s.headline }}</a>
                        <p class="reason">{{ s.scorer }} · {{ s.reason }}</p>
                      </td>
                      <td>{{ label(s.category) }}</td>
                      <td class="muted">{{ s.sources.join(', ') }}</td>
                      <td class="muted">
                        {{ s.published_at ? 'Published ' + (s.published_at | date: 'd MMM HH:mm') : 'Candidate' }}
                        @if (s.pinned) { · pinned }
                        @if (s.hidden) { · hidden }
                      </td>
                      <td class="actions">
                        <button class="btn" type="button" [disabled]="busy() === s.id"
                          [attr.aria-pressed]="s.pinned" (click)="toggle(s, 'pinned')">
                          {{ s.pinned ? 'Unpin' : 'Pin' }}<span class="visually-hidden"> {{ s.headline }}</span>
                        </button>
                        <button class="btn" type="button" [disabled]="busy() === s.id"
                          [attr.aria-pressed]="s.hidden" (click)="toggle(s, 'hidden')">
                          {{ s.hidden ? 'Unhide' : 'Hide' }}<span class="visually-hidden"> {{ s.headline }}</span>
                        </button>
                      </td>
                    </tr>
                  }
                </tbody>
              </table>
            </div>
          } @else {
            <app-empty message="No stories scored yet." hint="Turn on a verified source, then run the job." />
          }
        </section>
      } @else if (data.isLoading()) {
        <app-loading message="Loading news…" />
      } @else if (data.error()) {
        <app-error-state message="Could not load the news admin." />
      }
    </main>
  `,
  styles: `
    .page {
      padding-block: var(--space-6);
    }
    .masthead {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      gap: var(--space-4);
      margin-bottom: var(--space-5);
    }
    .standfirst,
    .muted {
      color: var(--muted);
    }
    .notice {
      padding: var(--space-3) var(--space-4);
      background: var(--accent-soft);
      border-radius: var(--radius);
      margin-bottom: var(--space-4);
    }
    .block {
      margin-bottom: var(--space-6);
    }
    h2 {
      font-size: var(--text-xl);
      margin-bottom: var(--space-3);
    }
    .sources,
    .runs {
      list-style: none;
      margin: 0;
      padding: 0;
      display: grid;
      gap: var(--space-2);
      font-size: var(--text-sm);
    }
    .sources li {
      display: flex;
      gap: var(--space-3);
      align-items: baseline;
      flex-wrap: wrap;
    }
    .ok,
    .status[data-status='ok'] {
      color: var(--positive);
    }
    .off,
    .status[data-status='partial'] {
      color: var(--warning);
    }
    .status[data-status='failed'],
    .errors {
      color: var(--critical);
    }
    .status {
      font-weight: 700;
      text-transform: uppercase;
      font-size: var(--text-xs);
      margin-right: var(--space-2);
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: var(--text-sm);
    }
    th {
      text-align: left;
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      padding: var(--space-3);
      background: var(--surface-2);
    }
    td {
      padding: var(--space-3);
      border-bottom: 1px solid var(--line-soft);
      vertical-align: top;
    }
    .score {
      font-weight: 700;
      font-size: var(--text-base);
    }
    .reason {
      margin-top: var(--space-1);
      font-size: var(--text-xs);
      color: var(--muted);
    }
    .hidden-row {
      opacity: 0.55;
    }
    .actions {
      display: flex;
      gap: var(--space-2);
      white-space: nowrap;
    }
    .actions .btn {
      padding: var(--space-1) var(--space-3);
    }
  `,
})
export class NewsAdmin {
  private readonly admin = inject(NEWS_ADMIN);

  protected readonly data = resource({
    loader: () => this.admin.overview({ limit: PAGE }),
  });
  protected readonly busy = signal<string | null>(null);
  protected readonly running = signal(false);
  protected readonly lastRun = signal<NewsRun | null>(null);
  protected readonly actionError = signal<string | null>(null);

  protected label = categoryLabel;

  protected async toggle(s: AdminNewsStory, flag: 'pinned' | 'hidden'): Promise<void> {
    this.busy.set(s.id);
    this.actionError.set(null);
    try {
      await this.admin.setFlags(s.id, { [flag]: !s[flag] });
      this.data.reload();
    } catch (e) {
      this.actionError.set(messageFor(e, 'Could not update the story.'));
    } finally {
      this.busy.set(null);
    }
  }

  protected async runNow(): Promise<void> {
    this.running.set(true);
    this.actionError.set(null);
    try {
      this.lastRun.set(await this.admin.runNow());
      this.data.reload();
    } catch (e) {
      this.actionError.set(messageFor(e, 'The run failed.'));
    } finally {
      this.running.set(false);
    }
  }
}
