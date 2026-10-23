import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { DatePipe, Location } from '@angular/common';
import { Router, RouterLink } from '@angular/router';

import { ARTICLE_DESK, ArticleInput } from '../../../core/api/articles';
import { SessionStore } from '../../../core/auth/session-store';
import { Article, isLive } from '../../../core/models/articles';
import { Seo } from '../../../core/seo/seo';
import { messageFor } from '../../../shared/forms/submit';
import { ConfirmDialog } from '../../../shared/ui/confirm-dialog';
import { ErrorState, Loading } from '../../../shared/ui/states';
import { ArticleBody } from '../doc/article-body';
import {
  ArticleDoc,
  FigureAttrs,
  TacticAttrs,
  VideoAttrs,
  emptyArticleDoc,
  isEmptyDoc,
  safeImage,
  wordCount,
} from '../doc/article-doc';
import { ArticleIcon } from '../ui/article-icon';
import { ArticleCanvas } from './article-canvas';
import { ArticleToolbar, InsertKind } from './article-toolbar';
import { DialogRequest, DialogResult, InsertDialog } from './insert-dialog';
import { PublishPanel } from './publish-panel';
import type { EmbedHooks } from './tiptap-setup';

/** How long typing has to pause before the draft is saved. */
const AUTOSAVE_MS = 1500;

const BLANK_FIGURE: FigureAttrs = { src: '', alt: '', caption: '', credit: '', size: 'column' };

/**
 * Writing and reviewing one article.
 *
 * There is no Save button: a draft saves itself a moment after the writer
 * stops typing, and before anything that hands it on -- submitting,
 * publishing -- so what is reviewed is always what was written. A new article
 * is created on its first save and the address moves to its id without a
 * navigation, so the editor and the cursor stay where they are.
 *
 * Whether the text can be edited follows the API's rule: an administrator
 * always, a writer only while it is a draft or has been sent back.
 */
@Component({
  selector: 'app-article-editor',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    RouterLink,
    DatePipe,
    ArticleBody,
    ArticleCanvas,
    ArticleToolbar,
    ArticleIcon,
    PublishPanel,
    InsertDialog,
    ConfirmDialog,
    Loading,
    ErrorState,
  ],
  templateUrl: './article-editor.html',
  styleUrl: './article-editor.css',
  host: {
    '(window:beforeunload)': 'onBeforeUnload($event)',
    '(document:keydown)': 'onKey($event)',
  },
})
export class ArticleEditor {
  /** Absent on /desk/new. */
  readonly id = input<string>();

  private readonly desk = inject(ARTICLE_DESK);
  private readonly location = inject(Location);
  private readonly router = inject(Router);
  protected readonly session = inject(SessionStore);

  private readonly canvas = viewChild(ArticleCanvas);

  protected readonly article = signal<Article | null>(null);
  protected readonly loading = signal(false);
  protected readonly loadError = signal<string | null>(null);

  // The working copy.
  protected readonly title = signal('');
  protected readonly subtitle = signal('');
  protected readonly coverUrl = signal('');
  protected readonly coverCaption = signal('');
  protected readonly tags = signal<string[]>([]);
  protected readonly body = signal<ArticleDoc>(emptyArticleDoc());
  /** What the canvas starts from. Null until there is something to start from. */
  protected readonly initialBody = signal<ArticleDoc | null>(null);

  protected readonly dirty = signal(false);
  protected readonly saving = signal(false);
  protected readonly savedAt = signal<Date | null>(null);
  protected readonly saveError = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly notice = signal<string | null>(null);
  protected readonly actionError = signal<string | null>(null);
  protected readonly mode = signal<'write' | 'preview'>('write');
  protected readonly panelOpen = signal(true);
  protected readonly confirmingDelete = signal(false);

  protected readonly dialogRequest = signal<DialogRequest | null>(null);
  private resolveDialog: ((r: DialogResult | null) => void) | null = null;

  private timer: ReturnType<typeof setTimeout> | null = null;
  /** Bumped on every edit, so a save knows whether it caught the latest. */
  private edits = 0;
  private inFlight: Promise<void> | null = null;

  protected readonly history = computed(() => {
    this.version();
    const e = this.editor();
    return { undo: !!e?.isEditable && e.can().undo(), redo: !!e?.isEditable && e.can().redo() };
  });
  protected readonly isLive = computed(() => {
    const a = this.article();
    return !!a && isLive(a);
  });
  protected readonly words = computed(() => wordCount(this.body()));
  protected readonly minutes = computed(() => Math.max(1, Math.ceil(this.words() / 230)));
  protected readonly cover = computed(() => safeImage(this.coverUrl()));
  protected readonly editor = computed(() => this.canvas()?.editor() ?? null);
  protected readonly version = computed(() => this.canvas()?.version() ?? 0);

  protected readonly canEdit = computed(() => {
    const a = this.article();
    if (!a || this.session.isAdmin()) return true;
    return a.status === 'draft' || a.status === 'changes_requested';
  });

  protected readonly lockReason = computed(() => {
    const a = this.article();
    if (!a || this.canEdit()) return null;
    return a.status === 'in_review'
      ? 'This article is waiting for review, so it is locked.'
      : 'This article is published. Only an administrator can change it now.';
  });

