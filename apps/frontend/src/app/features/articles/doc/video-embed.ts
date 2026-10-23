import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { DomSanitizer, SafeResourceUrl } from '@angular/platform-browser';

import { VideoProvider, videoEmbedUrl, videoPoster } from './article-doc';

/**
 * A video in an article, as a poster until it is played.
 *
 * The player is not loaded with the page. A YouTube iframe is a megabyte of
 * script and a set of cookies per embed, for a reader who may never press
 * play, so the page shows the video's still and swaps in the player -- from
 * YouTube's no-cookie host -- on the first click.
 */
@Component({
  selector: 'app-article-video',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="frame">
      @if (playing() && src(); as s) {
        <iframe
          [src]="s"
          [title]="caption() || 'Video'"
          allow="autoplay; encrypted-media; picture-in-picture; fullscreen"
          allowfullscreen
          referrerpolicy="strict-origin-when-cross-origin"
        ></iframe>
      } @else {
        <button type="button" class="poster" (click)="playing.set(true)">
          @if (poster(); as p) {
            <img [src]="p" alt="" loading="lazy" referrerpolicy="no-referrer" />
          }
          <span class="play" aria-hidden="true">
            <svg viewBox="0 0 24 24" width="28" height="28">
              <path d="M8 5v14l11-7z" fill="currentColor" />
            </svg>
          </span>
          <span class="visually-hidden">Play video{{ caption() ? ': ' + caption() : '' }}</span>
          <span class="via">{{ provider() === 'youtube' ? 'YouTube' : 'Vimeo' }}</span>
        </button>
      }
    </div>
  `,
  styles: `
    .frame {
      position: relative;
      aspect-ratio: 16 / 9;
      border-radius: var(--radius-lg);
      overflow: hidden;
      background: #0b0f12;
    }
    iframe,
    .poster {
      position: absolute;
      inset: 0;
      width: 100%;
      height: 100%;
      border: 0;
    }
    .poster {
      padding: 0;
      cursor: pointer;
      background: #0b0f12;
      display: grid;
      place-items: center;
    }
    .poster img {
      position: absolute;
      inset: 0;
      width: 100%;
      height: 100%;
      object-fit: cover;
      opacity: 0.85;
      transition: opacity 0.2s ease;
    }
    .poster:hover img {
      opacity: 1;
    }
    .play {
      position: relative;
      width: 4.25rem;
      height: 4.25rem;
      display: grid;
      place-items: center;
      padding-left: 4px;
      border-radius: 50%;
      background: rgb(10 14 18 / 70%);
      border: 2px solid rgb(255 255 255 / 75%);
      color: #fff;
      backdrop-filter: blur(4px);
      transition: transform 0.15s ease;
    }
    .poster:hover .play {
      transform: scale(1.06);
    }
    .via {
      position: absolute;
      right: var(--space-3);
      bottom: var(--space-3);
      font-size: var(--text-xs);
      color: #e2e8f0;
      background: rgb(10 14 18 / 60%);
      padding: 2px 8px;
      border-radius: 999px;
    }
  `,
})
export class ArticleVideo {
  readonly provider = input.required<VideoProvider>();
  readonly videoId = input.required<string>();
  readonly caption = input('');

  private readonly sanitizer = inject(DomSanitizer);
  protected readonly playing = signal(false);

  protected readonly poster = computed(() => videoPoster(this.provider(), this.videoId()));

  /**
   * Trusted only after videoEmbedUrl has rebuilt the address from a provider
   * and an id that both passed their patterns -- never from what was stored.
   */
  protected readonly src = computed<SafeResourceUrl | null>(() => {
    const url = videoEmbedUrl(this.provider(), this.videoId());
    return url ? this.sanitizer.bypassSecurityTrustResourceUrl(url) : null;
  });
}
