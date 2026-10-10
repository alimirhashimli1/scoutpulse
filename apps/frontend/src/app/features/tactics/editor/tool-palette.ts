import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';

import { DrawingKind, EquipmentKind, PALETTE, StrokeStyle, TeamId } from '../board/model';
import { TacticIcon } from '../ui/icon';
import { EditorStore, Placeable } from './editor-store';

interface PaletteTool {
  key: string;
  label: string;
  icon: string;
  pick: () => void;
}

const EQUIPMENT: { kind: EquipmentKind; label: string }[] = [
  { kind: 'ball', label: 'Ball' },
  { kind: 'cone', label: 'Cone' },
  { kind: 'disc', label: 'Disc marker' },
  { kind: 'pole', label: 'Pole' },
  { kind: 'mannequin', label: 'Mannequin' },
  { kind: 'hurdle', label: 'Hurdle' },
  { kind: 'ladder', label: 'Speed ladder' },
  { kind: 'ring', label: 'Ring' },
  { kind: 'minigoal', label: 'Mini goal' },
  { kind: 'goal', label: 'Full-size goal' },
  { kind: 'popup', label: 'Pop-up goal' },
  { kind: 'flag', label: 'Flag' },
];

const WIDTHS = [
  { label: 'Thin', value: 0.2 },
  { label: 'Medium', value: 0.32 },
  { label: 'Thick', value: 0.55 },
];

/**
 * The left-hand toolbar: what a click on the pitch does.
 *
 * Everything is one click to pick. Players are offered per team, so placing
 * eleven is eleven clicks with numbers filled in; equipment and drawing tools
 * follow, then the pen's colour and weight.
 */
@Component({
  selector: 'app-tool-palette',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [TacticIcon],
  template: `
    <div class="group">
      <button
        type="button"
        class="tool"
        [class.on]="active() === 'select'"
        title="Select and move (V)"
        (click)="select()"
      >
        <app-tactic-icon name="select" /><span>Select</span>
      </button>
    </div>

    <h5>Players</h5>
    <div class="group">
      @for (t of teams(); track t.key) {
        <button
          type="button"
          class="tool"
          [class.on]="active() === t.key"
          [title]="t.label"
          (click)="t.pick()"
        >
          <span class="kit" [style.background]="t.color" [style.border-color]="t.edge"></span>
          <span>{{ t.short }}</span>
        </button>
      }
    </div>

    <h5>Equipment</h5>
    <div class="group grid">
      @for (e of equipment; track e.kind) {
        <button
          type="button"
          class="tool icon-only"
          [class.on]="active() === 'place:' + e.kind"
          [title]="e.label"
          [attr.aria-label]="e.label"
          (click)="placeThing({ kind: e.kind })"
        >
          <app-tactic-icon [name]="e.kind" />
        </button>
      }
    </div>

    <h5>Draw</h5>
    <div class="group grid">
      @for (d of drawTools; track d.key) {
        <button
          type="button"
          class="tool icon-only"
          [class.on]="active() === d.key"
          [title]="d.label"
          [attr.aria-label]="d.label"
          (click)="d.pick()"
        >
          <app-tactic-icon [name]="d.icon" />
        </button>
      }
    </div>

    <h5>Colour</h5>
    <div class="swatches" role="radiogroup" aria-label="Drawing colour">
      @for (c of palette; track c) {
        <button
          type="button"
          class="swatch"
          role="radio"
          [attr.aria-checked]="store.pen().color === c"
          [attr.aria-label]="c"
          [style.background]="c"
          (click)="setPen({ color: c })"
        ></button>
      }
      <label class="swatch custom" title="Any colour">
        <input
          type="color"
          [value]="store.pen().color"
          (input)="setPen({ color: $any($event.target).value })"
        />
      </label>
    </div>
    <div class="widths">
      @for (w of widths; track w.value) {
        <button
          type="button"
          [class.on]="store.pen().width === w.value"
          [title]="w.label"
          [attr.aria-label]="w.label + ' line'"
          (click)="setPen({ width: w.value })"
        >
          <span [style.height.px]="w.value * 10"></span>
        </button>
      }
    </div>
  `,
  styleUrl: './tool-palette.css',
})
export class ToolPalette {
  protected readonly store = inject(EditorStore);
  protected readonly palette = PALETTE;
  protected readonly widths = WIDTHS;
  protected readonly equipment = EQUIPMENT;