  protected readonly saveLabel = computed(() => {
    if (this.saveError()) return 'Not saved';
    if (this.saving()) return 'Saving…';
    if (this.dirty()) return 'Unsaved changes';
    const at = this.savedAt();
    return at ? 'Saved' : this.article() ? 'All changes saved' : 'New draft';
  });

  protected readonly hooks: EmbedHooks = {
    editFigure: async (attrs) => this.figureFrom(await this.ask({ kind: 'image', attrs })),
    editTactic: async (attrs) => this.tacticFrom(await this.ask({ kind: 'board', attrs })),
    editVideo: async (attrs) => this.videoFrom(await this.ask({ kind: 'video', attrs })),
  };

  constructor() {
    const seo = inject(Seo);
    effect(() => {
      seo.describe({
        title: this.title() ? `Editing: ${this.title()}` : 'New article',
        description: 'The ScoutPulse editorial desk.',
        path: '/desk',
        noindex: true,
      });
    });

    effect(() => {
      const id = this.id();
      untracked(() => {
        if (id) void this.load(id);
        else this.initialBody.set(emptyArticleDoc());
      });
    });

    inject(DestroyRef).onDestroy(() => {
      if (this.timer) clearTimeout(this.timer);
    });
  }

  private async load(id: string): Promise<void> {
    this.loading.set(true);
    this.loadError.set(null);
    try {
      const a = await this.desk.byId(id);
      this.article.set(a);
      this.title.set(a.title);
      this.subtitle.set(a.subtitle);
      this.coverUrl.set(a.cover_url);
      this.coverCaption.set(a.cover_caption);
      this.tags.set(a.tags);
      const body = a.body?.type === 'doc' ? a.body : emptyArticleDoc();
      this.body.set(body);
      this.initialBody.set(body);
    } catch (e) {
      this.loadError.set(messageFor(e, 'Could not open this article.'));
    } finally {
      this.loading.set(false);
    }
  }

  // --- editing ----------------------------------------------------------------

  protected value(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }

  protected edited(): void {
    if (!this.canEdit()) return;
    this.edits++;
    this.dirty.set(true);
    this.notice.set(null);
    if (this.timer) clearTimeout(this.timer);
    this.timer = setTimeout(() => void this.save(), AUTOSAVE_MS);
  }

  protected onBody(doc: ArticleDoc): void {
    this.body.set(doc);
    this.edited();
  }

  protected setTitle(value: string): void {
    // A headline is one line; Enter moves on to the standfirst.
    this.title.set(value.replace(/\s*\n\s*/g, ' '));
    this.edited();
  }

  protected setTags(tags: string[]): void {
    this.tags.set(tags);
    this.edited();
  }

  protected autoGrow(e: Event): void {
    const el = e.target as HTMLTextAreaElement;
    el.style.height = 'auto';
    el.style.height = `${el.scrollHeight}px`;
  }

  protected toBody(e: Event): void {
    e.preventDefault();
    this.canvas()?.focus();
  }

  private input(): ArticleInput {
    return {
      title: this.title(),
      subtitle: this.subtitle(),
      cover_url: this.coverUrl(),
      cover_caption: this.coverCaption(),
      body: this.body(),
      tags: this.tags(),
    };
  }

  /**
   * Saves the working copy. One save at a time: a second request while one is
   * in flight waits for it and then saves whatever changed meanwhile.
   */
  protected async save(): Promise<void> {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    if (this.inFlight) {
      await this.inFlight;
      if (!this.dirty()) return;
    }
    if (!this.dirty() || !this.canEdit()) return;

    const a = this.article();
    // Nothing worth creating a draft for yet.
    if (
      !a &&
      !this.title().trim() &&
      !this.subtitle().trim() &&
      !this.coverUrl() &&
      isEmptyDoc(this.body())
    ) {
      this.dirty.set(false);
      return;
    }

    const seen = this.edits;
    this.saving.set(true);
    this.saveError.set(null);
    this.inFlight = (async () => {
      try {
        const saved = a
          ? await this.desk.update(a.id, this.input())
          : await this.desk.create(this.input());
        this.article.set(saved);
        if (!a) this.location.replaceState(`/desk/${saved.id}`);
        this.savedAt.set(new Date());
        if (this.edits === seen) this.dirty.set(false);
      } catch (e) {
        this.saveError.set(
          messageFor(e, 'Could not save. Your text is still here; it will retry when you type.'),
        );
      } finally {
        this.saving.set(false);
        this.inFlight = null;
      }
    })();
    await this.inFlight;
    // Edits made while that save was on the wire.
    if (this.dirty() && !this.saveError() && !this.timer) {
      this.timer = setTimeout(() => void this.save(), AUTOSAVE_MS);
    }
  }

  // --- the workflow -------------------------------------------------------------

  private async act(run: (id: string) => Promise<Article>, done: string): Promise<void> {
    await this.save();
    const a = this.article();
    if (!a || this.saveError()) return;
    this.busy.set(true);
    this.actionError.set(null);
    try {
      const next = await run(a.id);
      this.article.set(next);
      this.notice.set(done);
    } catch (e) {
      this.actionError.set(messageFor(e, 'That did not work. Try again.'));
    } finally {
      this.busy.set(false);
    }
  }

