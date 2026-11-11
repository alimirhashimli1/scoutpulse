import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';

import { RoomPlayer } from '../../../../core/models/games';
import { GamesI18n } from '../../games-i18n';

/**
 * The room's scoreboard, best first. A few players are chips in a row; a big
 * room (up to thirty) becomes a compact ranked table that scrolls, so the
 * pitch stays on a phone's screen.
 */
@Component({
  selector: 'app-pfb-room-players',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <ol class="players" [class.many]="many()" [attr.aria-label]="t().players">
      @for (p of players(); track p.id; let i = $index) {
        <li [class.me]="p.id === me()" [class.away]="!p.online">
          @if (many()) {
            <span class="rank tabular">{{ i + 1 }}</span>
          }
          <span class="who">
            {{ p.name }}
            @if (p.id === me()) {
              <small>{{ t().you }}</small>
            }
            @if (p.id === host()) {
              <small class="host">{{ t().hostTag }}</small>
            }
            @if (!p.online) {
              <small>{{ t().offline }}</small>
            }
          </span>
          <span class="score tabular">{{ p.score }}</span>
        </li>
      }
    </ol>
  `,
  styles: `
    .players {
      display: flex;
      flex-wrap: wrap;
      gap: var(--space-2);
      margin: 0;
      padding: 0;
      list-style: none;
    }
    .players li {
      display: flex;
      align-items: center;
      gap: var(--space-3);
      padding: var(--space-2) var(--space-3);
      border: 1px solid var(--line);
      border-radius: 99px;
      background: var(--surface);
      min-width: 0;
    }
    .players li.me {
      border-color: var(--accent);
    }
    .players li.away {
      opacity: 0.55;
    }
    /* A big room: a compact ranked table that scrolls, so thirty names do not
       push the pitch off a phone screen. */
    .players.many {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(10rem, 1fr));
      gap: 4px;
      max-height: 9.5rem;
      overflow-y: auto;
      padding: 4px;
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      background: var(--surface-2);
    }
    .players.many li {
      padding: 4px var(--space-2);
      border-radius: var(--radius);
      gap: var(--space-2);
      font-size: var(--text-sm);
    }
    .players.many .who {
      flex: 1;
    }
    .players.many .score {
      font-size: var(--text-base);
    }
    .rank {
      min-width: 1.4em;
      color: var(--muted);
      font-size: var(--text-xs);
    }

    .who {
      display: flex;
      align-items: baseline;
      gap: var(--space-1);
      min-width: 0;
      overflow-wrap: anywhere;
      font-weight: 600;
    }
    .who small {
      font-weight: 400;
      font-size: var(--text-xs);
      color: var(--muted);
    }
    .who small.host {
      color: var(--accent);
    }
    .score {
      font: 700 var(--text-lg) / 1 var(--font-display);
    }
  `,
})
export class RoomPlayers {
  protected readonly t = inject(GamesI18n).t;

  /** Already in order. */
  readonly players = input.required<RoomPlayer[]>();
  readonly host = input('');
  readonly me = input('');

  protected readonly many = computed(() => this.players().length > 8);
}
