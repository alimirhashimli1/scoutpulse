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
import { RouterLink } from '@angular/router';

import { TACTIC_READER } from '../../../core/api/tactics';
import { BoardView } from '../../tactics/board/board-view';
import type { CameraView } from '../../tactics/board/engine';
import { emptyDoc, normalizeDoc } from '../../tactics/board/model';
import { BoardPlayer } from '../../tactics/board/player';
import { TacticIcon } from '../../tactics/ui/icon';
import { PlaybackBar } from '../../tactics/ui/playback-bar';

const VIEWS: { value: CameraView; label: string; icon: string }[] = [
  { value: 'top', label: 'From above', icon: 'camTop' },
  { value: 'tactical', label: 'Tactical camera', icon: 'camTilt' },
  { value: 'broadcast', label: 'Broadcast camera', icon: 'camSide' },
];

/**
 * A tactics board playing inside an article.
 *
 * The same 3D board and controls as the board's own page, so a reader can
 * play, scrub and turn the camera without leaving the piece. The article
 * mounts it with `@defer (on viewport)`: nothing of Three.js or the board is
 * fetched until a reader scrolls to it, and the server never renders one.
 *
 * A board is unlisted, but putting one in a published article is its
 * author's decision to show it to every reader -- the editor says so when it
 * is inserted.
 */
@Component({
  selector: 'app-article-tactic',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, BoardView, PlaybackBar, TacticIcon],
  template: `
    @if (board.value(); as b) {
      <div class="stage">
        <app-board-view [doc]="doc()" [frame]="player.frame()" [view]="view()" [tick]="tick" />
        <div class="views" role="group" aria-label="Camera">
          @for (v of views; track v.value) {
            <button
              type="button"
              [class.on]="view() === v.value"
              [title]="v.label"
              [attr.aria-label]="v.label"
              (click)="view.set(v.value)"
            >
              <app-tactic-icon [name]="v.icon" />
            </button>
          }
        </div>
        @if (player.frame().note) {
          <p class="caption">{{ player.frame().note }}</p>
        }
        @if (!started() && doc().steps.length > 1) {
          <button
            type="button"
            class="big-play"
            [attr.aria-label]="'Play ' + b.title"
            (click)="start()"
          >
            <app-tactic-icon name="play" />
          </button>
        }
      </div>
      <app-playback-bar [player]="player" />
      <p class="meta">
        <app-tactic-icon name="link" />
        <span class="name">{{ b.title }}</span>
        <span class="by">by {{ b.owner_name || 'a member' }}</span>
        <a [routerLink]="['/tactics', b.slug]">Open the board</a>
      </p>
    } @else if (board.error()) {
      <div class="stage missing">
        <p>This tactics board is no longer available.</p>
      </div>
    } @else {
      <div class="stage"><p class="loading">Setting out the pitch…</p></div>
    }
  `,
  styleUrls: ['../../tactics/ui/stage.css'],
  styles: `
    /* Controls in the interface's type, not the article's serif around them. */
    :host {
      display: block;
      font-family: var(--font-body);
      font-size: var(--text-base);
      line-height: var(--leading-normal);
    }
    .stage {
      position: relative;
      aspect-ratio: 16 / 9;
      border-radius: var(--radius-lg);
      overflow: hidden;
      background: radial-gradient(ellipse at center, #3f9b4c 0%, #24572c 100%);
      box-shadow: var(--shadow);
    }
    .missing p,
    .loading {
      position: absolute;
      inset: 0;
      display: grid;
      place-items: center;
      margin: 0;
      max-width: none;
      color: #ecfdf5;
      font-size: var(--text-sm);
    }
    .big-play {
      position: absolute;
      left: 50%;
      top: 50%;
      transform: translate(-50%, -50%);
      width: 4.5rem;
      height: 4.5rem;
      display: grid;
      place-items: center;
      border-radius: 50%;
      border: 2px solid rgb(255 255 255 / 70%);
      background: rgb(10 14 18 / 55%);
      backdrop-filter: blur(6px);
      color: #fff;
      cursor: pointer;
      box-shadow: 0 8px 30px rgb(0 0 0 / 35%);
    }
    .big-play app-tactic-icon {
      width: 1.8rem;
      height: 1.8rem;
      margin-left: 4px;
    }
    app-playback-bar {
      margin-top: var(--space-2);
    }
    .meta {
      display: flex;
      align-items: center;
      gap: var(--space-2);
      flex-wrap: wrap;
      margin-top: var(--space-2);
      max-width: none;
      font-family: var(--font-body);
      font-size: var(--text-xs);
      color: var(--muted);
    }
    .meta app-tactic-icon {
      width: 0.9rem;
      height: 0.9rem;
    }
    .name {
      font-weight: 600;
      color: var(--ink-soft);
    }
    .meta a {
      margin-left: auto;
    }
    @media (max-width: 40rem) {
      .stage {
        aspect-ratio: 4 / 3;
      }
    }
  `,
})
export class ArticleTactic {
  readonly slug = input.required<string>();

  private readonly reader = inject(TACTIC_READER);

  protected readonly board = resource({
    params: () => this.slug(),
    loader: ({ params }) => this.reader.bySlug(params),
  });
  protected readonly doc = computed(() => {
    const b = this.board.value();
    return b ? normalizeDoc(b.content) : emptyDoc();
  });
  protected readonly player = new BoardPlayer(this.doc);
  protected readonly tick = (dt: number) => this.player.tick(dt);
  protected readonly views = VIEWS;
  protected readonly view = signal<CameraView>('tactical');
  protected readonly started = signal(false);

  constructor() {
    effect(() => {
      if (this.player.playing()) this.started.set(true);
    });
  }

  protected start(): void {
    this.started.set(true);
    this.player.restart();
  }
}
