import { ChangeDetectionStrategy, Component, computed, inject, input, model } from '@angular/core';

import { LookupStore } from '../../core/api/lookup-store';
import { Permissions } from '../../core/auth/permissions';

/**
 * Picks a club.
 *
 * A native `<select>` over the whole club list, which is the same trade
 * `LookupStore` already makes and carries the same limit: fine for hundreds of
 * clubs, wrong for tens of thousands. At that size this becomes a type-ahead
 * against `/search?kind=team`, and the change is confined to this component.
 *
 * `restrict` is the interesting input. On a form where the *choice determines
 * whether the save is permitted* — creating a player, signing one — offering
 * clubs the user has no grant for produces a form that can only fail. Setting
 * it lists the clubs they can actually write to. Where the club is fixed by
 * the record instead, the full list is right.
 */
@Component({
  selector: 'app-club-select',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <select
      [id]="id()"
      [name]="id()"
      [value]="value() ?? ''"
      (change)="pick($event)"
      [attr.aria-describedby]="describedBy()"
    >
      @if (allowNone()) {
        <option value="">{{ noneLabel() }}</option>
      }
      @for (club of options(); track club.id) {
        <option [value]="club.id">{{ club.name }}</option>
      }
    </select>
  `,
})
export class ClubSelect {
  private readonly lookup = inject(LookupStore);
  private readonly permissions = inject(Permissions);

  readonly id = input.required<string>();
  readonly value = model<string | null>(null);

  /** Offer only clubs the signed-in user may write to. */
  readonly restrict = input(false);
  /** Whether "no club" is a choice. It means free agent, or unattached. */
  readonly allowNone = input(true);
  readonly noneLabel = input('— none —');
  readonly describedBy = input<string | null>(null);

  protected readonly options = computed(() => {
    const all = [...this.lookup.teams().values()].sort((a, b) => a.name.localeCompare(b.name));
    if (!this.restrict() || this.permissions.isAdmin()) return all;
    return all.filter((club) => this.permissions.canEditTeam(club.id));
  });

  protected pick(event: Event): void {
    const chosen = (event.target as HTMLSelectElement).value;
    // An empty option means "no club", which is a real value here — a free
    // agent — so it becomes null rather than an empty string the API would
    // reject as a malformed uuid.
    this.value.set(chosen === '' ? null : chosen);
  }
}
