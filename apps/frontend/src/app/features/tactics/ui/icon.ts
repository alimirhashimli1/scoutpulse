import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';

/**
 * Line icons for the board's toolbars, drawn on a 24-unit grid in the
 * current text colour. Inline rather than an icon font: there are a few dozen,
 * they are specific to football, and they should inherit colour and size from
 * the button they sit in.
 */
const ICONS: Record<string, string> = {
  select: '<path d="M5 3l14 8-6 1.5L10 19z"/>',
  player:
    '<circle cx="12" cy="6.5" r="2.6"/><path d="M7.5 21v-7a4.5 4.5 0 019 0v7"/><ellipse cx="12" cy="21" rx="6" ry="1.2"/>',
  keeper:
    '<circle cx="12" cy="6.5" r="2.6"/><path d="M7.5 21v-7a4.5 4.5 0 019 0v7M5 11l2.5 2M19 11l-2.5 2"/>',
  ball: '<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5l3.6 2.6-1.4 4.2H9.8l-1.4-4.2z"/><path d="M12 3.5v4M20 10l-4.4.1M17 19l-2.8-4.7M7 19l2.8-4.7M4 10l4.4.1"/>',
  cone: '<path d="M12 3l5 15H7z"/><path d="M4 20h16M9.4 11h5.2"/>',
  disc: '<ellipse cx="12" cy="15" rx="8" ry="3"/><path d="M4 15c1-3 3.5-5 8-5s7 2 8 5"/>',
  pole: '<path d="M12 3v15"/><ellipse cx="12" cy="20" rx="4" ry="1.4"/>',
  mannequin:
    '<circle cx="12" cy="4.5" r="2"/><rect x="8.5" y="7.5" width="7" height="10" rx="3.5"/><path d="M12 17.5V21"/>',
  hurdle: '<path d="M5 9h14M7 9v10M17 9v10M4.5 19h5M14.5 19h5"/>',
  ladder: '<path d="M3 8h18M3 16h18M6 8v8M10 8v8M14 8v8M18 8v8"/>',
  ring: '<ellipse cx="12" cy="13" rx="8" ry="4.5"/>',
  minigoal:
    '<path d="M4 18V9h16v9M4 9l3-3h10l3 3"/><path d="M8 9v9M12 9v9M16 9v9M4 13.5h16" opacity=".45"/>',
  goal: '<path d="M2.5 18V7h19v11"/><path d="M2.5 7l3 3v8M21.5 7l-3 3v8M5.5 10h13" opacity=".55"/><path d="M8 10v8M11.5 10v8M15 10v8" opacity=".35"/>',
  popup: '<path d="M4 19a8 8 0 0116 0"/><path d="M7.5 19a4.5 6 0 019 0" opacity=".5"/>',
  flag: '<path d="M6 21V3l12 4-12 4"/>',
  arrow: '<path d="M4 20L19 5"/><path d="M11 5h8v8"/>',
  pass: '<path d="M4 20l2.5-2.5M9 15l2.5-2.5M14 10l2-2" /><path d="M12 5h7v7"/>',
  dribble: '<path d="M3 18c2-4 4 2 6-2s4 2 6-2 3-3 3-3"/><path d="M14 7h5v5"/>',
  line: '<path d="M4 20L20 4"/>',
  pen: '<path d="M4 20l1-4L16 5l3 3L8 19z"/><path d="M14 7l3 3"/>',
  rect: '<rect x="4" y="6" width="16" height="12" rx="1"/>',
  ellipse: '<ellipse cx="12" cy="12" rx="9" ry="6.5"/>',
  polygon: '<path d="M5 8l6-4 8 5-2 10H7z"/>',
  text: '<path d="M5 6V4h14v2M12 4v16M9 20h6"/>',
  undo: '<path d="M9 14L4 9l5-5"/><path d="M4 9h10a6 6 0 010 12h-3"/>',
  redo: '<path d="M15 14l5-5-5-5"/><path d="M20 9H10a6 6 0 000 12h3"/>',
  trash: '<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>',
  copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 00-1-1H5a1 1 0 00-1 1v10a1 1 0 001 1h3"/>',
  play: '<path d="M7 4.5v15l12-7.5z" fill="currentColor"/>',
  pause: '<path d="M7 4h3.5v16H7zM13.5 4H17v16h-3.5z" fill="currentColor"/>',
  restart: '<path d="M4 12a8 8 0 108-8 8 8 0 00-5.7 2.3L4 8.5"/><path d="M4 3.5v5h5"/>',
  prev: '<path d="M6 5v14M18 5L9 12l9 7z"/>',
  next: '<path d="M18 5v14M6 5l9 7-9 7z"/>',
  earlier: '<path d="M15 5l-7 7 7 7"/>',
  later: '<path d="M9 5l7 7-7 7"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  link: '<path d="M10 14a4 4 0 005.7 0l3-3a4 4 0 00-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 00-5.7 0l-3 3a4 4 0 005.7 5.7l1-1"/>',
  download: '<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>',
  expand: '<path d="M4 9V4h5M20 9V4h-5M4 15v5h5M20 15v5h-5"/>',
  camTop:
    '<rect x="5" y="5" width="14" height="14" rx="1"/><path d="M12 5v14M5 12h14" opacity=".5"/>',
  camTilt: '<path d="M7 7h10l4 12H3z"/><path d="M12 7v12" opacity=".5"/>',
  camSide: '<path d="M2 17h20M4 17l3-7h10l3 7"/><circle cx="12" cy="6" r="1.6"/>',
  camBehind: '<path d="M3 19l6-12h6l6 12z"/><path d="M9.5 7h5v-2h-5z"/>',
  spot: '<path d="M9 3h6l-1 4h-4z"/><path d="M10 7l-5 13h14L14 7" opacity=".55"/>',
  air: '<path d="M3 18c4-14 14-14 18 0"/><circle cx="18" cy="12" r="1.5" fill="currentColor"/>',
  close: '<path d="M6 6l12 12M18 6L6 18"/>',
};

@Component({
  selector: 'app-tactic-icon',
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
      width: 1.25rem;
      height: 1.25rem;
      flex: none;
    }
    svg {
      width: 100%;
      height: 100%;
    }
  `,
})
export class TacticIcon {
  readonly name = input.required<string>();
  private readonly sanitizer = inject(DomSanitizer);
  // The markup is this file's own constants, never user input.
  protected readonly markup = computed<SafeHtml>(() =>
    this.sanitizer.bypassSecurityTrustHtml(ICONS[this.name()] ?? ''),
  );
}
