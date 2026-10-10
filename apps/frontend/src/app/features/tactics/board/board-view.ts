import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  afterNextRender,
  effect,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';

import type { BoardEngine, CameraView, DrawingChange, EditState, Target, Tool } from './engine';
import { Drawing, TacticDoc, Vec2 } from './model';
import { Frame } from './playback';

/**
 * Hosts the 3D board.
 *
 * The engine is imported dynamically after the first render, so Three.js is
 * never loaded on the server — there is no WebGL there — and is only fetched
 * by the pages that show a board. Until it arrives the frame shows the pitch
 * colour, which is the right placeholder for a pitch.
 */
@Component({
  selector: 'app-board-view',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stage" #stage>
      @if (failed()) {
        <p class="fallback">
          This board needs WebGL, which this browser has turned off or does not support.
        </p>
      } @else if (!ready()) {
        <p class="fallback" aria-live="polite">Setting out the pitch…</p>
      }
    </div>
  `,
  styles: `
    :host {
      display: block;
      position: relative;
      min-height: 0;
    }
    .stage {
      position: absolute;
      inset: 0;
      overflow: hidden;
      background: radial-gradient(ellipse at center, #3f9b4c 0%, #24572c 100%);
      border-radius: inherit;
    }
    .fallback {
      position: absolute;
      inset: 0;
      display: grid;
      place-items: center;
      margin: 0;
      max-width: none;
      padding: var(--space-5);
      text-align: center;
      color: #ecfdf5;
      font-size: var(--text-sm);
    }
  `,
})
export class BoardView {
  readonly doc = input.required<TacticDoc>();
  readonly frame = input.required<Frame>();
  readonly editState = input<EditState>({
    editing: false,
    step: 0,
    selection: null,
    trails: false,
  });
  readonly tool = input<Tool>('select');
  readonly preview = input<Drawing | null>(null);
  readonly view = input<CameraView>('tactical');
  /** Called every animation frame; playback advances its clock here. */
  readonly tick = input<((dt: number) => void) | null>(null);

  readonly selected = output<Target | null>();
  readonly objectMoved = output<{ id: string; at: Vec2; done: boolean }>();
  readonly bent = output<{ id: string; mid: Vec2; done: boolean }>();
  readonly drawingEdited = output<{ id: string; change: DrawingChange; done: boolean }>();
  readonly placed = output<Vec2>();
  readonly stroked = output<{ points: Vec2[]; done: boolean }>();

  protected readonly ready = signal(false);
  protected readonly failed = signal(false);

  private readonly stage = viewChild.required<ElementRef<HTMLElement>>('stage');
  private engine: BoardEngine | null = null;

  constructor() {
    const destroyRef = inject(DestroyRef);
    let destroyed = false;
    destroyRef.onDestroy(() => {
      destroyed = true;
      this.engine?.dispose();
      this.engine = null;
    });

    afterNextRender(async () => {
      try {
        const { BoardEngine } = await import('./engine');
        if (destroyed) return;
        const engine = new BoardEngine(this.stage().nativeElement);
        engine.handlers = {
          select: (t) => this.selected.emit(t),
          moveObject: (id, at, done) => this.objectMoved.emit({ id, at, done }),
          bend: (id, mid, done) => this.bent.emit({ id, mid, done }),
          editDrawing: (id, change, done) => this.drawingEdited.emit({ id, change, done }),
          place: (at) => this.placed.emit(at),
          stroke: (points, done) => this.stroked.emit({ points, done }),
        };
        this.engine = engine;
        this.ready.set(true);
      } catch (error) {
        console.error('Could not start the tactics board', error);
        this.failed.set(true);
      }
    });

    // Each input reaches the engine through its own effect, so a frame change
    // during playback does not also re-run the document reconciliation.
    effect(() => {
      const doc = this.doc();
      if (this.ready()) this.engine?.setDoc(doc);
    });
    effect(() => {
      const frame = this.frame();
      if (this.ready()) this.engine?.setFrame(frame);
    });
    effect(() => {
      const state = this.editState();
      if (this.ready()) this.engine?.setEditState(state);
    });
    effect(() => {
      const tool = this.tool();
      if (this.ready() && this.engine) {
        if (this.engine.tool !== tool) this.engine.cancelGesture();
        this.engine.tool = tool;
      }
    });
    effect(() => {
      const preview = this.preview();
      if (this.ready()) this.engine?.setPreview(preview);
    });
    effect(() => {
      const view = this.view();
      if (this.ready() && this.engine && this.engine.currentView !== view)
        this.engine.setView(view);
    });
    effect(() => {
      const tick = this.tick();
      if (this.ready() && this.engine) this.engine.onTick = tick;
    });
  }

  /** The canvas, for recording a video of it. */
  canvas(): HTMLCanvasElement | null {
    return this.engine?.canvas ?? null;
  }

  /** Re-applies the current view, e.g. after the pitch layout changes. */
  reframe(): void {
    this.engine?.setView(this.view());
  }

  cancelGesture(): void {
    this.engine?.cancelGesture();
  }
}
