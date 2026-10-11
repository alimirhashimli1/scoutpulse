import {
  ChangeDetectionStrategy,
  Component,
  DOCUMENT,
  OnInit,
  PLATFORM_ID,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { Location, isPlatformBrowser } from '@angular/common';
import { Router, RouterLink } from '@angular/router';

import { TACTIC_READER, TACTIC_WRITER } from '../../../core/api/tactics';
import { SessionStore } from '../../../core/auth/session-store';
import { Seo } from '../../../core/seo/seo';
import { messageFor } from '../../../shared/forms/submit';
import { BoardView } from '../board/board-view';
import type { CameraView, EditState } from '../board/engine';
import { TacticDoc, normalizeDoc } from '../board/model';
import { BoardPlayer, recordVideo, videoExtension } from '../board/player';
import { TacticIcon } from '../ui/icon';
import { PlaybackBar } from '../ui/playback-bar';
import { EditorStore } from './editor-store';
import { Inspector } from './inspector';
import { ToolPalette } from './tool-palette';

/** Where a signed-out visitor's board waits while they sign in. */
const DRAFT_KEY = 'scoutpulse:tactics-draft';

const VIEWS: { value: CameraView; label: string; icon: string }[] = [
  { value: 'top', label: 'From above', icon: 'camTop' },
  { value: 'tactical', label: 'Tactical camera', icon: 'camTilt' },
  { value: 'broadcast', label: 'Broadcast camera', icon: 'camSide' },
  { value: 'behind', label: 'Behind the goal', icon: 'camBehind' },
];

/**
 * The tactics board editor.
 *
 * Open to everyone. A signed-in member saves to their own list and gets a
 * link to send; a visitor without an account can build and play a board in
 * full, but it lives only in this page — refreshing loses it, which the page
 * says up front. The one courtesy is that "sign in to save" carries the board
 * through the sign-in, so asking to keep it is not what loses it.
 */
@Component({
  selector: 'app-tactics-editor',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, BoardView, ToolPalette, Inspector, PlaybackBar, TacticIcon],
  providers: [EditorStore],
  templateUrl: './tactics-editor.html',
  styleUrls: ['./tactics-editor.css', '../ui/stage.css'],
  host: {
    '(document:keydown)': 'onKey($event)',
    '(window:beforeunload)': 'onBeforeUnload($event)',
  },
})
export class TacticsEditor implements OnInit {
  /** Present when editing a saved board; absent for a new one. */
  readonly slug = input<string>();

  protected readonly store = inject(EditorStore);
  protected readonly session = inject(SessionStore);
  private readonly reader = inject(TACTIC_READER);
  private readonly writer = inject(TACTIC_WRITER);
  private readonly router = inject(Router);
  private readonly location = inject(Location);
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  protected readonly views = VIEWS;
  protected readonly view = signal<CameraView>('tactical');
  protected readonly title = signal('');
  protected readonly description = signal('');
  protected readonly savedSlug = signal<string | null>(null);
  protected readonly loading = signal(false);
  protected readonly loadError = signal<string | null>(null);
  protected readonly saving = signal(false);
  protected readonly saveError = signal<string | null>(null);
  protected readonly copied = signal(false);
  protected readonly recording = signal(false);
  protected readonly showDetails = signal(false);

  protected readonly player = new BoardPlayer(this.store.doc);
  protected readonly tick = (dt: number) => this.player.tick(dt);
  private readonly board = viewChild(BoardView);

  /** Standing on a step, rather than playing or scrubbed between two. */
  protected readonly atRest = computed(() => {
    if (this.player.playing()) return false;
    const s = this.player.step();
    const start = this.player.timeline().stepStarts[s] ?? 0;
    const hold = this.store.doc().steps[s]?.hold ?? 0;
    const t = this.player.time();
    return t >= start - 1e-6 && t <= start + hold + 1e-6;
  });

  protected readonly frame = computed(() =>
    this.atRest() ? this.store.stillFrame() : this.player.frame(),
  );

  protected readonly editState = computed<EditState>(() => ({
    editor: true,
    editing: this.atRest() && !this.recording(),
    step: this.store.stepIndex(),
    selection: this.store.selection(),
    trails: true,
  }));

  protected readonly caption = computed(() => this.frame().note);
  protected readonly shareUrl = computed(() => {
    const slug = this.savedSlug();
    return slug ? `${this.document.location?.origin ?? ''}/tactics/${slug}` : '';
  });
  protected readonly canSave = computed(() => this.session.isAuthenticated());

