import { ChangeDetectionStrategy, Component, input } from '@angular/core';

import { NewsCategory } from '../../core/models/news';

/**
 * A category glyph, standing in for the photo a news card would otherwise
 * carry: feed images are never shown. Decorative -- the category is always
 * written next to it -- so it is hidden from assistive technology.
 */
@Component({
  selector: 'app-news-icon',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { 'aria-hidden': 'true' },
  template: `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"
      stroke-linecap="round" stroke-linejoin="round">
      @switch (category()) {
        @case ('transfers') {
          <path d="M4 8h13l-3-3M20 16H7l3 3" />
        }
        @case ('injuries') {
          <rect x="3" y="3" width="18" height="18" rx="4" />
          <path d="M12 8v8M8 12h8" />
        }
        @case ('results') {
          <path d="M8 4h8v5a4 4 0 0 1-8 0V4ZM8 6H5a3 3 0 0 0 3 4M16 6h3a3 3 0 0 1-3 4M12 13v4M8 20h8" />
        }
        @case ('managers') {
          <rect x="4" y="3" width="16" height="18" rx="2" />
          <path d="M8 8h8M8 12h5M8 16h3" />
        }
        @case ('young_talents') {
          <path d="m12 3 2.6 5.6 6 .7-4.5 4.1 1.2 6L12 16.4 6.7 19.4l1.2-6L3.4 9.3l6-.7L12 3Z" />
        }
        @default {
          <circle cx="12" cy="12" r="9" />
          <path d="M12 7l3.5 2.5-1.3 4H9.8l-1.3-4L12 7Z" />
        }
      }
    </svg>
  `,
  styles: `
    :host {
      display: inline-grid;
      place-items: center;
    }
    svg {
      width: 100%;
      height: 100%;
    }
  `,
})
export class NewsIcon {
  readonly category = input.required<NewsCategory>();
}
