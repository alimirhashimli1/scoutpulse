import {
  ChangeDetectionStrategy,
  Component,
  DOCUMENT,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { DatePipe } from '@angular/common';
import { RouterLink } from '@angular/router';

import { ApiError } from '../../../core/api/api-error';
import { ssrResource } from '../../../core/api/ssr-resource';
import { TACTIC_READER } from '../../../core/api/tactics';
import { SessionStore } from '../../../core/auth/session-store';
import { Seo } from '../../../core/seo/seo';
import { ErrorState, Loading } from '../../../shared/ui/states';
import { BoardView } from '../board/board-view';
import type { CameraView } from '../board/engine';
import { emptyDoc, normalizeDoc } from '../board/model';
import { BoardPlayer, recordVideo, videoExtension } from '../board/player';
import { TacticIcon } from '../ui/icon';
import { PlaybackBar } from '../ui/playback-bar';

const VIEWS: { value: CameraView; label: string; icon: string }[] = [
  { value: 'top', label: 'From above', icon: 'camTop' },
  { value: 'tactical', label: 'Tactical camera', icon: 'camTilt' },
  { value: 'broadcast', label: 'Broadcast camera', icon: 'camSide' },
  { value: 'behind', label: 'Behind the goal', icon: 'camBehind' },
];

/**
 * One board, for whoever was sent the link.
 *
 * Public and account-free: the players and coaches a board is made for have
 * no accounts. It is rendered on the server so a link pasted into a chat
 * unfurls with the board's title, but it carries `noindex` — a board is
 * unlisted, and a search engine is the last place it should be listed.
 */
@Component({
  selector: 'app-tactics-viewer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, DatePipe, BoardView, PlaybackBar, TacticIcon, Loading, ErrorState],
  templateUrl: './tactics-viewer.html',
  styleUrls: ['./tactics-viewer.css', '../ui/stage.css'],
  host: { '(document:keydown)': 'onKey($event)' },
})
export class TacticsViewer {
  readonly slug = input.required<string>();

  private readonly reader = inject(TACTIC_READER);
  private readonly document = inject(DOCUMENT);
  protected readonly session = inject(SessionStore);

  protected readonly board = ssrResource('tactics-board', {
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
  protected readonly view = signal<CameraView>('broadcast');
  protected readonly copied = signal(false);
  protected readonly recording = signal(false);
  protected readonly started = signal(false);

  protected readonly isOwner = computed(() => {
    const b = this.board.value();
    return !!b && b.owner_id === this.session.user()?.id;
  });
  protected readonly notFound = computed(() => {
    const e = this.board.error();
    return e instanceof ApiError && e.code === 'not_found';
  });

  private readonly stage = viewChild<ElementRef<HTMLElement>>('stage');
  private readonly boardView = viewChild(BoardView);

  constructor() {
    const seo = inject(Seo);
    effect(() => {
      const b = this.board.value();
      seo.describe({
        title: b ? b.title : 'Tactics board',
        description: b
          ? b.description || `An animated tactics board by ${b.owner_name}.`
          : 'An animated tactics board on ScoutPulse.',
        path: `/tactics/${this.slug()}`,
        noindex: true,
      });
    });
    // A fresh board starts on its first step.
    effect(() => {
      this.doc();
      this.player.pause();
      this.player.time.set(0);
    });
    effect(() => {
      if (this.player.playing()) this.started.set(true);
    });
  }

  protected start(): void {
    this.started.set(true);
    this.player.restart();
  }

  protected async copyLink(): Promise<void> {
    try {
      await navigator.clipboard.writeText(this.document.location.href);
      this.copied.set(true);
      setTimeout(() => this.copied.set(false), 2000);
    } catch {
      // The address bar has it.
    }
  }

  protected fullscreen(): void {
    const el = this.stage()?.nativeElement;
    if (!el) return;
    if (this.document.fullscreenElement) void this.document.exitFullscreen();
    else void el.requestFullscreen?.();
  }

  protected async downloadVideo(): Promise<void> {
    const canvas = this.boardView()?.canvas();
    if (!canvas || this.recording()) return;
    this.recording.set(true);
    this.started.set(true);
    try {
      const blob = await recordVideo(canvas, this.player);
      const a = this.document.createElement('a');
      a.href = URL.createObjectURL(blob);
      const name = (this.board.value()?.title ?? 'tactics-board')
        .replace(/[^\p{L}\p{N}]+/gu, '-')
        .toLowerCase();
      a.download = `${name}.${videoExtension(blob)}`;
      a.click();
      setTimeout(() => URL.revokeObjectURL(a.href), 10_000);
    } finally {
      this.recording.set(false);
    }
  }

  protected onKey(e: KeyboardEvent): void {
    const target = e.target as HTMLElement | null;
    if (target && ['INPUT', 'TEXTAREA', 'SELECT', 'BUTTON'].includes(target.tagName)) return;
    if (e.key === ' ') {
      e.preventDefault();
      this.started.set(true);
      this.player.toggle();
    } else if (e.key === 'ArrowRight') {
      this.player.pause();
      this.player.seekStep(this.player.step() + 1);
    } else if (e.key === 'ArrowLeft') {
      this.player.pause();
      this.player.seekStep(this.player.step() - 1);
    } else if (e.key.toLowerCase() === 'f') {
      this.fullscreen();
    }
  }
}
