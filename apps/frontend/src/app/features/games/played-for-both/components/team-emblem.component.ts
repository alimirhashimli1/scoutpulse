import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { SideView } from '../played-for-both.store';

let nextClipId = 0;

/** The shield outline, shared by the clip path and the white border. */
const SHIELD = 'M50 4 L94 16 V58 C94 86 74 104 50 112 C26 104 6 86 6 58 V16 Z';

/**
 * A club badge, a national flag, or — before the first pair — a "?".
 *
 * Clubs are drawn rather than shown as crests: official crests are
 * trademarks, and the platform does not hold licensed ones. The badge is a
 * shield in the club's two colours with a short code across a dark band.
 *
 * Flags are the 4:3 SVGs from the flag-icons package (MIT), copied to
 * /flags/4x3 at build time (angular.json). An <img> rather than the
 * package's `fi fi-xx` classes: the stylesheet pulls in both aspect ratios
 * of all 270 flags, and an image carries its alt text natively.
 */
@Component({
  selector: 'app-team-emblem',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (side(); as s) {
      @if (s.kind === 'club') {
        <svg viewBox="0 0 100 116" role="img" [attr.aria-label]="s.alt">
          <defs>
            <clipPath [attr.id]="clipId"><path [attr.d]="shield" /></clipPath>
          </defs>
          <g [attr.clip-path]="'url(#' + clipId + ')'">
            <rect width="100" height="116" [attr.fill]="s.colour1" />
            <path d="M0 0 H100 V116 Z" [attr.fill]="s.colour2" opacity=".9" />
            <rect x="0" y="44" width="100" height="30" fill="rgba(0,0,0,.55)" />
          </g>
          <path [attr.d]="shield" fill="none" stroke="#fff" stroke-width="3.5" />
          <text
            x="50"
            [attr.y]="59 + fontSize() * 0.36"
            text-anchor="middle"
            font-weight="800"
            [attr.font-size]="fontSize()"
            fill="#fff"
            letter-spacing="1"
            aria-hidden="true"
          >
            {{ s.code }}
          </text>
        </svg>
      } @else {
        <img
          class="flag"
          [src]="'flags/4x3/' + s.flag + '.svg'"
          [alt]="s.alt"
          width="160"
          height="120"
          decoding="async"
        />
      }
    } @else {
      <span class="q" aria-hidden="true">?</span>
    }
  `,
  styles: `
    :host {
      width: min(170px, 32vw);
      aspect-ratio: 1;
      display: grid;
      place-items: center;
      filter: drop-shadow(0 8px 14px rgb(0 0 0 / 25%));
    }
    svg {
      width: 86%;
      height: auto;
    }
    text {
      /* Condensed and heavy, from whatever the system has. */
      font-family:
        'Saira Condensed', 'Roboto Condensed', 'Arial Narrow', 'Helvetica Neue', Impact, sans-serif;
      font-stretch: condensed;
    }
    .flag {
      width: 100%;
      height: auto;
      aspect-ratio: 4 / 3;
      object-fit: cover;
      display: block;
      border-radius: 6px;
      border: 2px solid rgb(255 255 255 / 85%);
    }
    .q {
      width: 72%;
      aspect-ratio: 1;
      border: 3px dashed var(--line);
      border-radius: 50%;
      display: grid;
      place-items: center;
      font: 400 clamp(3rem, 9vw, 5rem) / 1 var(--font-display);
      color: var(--muted);
    }
    @media (max-width: 560px) {
      :host {
        width: 130px;
      }
    }
  `,
})
export class TeamEmblem {
  /** Null shows the "?" placeholder of the idle state. */
  readonly side = input<SideView | null>(null);

  /** Unique per instance: two badges on screen must not share a clip path. */
  protected readonly clipId = `pfb-clip-${nextClipId++}`;
  protected readonly shield = SHIELD;

  protected readonly fontSize = computed(() => {
    const len = this.side()?.code?.length ?? 3;
    return len > 3 ? 28 : len === 3 ? 36 : 42;
  });
}