  constructor() {
    inject(Seo).describe({
      title: 'Tactics board',
      description: 'Draw and animate drills, shapes and match analysis on a 3D pitch.',
      path: '/tactics/new',
      noindex: true,
    });

    // Keep the editor's step and the playback clock on the same step. A step
    // picked in the editor moves the clock; scrubbing moves the editor.
    effect(() => {
      const i = this.store.stepIndex();
      untracked(() => {
        if (!this.player.playing() && this.player.step() !== i) this.player.seekStep(i);
      });
    });
    //
    // Only the clock moving may move the editor. Reading player.step() here
    // used to make a timing edit count too: changing a step's duration shifts
    // every later step's start, the clock stayed where it was, and the editor
    // was carried off to whichever step now covered that moment — or left
    // between two steps, where editing is off.
    effect(() => {
      this.player.time();
      const playing = this.player.playing();
      untracked(() => {
        const s = this.player.step();
        if (!playing && this.store.stepIndex() !== s) this.store.goToStep(s);
      });
    });
    // And when the timeline itself changes — a step added, removed or retimed —
    // the clock goes back to the start of the step being edited, so the board
    // is never left stranded between steps by an edit.
    effect(() => {
      this.player.timeline();
      untracked(() => {
        if (!this.player.playing()) this.player.seekStep(this.store.stepIndex());
      });
    });
    // Starting playback drops the selection and any half-drawn shape.
    effect(() => {
      if (this.player.playing()) {
        untracked(() => {
          this.store.selection.set(null);
          this.store.preview.set(null);
        });
      }
    });
  }

  async ngOnInit(): Promise<void> {
    const slug = this.slug();
    if (slug) {
      await this.load(slug);
      return;
    }
    this.restoreDraft();
  }

  private async load(slug: string): Promise<void> {
    this.loading.set(true);
    try {
      const board = await this.reader.bySlug(slug);
      if (board.owner_id !== this.session.user()?.id) {
        this.loadError.set(
          'Only the member who made this board can edit it. You can still watch it.',
        );
        return;
      }
      this.title.set(board.title);
      this.description.set(board.description);
      this.store.load(normalizeDoc(board.content));
      this.savedSlug.set(board.slug);
    } catch (error) {
      this.loadError.set(messageFor(error, 'Could not open this board.'));
    } finally {
      this.loading.set(false);
    }
  }

  private restoreDraft(): void {
    if (!this.isBrowser) return;
    try {
      const raw = sessionStorage.getItem(DRAFT_KEY);
      if (!raw) return;
      sessionStorage.removeItem(DRAFT_KEY);
      const draft = JSON.parse(raw) as { title?: string; description?: string; doc?: unknown };
      this.title.set(draft.title ?? '');
      this.description.set(draft.description ?? '');
      this.store.load(normalizeDoc(draft.doc));
      // Not saved yet: it came through sign-in precisely to be saved.
      this.store.dirty.set(true);
    } catch {
      // Storage blocked or the draft unreadable: start blank, as on any visit.
    }
  }

  // --- saving -------------------------------------------------------------

  protected async save(): Promise<void> {
    if (!this.canSave()) {
      this.signInToSave();
      return;
    }
    this.saving.set(true);
    this.saveError.set(null);
    const input = {
      title: this.title().trim() || 'Untitled board',
      description: this.description().trim(),
      content: this.store.doc() as TacticDoc,
    };
    try {
      const slug = this.savedSlug();
      const board = slug ? await this.writer.update(slug, input) : await this.writer.create(input);
      this.title.set(board.title);
      this.store.markSaved();
      if (!slug) {
        this.savedSlug.set(board.slug);
        // The address becomes the board's own without reloading the editor,
        // which would cost the undo history for nothing.
        this.location.replaceState(`/tactics/${board.slug}/edit`);
      }
    } catch (error) {
      this.saveError.set(messageFor(error, 'Could not save the board. Please try again.'));
    } finally {
      this.saving.set(false);
    }
  }

  protected signInToSave(): void {
    try {
      sessionStorage.setItem(
        DRAFT_KEY,
        JSON.stringify({
          title: this.title(),
          description: this.description(),
          doc: this.store.doc(),
        }),
      );
    } catch {
      // Without storage the board cannot survive the sign-in; it is still
      // better to let them sign in than to block it.
    }
    this.store.markSaved();
    void this.router.navigate(['/login'], { queryParams: { returnTo: '/tactics/new' } });
  }

