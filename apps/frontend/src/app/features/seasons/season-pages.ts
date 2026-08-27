import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  resource,
  signal,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';

import { SEASON_READER, SEASON_WRITER } from '../../core/api/contracts';
import { Permissions } from '../../core/auth/permissions';
import { Seo } from '../../core/seo/seo';
import { Season } from '../../core/models/football';
import { Field } from '../../shared/forms/field';
import { messageFor, requestIdFor } from '../../shared/forms/submit';
import { toApiDate, toDateInput } from '../../shared/util/dates';
import { Empty, ErrorState, Loading } from '../../shared/ui/states';
import { ssrResource } from '../../core/api/ssr-resource';

/**
 * Seasons — the spine of the temporal model.
 *
 * Almost everything else is a statement about a season: which competition a
 * club contested, which transfer window a move fell in. A transfer records
 * itself against whichever season contains its date, so a missing season is
 * not an error but does leave moves unfiled.
 *
 * The list is public because the data is; only administrators may write.
 */
@Component({
  selector: 'app-season-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, Loading, Empty, ErrorState],
  template: `
    <main class="page">
      <header class="head">
        <div>
          <h1>Seasons</h1>
          <p class="standfirst">
            The windows every transfer and competition entry is filed against.
          </p>
        </div>
        @if (permissions.canAdminister()) {
          <a class="btn primary" routerLink="/seasons/new">New season</a>
        }
      </header>

      @if (seasons.isLoading() && !seasons.value()) {
        <app-loading message="Loading seasons…" />
      } @else if (seasons.error() && !seasons.value()) {
        <app-error-state [message]="errorMessage()" />
      } @else if (!seasons.value()?.items?.length) {
        <app-empty
          message="No seasons yet."
          hint="Without one, transfers are recorded but not filed against a season."
        />
      } @else {
        <ul class="list">
          @for (season of seasons.value()!.items; track season.id) {
            <li>
              <span class="label">{{ season.label }}</span>
              <span class="dates tabular">
                {{ season.start_date | date: 'd MMM y' }} – {{ season.end_date | date: 'd MMM y' }}
              </span>
              @if (permissions.canAdminister()) {
                <a class="edit" [routerLink]="['/seasons', season.id, 'edit']">Edit</a>
              }
            </li>
          }
        </ul>
      }
    </main>
  `,
  styles: `
    .head {
      display: flex;
      justify-content: space-between;
      align-items: flex-end;
      gap: var(--space-4);
      padding-block: var(--space-6) var(--space-5);
      flex-wrap: wrap;
    }
    h1 {
      margin-bottom: var(--space-2);
    }
    .standfirst {
      color: var(--ink-soft);
    }
    .list {
      list-style: none;
      margin: 0;
      padding: 0;
    }
    .list li {
      display: flex;
      gap: var(--space-4);
      align-items: baseline;
      padding: var(--space-3) 0;
      border-bottom: 1px solid var(--line-soft);
    }
    .label {
      font-weight: 600;
      min-width: 6rem;
    }
    .dates {
      color: var(--muted);
      font-size: var(--text-sm);
    }
    .edit {
      margin-left: auto;
      font-size: var(--text-sm);
    }
  `,
})
export class SeasonList {
  private readonly reader = inject(SEASON_READER);
  protected readonly permissions = inject(Permissions);
  private readonly seo = inject(Seo);

  constructor() {
    this.seo.describe({
      title: 'Seasons',
      description: 'The seasons every transfer and competition entry is filed against.',
      path: '/seasons',
    });
  }

  protected readonly seasons = ssrResource('season-pages.seasons', {
    loader: () => this.reader.list({ limit: 100 }),
  });

  protected readonly errorMessage = computed(() =>
    messageFor(this.seasons.error(), 'Could not load seasons.'),
  );
}

/**
 * Create or edit a season. Administrators only.
 *
 * Overlap is the interesting failure. The API refuses two seasons covering the
 * same dates with a 409 naming the season already there — a rule that exists
 * because "the season containing this date" has to have one answer. That check
 * needs the other seasons, so it is left to the server rather than duplicated
 * here; the conflict message is already specific enough to act on.
 */
