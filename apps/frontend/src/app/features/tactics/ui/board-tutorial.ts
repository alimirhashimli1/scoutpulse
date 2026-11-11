import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';

import { TacticIcon } from './icon';

export type TutorialMode = 'edit' | 'watch';

type Animation = 'place' | 'move' | 'turn' | 'zoom' | 'pan' | 'play' | 'full';

export interface TutorialScene {
  animation: Animation;
  title: string;
  text: string;
  /** The mouse button shown pressed. */
  press?: 'left' | 'right' | 'wheel';
  /** How many fingers a touch screen shows. */
  fingers?: 1 | 2;
}

/** Seconds each scene runs; the CSS animations share this length. */
const SCENE_SECONDS = 3.2;

const FULL_SCREEN = 'Press Full screen for the biggest pitch.';

/**
 * What to show. The gestures differ in each case: in the editor the left
 * button or one finger edits, so the camera moves with the right button or
 * two fingers; a viewer has nothing to edit, so one drag turns it.
 */
export function tutorialScenes(mode: TutorialMode, touch: boolean): TutorialScene[] {
  const zoom: TutorialScene = touch
    ? { animation: 'zoom', fingers: 2, title: 'Zoom in and out', text: 'Pinch with two fingers.' }
    : { animation: 'zoom', press: 'wheel', title: 'Zoom in and out', text: 'Roll the mouse wheel.' };

  if (mode === 'watch') {
    return [
      {
        animation: 'play',
        fingers: 1,
        press: 'left',
        title: 'Play the board',
        text: touch ? 'Tap play to watch it move.' : 'Click play, or press the space bar.',
      },
      touch
        ? { animation: 'turn', fingers: 1, title: 'Turn the camera', text: 'Drag with one finger.' }
        : {
            animation: 'turn',
            press: 'left',
            title: 'Turn the camera',
            text: 'Hold the left button and drag.',
          },
      zoom,
      touch
        ? { animation: 'pan', fingers: 2, title: 'Slide the view', text: 'Drag with two fingers.' }
        : {
            animation: 'pan',
            press: 'right',
            title: 'Slide the view',
            text: 'Hold the right button and drag.',
          },
      {
        animation: 'full',
        fingers: 1,
        press: 'left',
        title: 'Go full screen',
        text: FULL_SCREEN,
      },
    ];
  }

  const full: TutorialScene = {
    animation: 'full',
    fingers: 1,
    press: 'left',
    title: 'Go full screen',
    text: `${FULL_SCREEN} Pull the side panels in and out by their tabs.`,
  };
  if (touch) {
    return [
      {
        animation: 'place',
        fingers: 1,
        title: 'Add to the pitch',
        text: 'Pull out Tools, pick one, then tap the pitch.',
      },
      { animation: 'move', fingers: 1, title: 'Move a piece', text: 'Drag it with one finger.' },
      { animation: 'turn', fingers: 2, title: 'Turn the camera', text: 'Drag with two fingers.' },
      zoom,
      full,
    ];
  }
  return [
    {
      animation: 'place',
      press: 'left',
      title: 'Add to the pitch',
      text: 'Pick a player or tool on the left, then click the pitch.',
    },
    {
      animation: 'move',
      press: 'left',
      title: 'Move a piece',
      text: 'Hold the left button on a piece and drag it.',
    },
    {
      animation: 'turn',
      press: 'right',
      title: 'Turn the camera',
      text: 'Hold the right button and drag.',
    },
    zoom,
    {
      animation: 'pan',
      press: 'wheel',
      title: 'Slide the view',
      text: 'Hold the wheel down and drag.',
    },
    full,
  ];
}

const key = (mode: TutorialMode) => `scoutpulse:tactics-tutorial:${mode}`;

/** Whether this browser has already been shown the tutorial. */
export function tutorialSeen(mode: TutorialMode): boolean {
  try {
    return localStorage.getItem(key(mode)) === '1';
  } catch {
    // Storage blocked: showing it on each visit would nag, so treat it as seen.
    return true;
  }
}

function markSeen(mode: TutorialMode): void {
  try {
    localStorage.setItem(key(mode), '1');
  } catch {
    // Storage blocked; nothing to remember it in.
  }
}

/**
 * A short animated walk through the controls, played the first time someone
 * opens a board and again from its help button.
 *
 * Drawn in CSS rather than shipped as a video: it weighs nothing, and each
 * scene can show the real gesture for a mouse or for a touch screen.
 */
@Component({
  selector: 'app-board-tutorial',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TacticIcon],
  templateUrl: './board-tutorial.html',
  styleUrls: ['./board-tutorial.css', './board-tutorial-scenes.css'],
})
export class BoardTutorial {
  readonly mode = input.required<TutorialMode>();
  readonly done = output<void>();

  private readonly touch = !!globalThis.matchMedia?.('(pointer: coarse)').matches;
  private readonly still = !!globalThis.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
  protected readonly usesTouch = this.touch;
  protected readonly seconds = SCENE_SECONDS;

  protected readonly scenes = computed(() => tutorialScenes(this.mode(), this.touch));
  protected readonly index = signal(0);
  protected readonly scene = computed(() => this.scenes()[this.index()]);
  protected readonly last = computed(() => this.index() === this.scenes().length - 1);

  private readonly dialog = viewChild<ElementRef<HTMLDialogElement>>('dialog');

  constructor() {
    effect(() => {
      const el = this.dialog()?.nativeElement;
      if (el && !el.open) el.showModal?.();
    });
    // It plays by itself, and waits on the last scene for the button.
    effect((onCleanup) => {
      if (this.still || this.last()) return;
      const next = this.index() + 1;
      const timer = setTimeout(() => this.index.set(next), SCENE_SECONDS * 1000);
      onCleanup(() => clearTimeout(timer));
    });
  }

  protected next(): void {
    if (this.last()) this.finish();
    else this.index.update((i) => i + 1);
  }

  protected finish(): void {
    markSeen(this.mode());
    this.dialog()?.nativeElement.close?.();
    this.done.emit();
  }
}
