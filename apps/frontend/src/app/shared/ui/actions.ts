import { ChangeDetectionStrategy, Component } from '@angular/core';

/**
 * The row of write actions on a read page.
 *
 * Its whole job is to look the same everywhere and to wrap on a narrow screen
 * instead of pushing the page sideways. The buttons themselves are projected
 * and styled globally, because `.btn` is shared with the forms.
 *
 * Nothing here decides *whether* to show an action — that is the page's call,
 * made through `Permissions`, so the reasoning sits next to the record whose
 * permissions are being consulted.
 */
@Component({
  selector: 'app-actions',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `<div class="actions"><ng-content /></div>`,
  styles: `
    .actions {
      display: flex;
      gap: var(--space-2);
      flex-wrap: wrap;
      align-items: center;
    }
  `,
})
export class Actions {}