@Component({
  selector: 'app-season-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule, RouterLink, Field, Loading, ErrorState],
  template: `
    <main class="page form-page">
      @if (loading()) {
        <app-loading message="Loading season…" />
      } @else if (existing.error()) {
        <app-error-state [message]="loadErrorMessage()" />
      } @else {
        <header class="head">
          <p class="eyebrow">{{ isEdit() ? 'Edit season' : 'New season' }}</p>
          <h1>{{ isEdit() ? label() || 'Season' : 'Add a season' }}</h1>
        </header>

        <form (ngSubmit)="save()" novalidate>
          <section class="grid">
            <app-field
              for="label"
              label="Label"
              hint="How it is written — 2025/26."
              [error]="fieldError('label')"
            >
              <input id="label" name="label" required [(ngModel)]="label" />
            </app-field>

            <app-field for="start" label="From" [error]="fieldError('start_date')">
              <input id="start" name="start" type="date" required [(ngModel)]="startDate" />
            </app-field>

            <app-field for="end" label="Until" [error]="fieldError('end_date')">
              <input id="end" name="end" type="date" required [(ngModel)]="endDate" />
            </app-field>
          </section>

          @if (saveError()) {
            <app-error-state [message]="saveError()!" [requestId]="saveRequestId()" />
          }

          <footer class="actions">
            <button class="btn primary" type="submit" [disabled]="busy()">
              {{ busy() ? 'Saving…' : isEdit() ? 'Save changes' : 'Create season' }}
            </button>
            <a class="btn" routerLink="/seasons">Cancel</a>
          </footer>
        </form>
      }
    </main>
  `,
  styles: `
    .form-page {
      max-width: 38rem;
      padding-block: var(--space-6) var(--space-8);
    }
    .head {
      margin-bottom: var(--space-6);
    }
    .eyebrow {
      font-family: var(--font-mono);
      font-size: var(--text-xs);
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--muted);
      margin-bottom: var(--space-2);
    }
    h1 {
      font-size: var(--text-2xl);
    }
    form {
      display: flex;
      flex-direction: column;
      gap: var(--space-5);
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(12rem, 1fr));
      gap: var(--space-4);
    }
    .actions {
      display: flex;
      gap: var(--space-3);
    }
  `,
})
export class SeasonForm {
  private readonly reader = inject(SEASON_READER);
  private readonly writer = inject(SEASON_WRITER);
  private readonly router = inject(Router);

  readonly id = input<string | undefined>(undefined);

  protected readonly label = signal('');
  protected readonly startDate = signal('');
  protected readonly endDate = signal('');

  protected readonly busy = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly saveRequestId = signal<string | null>(null);
  private readonly errors = signal<Record<string, string>>({});

  protected readonly isEdit = computed(() => !!this.id());

  protected readonly existing = resource({
    params: () => {
      const id = this.id();
      return id ? { id } : undefined;
    },
    loader: ({ params }) => this.reader.byId(params.id),
  });

  protected readonly loading = computed(() => this.isEdit() && this.existing.isLoading());
  protected readonly loadErrorMessage = computed(() =>
    messageFor(this.existing.error(), 'Could not load the season.'),
  );

  constructor() {
    effect(() => {
      const season = this.existing.value();
      if (!season) return;
      this.label.set(season.label);
      this.startDate.set(toDateInput(season.start_date));
      this.endDate.set(toDateInput(season.end_date));
    });
  }

  protected fieldError(field: string): string | null {
    return this.errors()[field] ?? null;
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;

    const errors: Record<string, string> = {};
    const label = this.label().trim();

    if (!label) errors['label'] = 'A label is required.';
    if (!this.startDate()) errors['start_date'] = 'A start date is required.';
    if (!this.endDate()) errors['end_date'] = 'An end date is required.';
    if (this.startDate() && this.endDate() && this.endDate() <= this.startDate()) {
      errors['end_date'] = 'Must be after the start.';
    }

    this.errors.set(errors);
    if (Object.keys(errors).length > 0) return;

    const body: Partial<Season> = {
      label,
      start_date: toApiDate(this.startDate()),
      end_date: toApiDate(this.endDate()),
    };

    this.busy.set(true);
    this.saveError.set(null);
    this.saveRequestId.set(null);

    try {
      const id = this.id();
      if (id) await this.writer.update(id, body);
      else await this.writer.create(body);
      await this.router.navigateByUrl('/seasons');
    } catch (error) {
      // A 409 here is the overlap rule, and its message names the season
      // already covering those dates.
      this.saveError.set(messageFor(error));
      this.saveRequestId.set(requestIdFor(error));
    } finally {
      this.busy.set(false);
    }
  }
}