  protected submit(publishAt: string | null): void {
    const admin = this.session.isAdmin();
    const later = !!publishAt;
    void this.act(
      (id) => this.desk.submit(id, publishAt),
      admin
        ? later
          ? 'Scheduled. It will go live on its own at that time.'
          : 'Published. It is live in the Analysis section.'
        : 'Sent for review. An administrator will publish it or send it back.',
    );
  }

  protected requestChanges(note: string): void {
    void this.act(
      (id) => this.desk.requestChanges(id, note),
      'Sent back to the writer with your note.',
    );
  }

  protected withdraw(): void {
    const status = this.article()?.status;
    void this.act(
      (id) => this.desk.withdraw(id),
      status === 'published'
        ? 'Unpublished. It is a draft again.'
        : 'Withdrawn. You can edit it again.',
    );
  }

  protected async confirmDelete(): Promise<void> {
    const a = this.article();
    if (!a) return;
    this.busy.set(true);
    try {
      await this.desk.remove(a.id);
      this.dirty.set(false);
      void this.router.navigate(['/desk']);
    } catch (e) {
      this.actionError.set(messageFor(e, 'Could not delete the article.'));
    } finally {
      this.busy.set(false);
    }
  }

  // --- dialogs ----------------------------------------------------------------

  private ask(request: DialogRequest): Promise<DialogResult | null> {
    this.resolveDialog?.(null);
    this.dialogRequest.set(request);
    return new Promise((resolve) => (this.resolveDialog = resolve));
  }

  protected onDialog(result: DialogResult | null): void {
    const resolve = this.resolveDialog;
    this.resolveDialog = null;
    this.dialogRequest.set(null);
    resolve?.(result);
  }

  private figureFrom(r: DialogResult | null): FigureAttrs | null {
    return r && (r.kind === 'image' || r.kind === 'cover') ? r.attrs : null;
  }
  private tacticFrom(r: DialogResult | null): TacticAttrs | null {
    return r?.kind === 'board' ? r.attrs : null;
  }
  private videoFrom(r: DialogResult | null): VideoAttrs | null {
    return r?.kind === 'video' ? r.attrs : null;
  }

  protected async insert(kind: InsertKind): Promise<void> {
    const editor = this.editor();
    if (!editor || !this.canEdit()) return;

    if (kind === 'link') {
      const href = (editor.getAttributes('link')['href'] as string | undefined) ?? '';
      const askText = editor.state.selection.empty && !editor.isActive('link');
      const r = await this.ask({ kind: 'link', href, askText });
      if (r?.kind !== 'link') return;
      const { applyLink } = await import('./tiptap-setup');
      applyLink(editor, r.href, r.text);
      return;
    }

    const node =
      kind === 'image'
        ? this.figureFrom(await this.ask({ kind: 'image', attrs: BLANK_FIGURE }))
        : kind === 'board'
          ? this.tacticFrom(
              await this.ask({ kind: 'board', attrs: { slug: '', title: '', caption: '' } }),
            )
          : this.videoFrom(
              await this.ask({
                kind: 'video',
                attrs: { provider: 'youtube', id: '', caption: '' },
              }),
            );
    if (!node) return;
    const type = kind === 'image' ? 'figure' : kind === 'board' ? 'tacticBoard' : 'video';
    const { insertEmbed } = await import('./tiptap-setup');
    insertEmbed(editor, type, node);
  }

  protected undo(): void {
    this.editor()?.chain().focus().undo().run();
  }

  protected redo(): void {
    this.editor()?.chain().focus().redo().run();
  }

  protected async editCover(): Promise<void> {
    if (!this.canEdit()) return;
    const r = this.figureFrom(
      await this.ask({
        kind: 'cover',
        attrs: { ...BLANK_FIGURE, src: this.coverUrl(), caption: this.coverCaption() },
      }),
    );
    if (!r) return;
    this.coverUrl.set(r.src);
    this.coverCaption.set([r.caption, r.credit].filter(Boolean).join(' · '));
    this.edited();
  }

  protected removeCover(): void {
    this.coverUrl.set('');
    this.coverCaption.set('');
    this.edited();
  }

  // --- leaving and keys -----------------------------------------------------------

  canLeave(): boolean {
    if (!this.dirty() && !this.saving()) return true;
    void this.save();
    return confirm('Your latest changes are still saving. Leave anyway?');
  }

  protected onBeforeUnload(e: BeforeUnloadEvent): void {
    if (this.dirty() || this.saving()) {
      void this.save();
      e.preventDefault();
    }
  }

  protected onKey(e: KeyboardEvent): void {
    const mod = e.ctrlKey || e.metaKey;
    if (!mod) return;
    const key = e.key.toLowerCase();
    if (key === 's') {
      e.preventDefault();
      void this.save();
    } else if (key === 'k' && this.mode() === 'write' && this.editor()?.isFocused) {
      e.preventDefault();
      void this.insert('link');
    }
  }
}
