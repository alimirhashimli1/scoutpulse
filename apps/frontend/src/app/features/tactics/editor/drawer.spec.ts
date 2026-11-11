import { TestBed } from '@angular/core/testing';

import { Drawer } from './drawer';

function make(side: 'left' | 'right', open: boolean, enabled = true) {
  const fixture = TestBed.createComponent(Drawer);
  const ref = fixture.componentRef;
  ref.setInput('side', side);
  ref.setInput('label', 'Tools');
  ref.setInput('enabled', enabled);
  ref.setInput('open', open);
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  Object.defineProperty(host, 'offsetWidth', { value: 200 });
  const handle = host.querySelector<HTMLButtonElement>('.handle');
  const pointer = (type: string, clientX: number) =>
    handle!.dispatchEvent(Object.assign(new MouseEvent(type, { clientX }), { pointerId: 1 }));
  return { fixture, host, handle, pointer, isOpen: () => fixture.componentInstance.open() };
}

describe('Drawer', () => {
  it('stays out of the layout when not enabled', () => {
    const { handle, host } = make('left', false, false);
    expect(handle).toBeNull();
    expect(host.classList.contains('on')).toBe(false);
  });

  it('opens and closes on a tap of its tab', () => {
    const { handle, isOpen } = make('left', false);
    handle!.click();
    expect(isOpen()).toBe(true);
    handle!.click();
    expect(isOpen()).toBe(false);
  });

  it('opens when pulled far enough, and springs back when not', () => {
    const a = make('left', false);
    a.pointer('pointerdown', 0);
    a.pointer('pointermove', 120);
    a.pointer('pointerup', 120);
    expect(a.isOpen()).toBe(true);

    const b = make('left', false);
    b.pointer('pointerdown', 0);
    b.pointer('pointermove', 20);
    b.pointer('pointerup', 20);
    expect(b.isOpen()).toBe(false);
  });

  it('pulls the other way on the right, and a drag is not also a tap', () => {
    const d = make('right', true);
    d.pointer('pointerdown', 300);
    d.pointer('pointermove', 420);
    d.pointer('pointerup', 420);
    expect(d.isOpen()).toBe(false);
    d.handle!.click();
    expect(d.isOpen()).toBe(false);
  });

  it('hides a closed panel from keyboard and screen readers', () => {
    const { host } = make('left', false);
    expect(host.querySelector('.panel')?.hasAttribute('inert')).toBe(true);
  });
});
