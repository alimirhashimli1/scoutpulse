import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { MarketValue } from '../../core/models/football';

interface Point {
  x: number;
  y: number;
  value: MarketValue;
}

/**
 * A player's valuation over time, as inline SVG.
 *
 * No chart library. One sparkline does not justify a dependency an order of
 * magnitude larger than itself, and hand-drawn SVG renders identically on the
 * server — a charting library that needs a DOM would either break SSR or have
 * to be excluded from it.
 */
@Component({
  selector: 'app-value-chart',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (points().length < 2) {
      <p class="note">Not enough history to chart yet — a line needs at least two valuations.</p>
    } @else {
      <figure>
        <svg
          [attr.viewBox]="'0 0 ' + width + ' ' + height"
          preserveAspectRatio="none"
          role="img"
          [attr.aria-label]="summary()"
        >
          <!-- Area under the line, for weight rather than decoration. -->
          <path [attr.d]="areaPath()" class="area" />
          <path [attr.d]="linePath()" class="line" />

          <!-- The latest point is emphasised: it is the number that matters. -->
          @if (last(); as point) {
            <circle [attr.cx]="point.x" [attr.cy]="point.y" r="3" class="endpoint" />
          }
        </svg>

        <figcaption>
          <span>{{ firstLabel() }}</span>
          <span>{{ lastLabel() }}</span>
        </figcaption>
      </figure>
    }
  `,
  styles: `
    figure {
      margin: 0;
    }
    svg {
      width: 100%;
      height: 7rem;
      display: block;
      overflow: visible;
    }
    .area {
      fill: var(--accent-soft);
      stroke: none;
    }
    .line {
      fill: none;
      stroke: var(--accent);
      stroke-width: 2;
      /* The viewBox is stretched by preserveAspectRatio="none", which would
         distort the stroke too without this. */
      vector-effect: non-scaling-stroke;
      stroke-linejoin: round;
    }
    .endpoint {
      fill: var(--accent);
    }
    figcaption {
      display: flex;
      justify-content: space-between;
      font-size: var(--text-xs);
      color: var(--muted);
      margin-top: var(--space-2);
    }
    .note {
      color: var(--muted);
      font-size: var(--text-sm);
    }
  `,
})
export class ValueChart {
  /** Newest first, as the API returns them. */
  readonly values = input.required<MarketValue[]>();

  protected readonly width = 600;
  protected readonly height = 160;

  /** Oldest first, scaled into the viewBox. */
  protected readonly points = computed<Point[]>(() => {
    const series = [...this.values()].reverse();
    if (series.length === 0) return [];

    const amounts = series.map((v) => v.value_minor);
    const min = Math.min(...amounts);
    const max = Math.max(...amounts);
    // A flat series would divide by zero; drawing it mid-height is honest.
    const span = max - min || 1;

    const padding = 8;
    const usable = this.height - padding * 2;

    return series.map((value, index) => ({
      x: series.length === 1 ? this.width / 2 : (index / (series.length - 1)) * this.width,
      y: padding + (1 - (value.value_minor - min) / span) * usable,
      value,
    }));
  });

  protected readonly last = computed(() => this.points().at(-1));

  protected readonly linePath = computed(() =>
    this.points()
      .map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x},${p.y}`)
      .join(' '),
  );

  protected readonly areaPath = computed(() => {
    const points = this.points();
    if (points.length < 2) return '';
    return `${this.linePath()} L${points.at(-1)!.x},${this.height} L${points[0].x},${this.height} Z`;
  });

  protected readonly firstLabel = computed(() => year(this.points()[0]?.value));
  protected readonly lastLabel = computed(() => year(this.last()?.value));

  /** Read out by screen readers, which cannot see the line. */
  protected readonly summary = computed(() => {
    const points = this.points();
    if (points.length < 2) return 'Market value history';
    const first = points[0].value.value_minor / 100;
    const latest = points.at(-1)!.value.value_minor / 100;
    const direction = latest > first ? 'risen' : latest < first ? 'fallen' : 'held steady';
    return `Market value has ${direction} between ${year(points[0].value)} and ${year(points.at(-1)!.value)}`;
  });
}

function year(value: MarketValue | undefined): string {
  return value ? new Date(value.valued_on).getFullYear().toString() : '';
}
