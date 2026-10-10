import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';

import {
  FORMATION_NAMES,
  KIT_PATTERNS,
  KitPattern,
  PitchLayout,
  PitchMarkings,
  PitchStyle,
  StrokeStyle,
  TeamId,
  playerLook,
} from '../board/model';
import { ColorPick } from '../ui/color-pick';
import { TacticIcon } from '../ui/icon';
import { EditorStore } from './editor-store';

const ROTATABLE = new Set(['goal', 'minigoal', 'popup', 'mannequin', 'hurdle', 'ladder', 'flag']);

/**
 * The right-hand panel: the properties of whatever is selected, or the board
 * itself — pitch, kits, formations — when nothing is.
 */
@Component({
  selector: 'app-inspector',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ColorPick, TacticIcon],
  templateUrl: './inspector.html',
  styleUrl: './inspector.css',
})
export class Inspector {
  protected readonly store = inject(EditorStore);
  protected readonly patterns = KIT_PATTERNS;
  protected readonly formations = FORMATION_NAMES;
  protected readonly teamIds: TeamId[] = ['home', 'away', 'third'];

  protected readonly layouts: { value: PitchLayout; label: string }[] = [
    { value: 'full', label: 'Full pitch' },
    { value: 'half', label: 'Half' },
    { value: 'third', label: 'Final third' },
    { value: 'box', label: 'Penalty area' },
    { value: 'drill', label: 'Drill area' },
  ];
  protected readonly markings: { value: PitchMarkings; label: string }[] = [
    { value: 'standard', label: 'Standard' },
    { value: 'grid', label: 'Grid' },
    { value: 'none', label: 'None' },
  ];
  protected readonly styles: { value: PitchStyle; label: string }[] = [
    { value: 'day', label: 'Day' },
    { value: 'night', label: 'Floodlit' },
    { value: 'board', label: 'Chalkboard' },
  ];
  protected readonly strokeStyles: { value: StrokeStyle; label: string }[] = [
    { value: 'solid', label: 'Solid' },
    { value: 'dashed', label: 'Dashed' },
    { value: 'wavy', label: 'Wavy' },
  ];

  protected readonly kitTab = signal<TeamId>('home');
  protected readonly formationName = signal('4-3-3');
  protected readonly formationTeam = signal<TeamId>('home');

  protected readonly obj = this.store.selectedObject;
  protected readonly drawing = this.store.selectedDrawing;

  protected readonly look = computed(() => {
    const o = this.obj();
    return o?.kind === 'player' ? playerLook(this.store.doc(), o) : null;
  });
  protected readonly rotatable = computed(() => ROTATABLE.has(this.obj()?.kind ?? ''));
  protected readonly inAir = computed(() => {
    const o = this.obj();
    return !!o && !!this.store.step().air?.includes(o.id);
  });
  protected readonly highlighted = computed(() => {
    const o = this.obj();
    return !!o && !!this.store.step().highlight?.includes(o.id);
  });
  protected readonly bent = computed(() => {
    const o = this.obj();
    return !!o && !!this.store.step().bend?.[o.id];
  });
  protected readonly isZone = computed(() =>
    ['rect', 'ellipse', 'polygon'].includes(this.drawing()?.kind ?? ''),
  );
  protected readonly isLine = computed(() =>
    ['arrow', 'line', 'pen'].includes(this.drawing()?.kind ?? ''),
  );

  private readonly textField = viewChild<ElementRef<HTMLInputElement>>('textField');

  constructor() {
    // A freshly placed text label opens with its text selected, ready to type.
    effect(() => {
      const id = this.store.focusText();
      const field = this.textField();
      if (id && field && this.drawing()?.id === id) {
        field.nativeElement.focus();
        field.nativeElement.select();
        this.store.focusText.set(null);
      }
    });
  }

  protected value(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }

  protected numberValue(e: Event): number {
    return Number((e.target as HTMLInputElement).value);
  }

  protected setPattern(e: Event): void {
    const o = this.obj();
    const v = this.value(e);
    if (o) this.store.updateObject(o.id, { pattern: v ? (v as KitPattern) : undefined });
  }

  protected addFormation(side: 'left' | 'right'): void {
    this.store.addFormation(this.formationName(), this.formationTeam(), side);
  }
}
