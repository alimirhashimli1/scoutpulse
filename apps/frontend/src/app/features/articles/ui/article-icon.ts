import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';

/**
 * Line icons for the writing tools, on the same 24-unit grid and stroke as
 * the tactics board's, so the two features read as one product.
 */
const ICONS: Record<string, string> = {
  bold: '<path d="M7 5h6a3.5 3.5 0 010 7H7zM7 12h7a3.5 3.5 0 010 7H7z" stroke-width="2.2"/>',
  italic: '<path d="M10 5h8M6 19h8M14 5l-4 14"/>',
  underline: '<path d="M7 4v7a5 5 0 0010 0V4M5 20h14"/>',
  strike:
    '<path d="M5 12h14M16.5 7.5C16 6 14.5 5 12 5c-3 0-4.5 1.5-4.5 3.3 0 1.2.7 2.2 2.3 2.8M8 16.5c.6 1.6 2.2 2.5 4.2 2.5 3 0 4.6-1.4 4.6-3.3 0-.6-.2-1.2-.5-1.7"/>',
  link: '<path d="M10 14a4 4 0 005.7 0l3-3a4 4 0 00-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 00-5.7 0l-3 3a4 4 0 005.7 5.7l1-1"/>',
  bullets:
    '<path d="M9 6h11M9 12h11M9 18h11"/><circle cx="4.5" cy="6" r="1" fill="currentColor"/><circle cx="4.5" cy="12" r="1" fill="currentColor"/><circle cx="4.5" cy="18" r="1" fill="currentColor"/>',
  numbers:
    '<path d="M10 6h10M10 12h10M10 18h10M4 5l1.5-1V9M3.5 14.5a1.5 1.5 0 013 .5c0 1-3 2-3 3.5h3"/>',
  quote:
    '<path d="M5 18c0-5 1-8 5-10M14 18c0-5 1-8 5-10"/><path d="M5 18h4v-4H5zM14 18h4v-4h-4z"/>',
  divider:
    '<path d="M4 12h16"/><circle cx="6" cy="7" r=".8" fill="currentColor"/><circle cx="12" cy="7" r=".8" fill="currentColor"/><circle cx="18" cy="7" r=".8" fill="currentColor"/>',
  image:
    '<rect x="3.5" y="5" width="17" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="M4 17l5-4.5 4 3.5 3-2.5 4 3.5"/>',
  board:
    '<rect x="3" y="5" width="18" height="14" rx="1.5"/><path d="M12 5v14"/><circle cx="12" cy="12" r="2.5"/><path d="M3 9.5h2.5v5H3M21 9.5h-2.5v5H21"/>',
  video:
    '<rect x="3" y="5.5" width="18" height="13" rx="2.5"/><path d="M10 9.2v5.6l4.8-2.8z" fill="currentColor"/>',
  undo: '<path d="M9 14L4 9l5-5"/><path d="M4 9h10a6 6 0 010 12h-3"/>',
  redo: '<path d="M15 14l5-5-5-5"/><path d="M20 9H10a6 6 0 000 12h3"/>',
  eye: '<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z"/><circle cx="12" cy="12" r="2.8"/>',
  pen: '<path d="M4 20l1-4L16 5l3 3L8 19z"/><path d="M14 7l3 3"/>',
  clock: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>',
  check: '<path d="M5 12.5l4.5 4.5L19 7.5"/>',
  send: '<path d="M4 12l16-8-6 16-2.5-6.5z"/><path d="M11.5 13.5L20 4"/>',
  back: '<path d="M15 5l-7 7 7 7"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
  close: '<path d="M6 6l12 12M18 6L6 18"/>',
  panel: '<rect x="3.5" y="4.5" width="17" height="15" rx="2"/><path d="M14.5 4.5v15"/>',
  search: '<circle cx="11" cy="11" r="6.5"/><path d="M16 16l4.5 4.5"/>',
  external: '<path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 01-1 1H5a1 1 0 01-1-1V7a1 1 0 011-1h5"/>',
  wide: '<path d="M3 12h18M7 8l-4 4 4 4M17 8l4 4-4 4"/>',
  narrow:
    '<path d="M8 12H3M21 12h-5M5.5 9.5L8 12l-2.5 2.5M18.5 9.5L16 12l2.5 2.5M11 5v14M13 5v14"/>',
};

@Component({
  selector: 'app-article-icon',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `<svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    [innerHTML]="markup()"
  ></svg>`,
  styles: `
    :host {
      display: inline-flex;
      width: 1.15rem;
      height: 1.15rem;
      flex: none;
    }
    svg {
      width: 100%;
      height: 100%;
    }
  `,
})
export class ArticleIcon {
  readonly name = input.required<string>();
  private readonly sanitizer = inject(DomSanitizer);
  // The markup is this file's own constants, never user input.
  protected readonly markup = computed<SafeHtml>(() =>
    this.sanitizer.bypassSecurityTrustHtml(ICONS[this.name()] ?? ''),
  );
}

/** The raw markup, for the editor's node views, which are plain DOM. */
export function iconSvg(name: string): string {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICONS[name] ?? ''}</svg>`;
}