  /** Which button is lit: the tool, plus what it places or how it draws. */
  protected readonly active = computed(() => {
    const tool = this.store.tool();
    if (tool === 'select') return 'select';
    if (tool === 'place') {
      const p = this.store.placing();
      return p.kind === 'player' ? `player:${p.team}:${p.keeper ? 'gk' : ''}` : `place:${p.kind}`;
    }
    if (tool === 'arrow') return `arrow:${this.store.pen().style}`;
    return `draw:${tool}`;
  });

  protected readonly teams = computed(() => {
    const kits = this.store.doc().teams;
    const out: (PaletteTool & { color: string; edge: string; short: string })[] = [];
    for (const team of ['home', 'away', 'third'] as TeamId[]) {
      const kit = kits[team];
      out.push({
        key: `player:${team}:`,
        label: `${kit.name} player`,
        short: kit.name,
        icon: 'player',
        color: kit.primary,
        edge: kit.secondary,
        pick: () => this.placeThing({ kind: 'player', team }),
      });
    }
    for (const team of ['home', 'away'] as TeamId[]) {
      const kit = kits[team];
      out.push({
        key: `player:${team}:gk`,
        label: `${kit.name} goalkeeper`,
        short: `${kit.name} GK`,
        icon: 'keeper',
        color: kit.keeper,
        edge: kit.secondary,
        pick: () => this.placeThing({ kind: 'player', team, keeper: true }),
      });
    }
    return out;
  });

  protected readonly drawTools: PaletteTool[] = [
    this.drawTool('arrow:solid', 'Run (solid arrow)', 'arrow', 'arrow', 'solid'),
    this.drawTool('arrow:dashed', 'Pass (dashed arrow)', 'pass', 'arrow', 'dashed'),
    this.drawTool('arrow:wavy', 'Dribble (wavy arrow)', 'dribble', 'arrow', 'wavy'),
    this.drawTool('draw:line', 'Line', 'line', 'line'),
    this.drawTool('draw:pen', 'Freehand pen', 'pen', 'pen'),
    this.drawTool('draw:rect', 'Rectangle zone', 'rect', 'rect'),
    this.drawTool('draw:ellipse', 'Circle zone', 'ellipse', 'ellipse'),
    this.drawTool(
      'draw:polygon',
      'Shape zone: click points, click the first again to close',
      'polygon',
      'polygon',
    ),
    this.drawTool('draw:text', 'Text', 'text', 'text'),
  ];

  private drawTool(
    key: string,
    label: string,
    icon: string,
    kind: DrawingKind,
    style?: StrokeStyle,
  ): PaletteTool {
    return {
      key,
      label,
      icon,
      pick: () => {
        if (style) this.setPen({ style });
        else if (kind === 'line' || kind === 'pen') this.setPen({ style: 'solid' });
        this.store.tool.set(kind);
        this.store.selection.set(null);
      },
    };
  }

  protected select(): void {
    this.store.tool.set('select');
  }

  protected placeThing(p: Placeable): void {
    this.store.placing.set(p);
    this.store.tool.set('place');
    this.store.selection.set(null);
  }

  protected setPen(patch: Partial<{ color: string; style: StrokeStyle; width: number }>): void {
    this.store.pen.update((p) => ({ ...p, ...patch }));
    // Restyle the selected drawing too: picking a colour with a drawing
    // selected is almost always meant for that drawing.
    const d = this.store.selectedDrawing();
    if (d && (patch.color || patch.width)) this.store.updateDrawing(d.id, patch);
  }
}
