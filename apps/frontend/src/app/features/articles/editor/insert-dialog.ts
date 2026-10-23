import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  output,
  resource,
  signal,
  viewChild,
} from '@angular/core';
import { DatePipe, NgTemplateOutlet } from '@angular/common';

import { SEARCH_READER } from '../../../core/api/contracts';
import { MAX_PAGE_SIZE } from '../../../core/api/page';
import { TACTIC_READER } from '../../../core/api/tactics';
import { SearchResult } from '../../../core/models/football';
import {
  FigureAttrs,
  TacticAttrs,
  VideoAttrs,
  normaliseLink,
  parseTacticSlug,
  parseVideo,
  safeImage,
  videoPoster,
} from '../doc/article-doc';
import { ArticleIcon } from '../ui/article-icon';

export type DialogRequest =
  | { kind: 'link'; href: string; askText: boolean }
  | { kind: 'image' | 'cover'; attrs: FigureAttrs }
  | { kind: 'board'; attrs: TacticAttrs }
  | { kind: 'video'; attrs: VideoAttrs };

export type DialogResult =
  | { kind: 'link'; href: string | null; text: string }
  | { kind: 'image' | 'cover'; attrs: FigureAttrs }
  | { kind: 'board'; attrs: TacticAttrs }
  | { kind: 'video'; attrs: VideoAttrs };

const KIND_PATH: Record<SearchResult['kind'], string> = {
  player: '/players/',
  team: '/clubs/',
  coach: '/coaches/',
  league: '/competitions/',
};
const KIND_LABEL: Record<SearchResult['kind'], string> = {
  player: 'Player',
  team: 'Club',
  coach: 'Coach',
  league: 'Competition',
};

/**
 * The editor's one dialog: a link, an image, the cover, a tactics board or a
 * video, depending on what was asked for.
 *
 * Built on `<dialog>` like ConfirmDialog, for the focus trap and Escape. Each
 * form checks its address with the same rules the reader and the API use,
 * and says what is wrong in words before anything reaches the article.
 */
@Component({
  selector: 'app-insert-dialog',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DatePipe, NgTemplateOutlet, ArticleIcon],
  templateUrl: './insert-dialog.html',
  styleUrl: './insert-dialog.css',
})
export class InsertDialog {
  readonly request = input<DialogRequest | null>(null);
  readonly done = output<DialogResult | null>();

  private readonly search = inject(SEARCH_READER);
  private readonly tactics = inject(TACTIC_READER);
  private readonly dialog = viewChild<ElementRef<HTMLDialogElement>>('dialog');

  protected readonly kindLabel = KIND_LABEL;

  // One set of fields, reused by whichever form is showing.
  protected readonly url = signal('');
  protected readonly text = signal('');
  protected readonly alt = signal('');
  protected readonly caption = signal('');
  protected readonly credit = signal('');
  protected readonly wide = signal(false);
  protected readonly boardFilter = signal('');
  protected readonly boardSlug = signal('');
  protected readonly boardTitle = signal('');
  protected readonly preview = signal<'idle' | 'loading' | 'ok' | 'failed'>('idle');
  protected readonly touched = signal(false);

  protected readonly kind = computed(() => this.request()?.kind ?? null);

  // --- link -----------------------------------------------------------------

  /** Searches the site when what was typed is not an address. */
  private readonly query = signal('');
  protected readonly hits = resource({
    params: () => (this.kind() === 'link' && this.query().length >= 2 ? this.query() : undefined),
    loader: async ({ params, abortSignal }) => {
      await new Promise((resolve) => setTimeout(resolve, 220));
      if (abortSignal.aborted) return [];
      return (await this.search.search(params, undefined, { limit: 6 })).items;
    },
  });
  protected readonly linkTarget = computed(() => normaliseLink(this.url()));
  protected readonly looksLikeAddress = computed(() => /[./:@]/.test(this.url().trim()));

  // --- image ----------------------------------------------------------------

  protected readonly imageSrc = computed(() => safeImage(this.url().trim()));

  // --- board ----------------------------------------------------------------

  protected readonly boards = resource({
    params: () => (this.kind() === 'board' ? true : undefined),
    loader: async () => (await this.tactics.list({ limit: MAX_PAGE_SIZE })).items,
  });
  protected readonly filteredBoards = computed(() => {
    const q = this.boardFilter().trim().toLowerCase();
    const all = this.boards.value() ?? [];
    return q ? all.filter((b) => b.title.toLowerCase().includes(q)) : all;
  });

  // --- video ----------------------------------------------------------------

  protected readonly video = computed(() => parseVideo(this.url()));
  protected readonly videoPoster = computed(() => {
    const v = this.video();
    return v ? videoPoster(v.provider, v.id) : null;
  });

