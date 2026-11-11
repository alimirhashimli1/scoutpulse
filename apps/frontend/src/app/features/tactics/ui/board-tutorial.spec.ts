import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { BoardTutorial, tutorialScenes, tutorialSeen } from './board-tutorial';

describe('tutorial scenes', () => {
  const find = (scenes: ReturnType<typeof tutorialScenes>, animation: string) =>
    scenes.find((s) => s.animation === animation)!;

  it('in the editor, the camera turns with the right button or two fingers', () => {
    expect(find(tutorialScenes('edit', false), 'turn').press).toBe('right');
    expect(find(tutorialScenes('edit', true), 'turn').fingers).toBe(2);
  });

  it('watching, one drag turns the camera', () => {
    expect(find(tutorialScenes('watch', false), 'turn').press).toBe('left');
    expect(find(tutorialScenes('watch', true), 'turn').fingers).toBe(1);
  });

  it('always covers zooming and full screen, and stays short', () => {
    for (const mode of ['edit', 'watch'] as const) {
      for (const touch of [false, true]) {
        const scenes = tutorialScenes(mode, touch);
        expect(find(scenes, 'zoom')).toBeTruthy();
        expect(find(scenes, 'full')).toBeTruthy();
        expect(scenes.length).toBeLessThanOrEqual(6);
      }
    }
  });
});

describe('BoardTutorial', () => {
  beforeEach(() => localStorage.clear());

  function open() {
    const fixture = TestBed.createComponent(BoardTutorial);
    fixture.componentRef.setInput('mode', 'edit');
    fixture.detectChanges();
    return { fixture, root: fixture.nativeElement as HTMLElement };
  }

  it('remembers that it was skipped, per mode', () => {
    expect(tutorialSeen('edit')).toBe(false);
    const { fixture, root } = open();
    let finished = false;
    fixture.componentInstance.done.subscribe(() => (finished = true));
    root.querySelector<HTMLButtonElement>('.skip')!.click();
    expect(finished).toBe(true);
    expect(tutorialSeen('edit')).toBe(true);
    expect(tutorialSeen('watch')).toBe(false);
  });

  it('stays on a scene until Next is pressed', () => {
    vi.useFakeTimers();
    try {
      const { fixture, root } = open();
      vi.advanceTimersByTime(120_000);
      fixture.detectChanges();
      expect(root.querySelector('.count')?.textContent).toContain('1 of');
    } finally {
      vi.useRealTimers();
    }
  });

  it('walks through the scenes and ends on Start', () => {
    const { fixture, root } = open();
    const next = () => root.querySelector<HTMLButtonElement>('.btn')!;
    const total = root.querySelectorAll('.progress span').length;
    for (let i = 1; i < total; i++) {
      expect(next().textContent).toContain('Next');
      next().click();
      fixture.detectChanges();
    }
    expect(next().textContent).toContain('Start');
    next().click();
    expect(tutorialSeen('edit')).toBe(true);
  });
});
