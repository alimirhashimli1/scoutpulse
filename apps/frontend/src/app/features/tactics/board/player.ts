import { Signal, computed, signal } from '@angular/core';

import { TacticDoc } from './model';
import { Frame, Timeline, frameAt, stepAtTime, timeline } from './playback';

/**
 * Playback state: the clock, play/pause and speed.
 *
 * Shared by the editor and the viewer, and driven by the engine's animation
 * frame through `tick`, so the clock advances exactly once per rendered frame
 * and the picture never runs ahead of or behind the time shown.
 */
export class BoardPlayer {
  readonly time = signal(0);
  readonly playing = signal(false);
  readonly speed = signal(1);
  /** Loop back to the start at the end, for a board left on a screen. */
  readonly loop = signal(false);

  readonly timeline: Signal<Timeline>;
  readonly frame: Signal<Frame>;
  readonly step: Signal<number>;
  readonly atEnd: Signal<boolean>;
  /**
   * A play-through ran to the end and nothing has moved the clock since: the
   * moment to offer a replay. Unlike atEnd, scrubbing or stepping to the last
   * step does not count, so an editor working on that step is not covered by
   * a replay button.
   */
  readonly finished: Signal<boolean>;
  private readonly endedAt = signal(-1);

  /** Called once when a play-through reaches the end. */
  onEnded: (() => void) | null = null;

  constructor(doc: Signal<TacticDoc>) {
    this.timeline = computed(() => timeline(doc()));
    this.frame = computed(() => frameAt(doc(), this.timeline(), this.time()));
    this.step = computed(() => stepAtTime(this.timeline(), this.time()));
    this.atEnd = computed(() => this.time() >= this.timeline().total - 1e-3);
    this.finished = computed(
      () => !this.playing() && this.endedAt() >= 0 && this.time() === this.endedAt(),
    );
  }

  tick(dt: number): void {
    if (!this.playing()) return;
    const total = this.timeline().total;
    const next = this.time() + dt * this.speed();
    if (next >= total) {
      if (this.loop() && total > 0) {
        this.time.set(0);
        return;
      }
      this.time.set(total);
      this.endedAt.set(total);
      this.playing.set(false);
      this.onEnded?.();
      return;
    }
    this.time.set(next);
  }

  play(): void {
    if (this.atEnd()) this.time.set(0);
    this.playing.set(true);
  }

  pause(): void {
    this.playing.set(false);
  }

  toggle(): void {
    if (this.playing()) this.pause();
    else this.play();
  }

  restart(): void {
    this.time.set(0);
    this.playing.set(true);
  }

  /** Hides the replay offer without moving the clock. */
  dismissFinished(): void {
    this.endedAt.set(-1);
  }

  seek(time: number): void {
    this.time.set(Math.max(0, Math.min(this.timeline().total, time)));
  }

  /** Jumps to the moment a step is reached. */
  seekStep(step: number): void {
    const starts = this.timeline().stepStarts;
    const i = Math.max(0, Math.min(starts.length - 1, step));
    this.seek(starts[i] ?? 0);
  }
}

/**
 * Records a play-through of the board to a video file, entirely in the
 * browser. Nothing is uploaded: the canvas is captured as it plays and the
 * result is handed to the person to save or send.
 */
export async function recordVideo(canvas: HTMLCanvasElement, player: BoardPlayer): Promise<Blob> {
  const types = ['video/mp4;codecs=avc1', 'video/mp4', 'video/webm;codecs=vp9', 'video/webm'];
  const mimeType = types.find((t) => MediaRecorder.isTypeSupported(t)) ?? '';
  const stream = canvas.captureStream(30);
  const recorder = new MediaRecorder(
    stream,
    mimeType ? { mimeType, videoBitsPerSecond: 6_000_000 } : undefined,
  );
  const chunks: BlobPart[] = [];
  recorder.ondataavailable = (e) => e.data.size && chunks.push(e.data);

  const done = new Promise<Blob>((resolve) => {
    recorder.onstop = () => resolve(new Blob(chunks, { type: recorder.mimeType || 'video/webm' }));
  });

  const loop = player.loop();
  player.loop.set(false);
  player.time.set(0);
  recorder.start(250);
  // A short lead-in, so the video does not open mid-movement.
  await new Promise((r) => setTimeout(r, 400));
  player.onEnded = () => setTimeout(() => recorder.stop(), 700);
  player.play();

  try {
    return await done;
  } finally {
    player.onEnded = null;
    player.loop.set(loop);
    stream.getTracks().forEach((t) => t.stop());
  }
}

export function videoExtension(blob: Blob): string {
  return blob.type.includes('mp4') ? 'mp4' : 'webm';
}