  protected async copyLink(): Promise<void> {
    try {
      await navigator.clipboard.writeText(this.shareUrl());
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      // Clipboard refused: the link is on screen to copy by hand.
    }
  }

  protected async downloadVideo(): Promise<void> {
    const canvas = this.board()?.canvas();
    if (!canvas || this.recording()) return;
    this.recording.set(true);
    try {
      const blob = await recordVideo(canvas, this.player);
      const a = this.document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `${fileName(this.title())}.${videoExtension(blob)}`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 10_000);
    } finally {
      this.recording.set(false);
      this.player.pause();
      this.player.seekStep(this.store.stepIndex());
    }
  }

  // --- steps --------------------------------------------------------------

  protected goToStep(i: number): void {
    this.player.pause();
    this.store.goToStep(i);
    this.player.seekStep(i);
  }

  protected addStep(): void {
    this.player.pause();
    this.store.addStep();
  }

  protected duplicateStep(): void {
    this.player.pause();
    this.store.duplicateStep();
  }

  protected moveStep(from: number, to: number): void {
    this.player.pause();
    this.store.moveStep(from, to);
  }

  /** The step chip being dragged to a new place, and the chip it is over. */
  protected readonly draggingStep = signal<number | null>(null);
  protected readonly dropStep = signal<number | null>(null);

  protected onStepDragStart(e: DragEvent, i: number): void {
    this.draggingStep.set(i);
    // Firefox starts no drag without data.
    e.dataTransfer?.setData('text/plain', String(i + 1));
    if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
  }

  protected onStepDragOver(e: DragEvent, i: number): void {
    if (this.draggingStep() === null) return;
    e.preventDefault();
    this.dropStep.set(i);
  }

  protected onStepDrop(e: DragEvent, i: number): void {
    const from = this.draggingStep();
    e.preventDefault();
    this.onStepDragEnd();
    if (from !== null) this.moveStep(from, i);
  }

  protected onStepDragEnd(): void {
    this.draggingStep.set(null);
    this.dropStep.set(null);
  }

  protected numberFrom(e: Event, min: number, max: number): number {
    const v = Number((e.target as HTMLInputElement).value);
    return Math.max(min, Math.min(max, Number.isFinite(v) ? v : min));
  }

  protected value(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }

  // --- leaving and shortcuts ----------------------------------------------

  canLeave(): boolean {
    if (!this.store.dirty()) return true;
    return confirm(
      this.canSave()
        ? 'This board has unsaved changes. Leave without saving?'
        : 'This board is not saved anywhere. Leave and lose it?',
    );
  }

  protected onBeforeUnload(e: BeforeUnloadEvent): void {
    if (this.store.dirty()) e.preventDefault();
  }

  protected onKey(e: KeyboardEvent): void {
    const target = e.target as HTMLElement | null;
    if (
      target &&
      (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
    )
      return;
    const mod = e.ctrlKey || e.metaKey;
    const key = e.key.toLowerCase();

    if (mod && key === 'z') {
      e.preventDefault();
      if (e.shiftKey) this.store.redo();
      else this.store.undo();
    } else if (mod && key === 'y') {
      e.preventDefault();
      this.store.redo();
    } else if (mod && key === 's') {
      e.preventDefault();
      void this.save();
    } else if (mod && key === 'd') {
      e.preventDefault();
      this.store.duplicateSelection();
    } else if (key === 'delete' || key === 'backspace') {
      if (this.store.selection()) {
        e.preventDefault();
        this.store.deleteSelection();
      }
    } else if (key === 'escape') {
      this.board()?.cancelGesture();
      this.store.preview.set(null);
      if (this.store.tool() !== 'select') this.store.tool.set('select');
      else this.store.selection.set(null);
    } else if (key === ' ') {
      e.preventDefault();
      this.player.toggle();
    } else if (key === 'v') {
      this.store.tool.set('select');
    } else if (key === 'arrowright' && !mod) {
      this.goToStep(this.store.stepIndex() + 1);
    } else if (key === 'arrowleft' && !mod) {
      this.goToStep(this.store.stepIndex() - 1);
    }
  }
}

function fileName(title: string): string {
  return (title.trim() || 'tactics-board')
    .replace(/[^\p{L}\p{N}]+/gu, '-')
    .replace(/^-|-$/g, '')
    .toLowerCase();
}