  protected readonly error = computed(() => {
    const url = this.url().trim();
    switch (this.kind()) {
      case 'link':
        return url && this.looksLikeAddress() && !this.linkTarget()
          ? 'That is not an address an article can link to. Use a web address, an email, or a page on this site.'
          : null;
      case 'image':
      case 'cover':
        if (!url) return null;
        if (!this.imageSrc()) return 'Images must come from an https:// address.';
        return this.preview() === 'failed'
          ? 'No image loaded from that address. Use the address of the image itself (right-click it, then “Copy image address”).'
          : null;
      case 'video':
        return url && !this.video() ? 'Paste a YouTube or Vimeo link.' : null;
      case 'board':
        return this.touched() && !this.boardSlug()
          ? 'Choose a board, or paste a board’s link.'
          : null;
    }
    return null;
  });

  protected readonly canApply = computed(() => {
    switch (this.kind()) {
      case 'link':
        return !!this.linkTarget();
      case 'image':
      case 'cover':
        return !!this.imageSrc() && this.preview() !== 'failed';
      case 'video':
        return !!this.video();
      case 'board':
        return !!this.boardSlug();
    }
    return false;
  });

  constructor() {
    // Each request starts its form from what it was given.
    effect(() => {
      const r = this.request();
      const el = this.dialog()?.nativeElement;
      if (!el) return;
      if (!r) {
        if (el.open) el.close();
        return;
      }
      this.reset(r);
      if (!el.open) el.showModal();
    });
  }

  private reset(r: DialogRequest): void {
    this.touched.set(false);
    this.boardFilter.set('');
    this.text.set('');
    this.alt.set('');
    this.credit.set('');
    this.wide.set(false);
    this.boardSlug.set('');
    this.boardTitle.set('');
    this.query.set('');
    switch (r.kind) {
      case 'link':
        this.url.set(r.href);
        this.caption.set('');
        break;
      case 'image':
      case 'cover':
        this.url.set(r.attrs.src);
        this.alt.set(r.attrs.alt);
        this.caption.set(r.attrs.caption);
        this.credit.set(r.attrs.credit);
        this.wide.set(r.attrs.size === 'wide');
        this.preview.set(r.attrs.src ? 'loading' : 'idle');
        break;
      case 'board':
        this.url.set('');
        this.boardSlug.set(r.attrs.slug);
        this.boardTitle.set(r.attrs.title);
        this.caption.set(r.attrs.caption);
        break;
      case 'video':
        this.url.set(
          r.attrs.id
            ? r.attrs.provider === 'youtube'
              ? `https://youtu.be/${r.attrs.id}`
              : `https://vimeo.com/${r.attrs.id}`
            : '',
        );
        this.caption.set(r.attrs.caption);
        break;
    }
  }

  protected value(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }

  protected onUrl(value: string): void {
    this.url.set(value);
    if (this.kind() === 'link') this.query.set(this.looksLikeAddress() ? '' : value.trim());
    if (this.kind() === 'image' || this.kind() === 'cover') {
      this.preview.set(safeImage(value.trim()) ? 'loading' : 'idle');
    }
    if (this.kind() === 'board') {
      const slug = parseTacticSlug(value);
      if (slug) {
        this.boardSlug.set(slug);
        this.boardTitle.set(this.boards.value()?.find((b) => b.slug === slug)?.title ?? '');
      }
    }
  }

  protected pickHit(hit: SearchResult): void {
    this.url.set(KIND_PATH[hit.kind] + hit.id);
    this.query.set('');
    if (!this.text()) this.text.set(hit.name);
  }

  protected pickBoard(slug: string, title: string): void {
    this.boardSlug.set(slug);
    this.boardTitle.set(title);
  }

  protected apply(): void {
    this.touched.set(true);
    const r = this.request();
    if (!r || !this.canApply()) return;
    switch (r.kind) {
      case 'link':
        this.finish({
          kind: 'link',
          href: this.linkTarget(),
          text: this.text().trim() || this.url().trim(),
        });
        break;
      case 'image':
      case 'cover':
        this.finish({
          kind: r.kind,
          attrs: {
            src: this.imageSrc()!,
            alt: this.alt().trim(),
            caption: this.caption().trim(),
            credit: this.credit().trim(),
            size: this.wide() ? 'wide' : 'column',
          },
        });
        break;
      case 'board':
        this.finish({
          kind: 'board',
          attrs: {
            slug: this.boardSlug(),
            title: this.boardTitle(),
            caption: this.caption().trim(),
          },
        });
        break;
      case 'video': {
        const v = this.video()!;
        this.finish({ kind: 'video', attrs: { ...v, caption: this.caption().trim() } });
        break;
      }
    }
  }

  protected removeLink(): void {
    this.finish({ kind: 'link', href: null, text: '' });
  }

  protected cancel(): void {
    this.finish(null);
  }

  protected onBackdrop(e: MouseEvent): void {
    if (e.target === this.dialog()?.nativeElement) this.cancel();
  }

  /**
   * Closes at once, before answering. Waiting for the request to clear would
   * leave the dialog open until the next change detection, and a key pressed
   * in that moment would land in a form that is already finished with.
   */
  private finish(result: DialogResult | null): void {
    const el = this.dialog()?.nativeElement;
    if (el?.open) el.close();
    this.done.emit(result);
  }
}
