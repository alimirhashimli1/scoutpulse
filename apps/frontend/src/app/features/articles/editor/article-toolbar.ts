import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import type { Editor } from '@tiptap/core';

import { ArticleIcon } from '../ui/article-icon';

export type InsertKind = 'link' | 'image' | 'board' | 'video';

type Block = 'paragraph' | 'h2' | 'h3' | 'h4';

const MARKS = [
  { mark: 'bold', icon: 'bold', label: 'Bold (Ctrl+B)' },
  { mark: 'italic', icon: 'italic', label: 'Italic (Ctrl+I)' },
  { mark: 'underline', icon: 'underline', label: 'Underline (Ctrl+U)' },
  { mark: 'strike', icon: 'strike', label: 'Strikethrough' },
] as const;

/**
 * The formatting bar above the text.
 *
 * Reads the editor's state on every transaction (`version`), so the buttons
 * show what is active at the cursor. Inserting a link or an embed is the
 * page's job, because it opens a dialog; the bar only asks for one.
 */
@Component({
  selector: 'app-article-toolbar',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ArticleIcon],
  template: `
    <div class="bar" role="toolbar" aria-label="Formatting" (mousedown)="keepFocus($event)">
      <label class="visually-hidden" for="block-style">Text style</label>
      <select
        id="block-style"
        [value]="state().block"
        [disabled]="!enabled()"
        (change)="setBlock($any($event.target).value)"
      >
        <option value="paragraph">Paragraph</option>
        <option value="h2">Heading</option>
        <option value="h3">Subheading</option>
        <option value="h4">Small heading</option>
      </select>

      <span class="sep"></span>
      @for (m of marks; track m.mark) {
        <button
          type="button"
          [class.on]="state().marks[m.mark]"
          [attr.aria-pressed]="state().marks[m.mark]"
          [title]="m.label"
          [attr.aria-label]="m.label"
          [disabled]="!enabled()"
          (click)="toggleMark(m.mark)"
        >
          <app-article-icon [name]="m.icon" />
        </button>
      }
      <button
        type="button"
        [class.on]="state().link"
        title="Link (Ctrl+K)"
        aria-label="Link"
        [disabled]="!enabled()"
        (click)="insert.emit('link')"
      >
        <app-article-icon name="link" />
      </button>

      <span class="sep"></span>
      <button
        type="button"
        [class.on]="state().bullets"
        title="Bulleted list"
        aria-label="Bulleted list"
        [disabled]="!enabled()"
        (click)="run('toggleBulletList')"
      >
        <app-article-icon name="bullets" />
      </button>
      <button
        type="button"
        [class.on]="state().numbers"
        title="Numbered list"
        aria-label="Numbered list"
        [disabled]="!enabled()"
        (click)="run('toggleOrderedList')"
      >
        <app-article-icon name="numbers" />
      </button>
      <button
        type="button"
        [class.on]="state().quote"
        title="Quote"
        aria-label="Quote"
        [disabled]="!enabled()"
        (click)="run('toggleBlockquote')"
      >
        <app-article-icon name="quote" />
      </button>
      <button
        type="button"
        title="Divider"
        aria-label="Divider"
        [disabled]="!enabled()"
        (click)="run('setHorizontalRule')"
      >
        <app-article-icon name="divider" />
      </button>

      <span class="sep"></span>
      <button
        type="button"
        class="wide"
        title="Add an image from a web address"
        [disabled]="!enabled()"
        (click)="insert.emit('image')"
      >
        <app-article-icon name="image" /><span>Image</span>
      </button>
      <button
        type="button"
        class="wide"
        title="Embed one of your tactics boards"
        [disabled]="!enabled()"
        (click)="insert.emit('board')"
      >
        <app-article-icon name="board" /><span>Board</span>
      </button>
      <button
        type="button"
        class="wide"
        title="Embed a YouTube or Vimeo video"
        [disabled]="!enabled()"
        (click)="insert.emit('video')"
      >
        <app-article-icon name="video" /><span>Video</span>
      </button>
    </div>
  `,
  styles: `
    :host {
      display: block;
      container-type: inline-size;
    }
    .bar {
      display: flex;
      align-items: center;
      flex-wrap: wrap;
      gap: 2px;
      padding: 4px;
      border: 1px solid var(--line);
      border-radius: var(--radius-lg);
      background: var(--surface);
      box-shadow: var(--shadow-sm);
    }
    select {
      height: 2.1rem;
      padding: 0 var(--space-2);
      border: 1px solid transparent;
      border-radius: var(--radius);
      background: none;
      font-size: var(--text-sm);
      cursor: pointer;
    }
    select:hover:not(:disabled) {
      border-color: var(--line);
    }
    button {
      min-width: 2.1rem;
      height: 2.1rem;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      border: 0;
      border-radius: var(--radius);
      background: none;
      color: var(--ink-soft);
      cursor: pointer;
      font-size: var(--text-sm);
      font-weight: 600;
    }
    button.wide {
      padding: 0 var(--space-2);
    }
    button:hover:not(:disabled) {
      background: var(--surface-2);
      color: var(--accent);
    }
    button.on {
      background: var(--accent-soft);
      color: var(--accent);
    }
    button:disabled,
    select:disabled {
      opacity: 0.35;
      cursor: default;
    }
    .sep {
      width: 1px;
      height: 1.4rem;
      margin-inline: 4px;
      background: var(--line);
    }
    /* Labels go before the bar wraps; the icons carry titles. */
    @container (max-width: 42rem) {
      button.wide span {
        display: none;
      }
    }
  `,
})
export class ArticleToolbar {
  readonly editor = input<Editor | null>(null);
  /** Changes on every transaction; read so the state below recomputes. */
  readonly version = input(0);
  readonly insert = output<InsertKind>();

  protected readonly marks = MARKS;

  protected readonly enabled = computed(() => {
    this.version();
    return !!this.editor()?.isEditable;
  });

  protected readonly state = computed(() => {
    this.version();
    const e = this.editor();
    const marks = { bold: false, italic: false, underline: false, strike: false };
    if (!e) {
      return {
        block: 'paragraph' as Block,
        marks,
        link: false,
        bullets: false,
        numbers: false,
        quote: false,
      };
    }
    for (const m of MARKS) marks[m.mark] = e.isActive(m.mark);
    const block: Block = e.isActive('heading', { level: 2 })
      ? 'h2'
      : e.isActive('heading', { level: 3 })
        ? 'h3'
        : e.isActive('heading', { level: 4 })
          ? 'h4'
          : 'paragraph';
    return {
      block,
      marks,
      link: e.isActive('link'),
      bullets: e.isActive('bulletList'),
      numbers: e.isActive('orderedList'),
      quote: e.isActive('blockquote'),
    };
  });

  /**
   * A click on a button must not take focus from the text: the selection it
   * acts on would be lost, and a dialog closing would hand focus back to the
   * button instead of the article.
   */
  protected keepFocus(e: MouseEvent): void {
    if ((e.target as HTMLElement).closest('button')) e.preventDefault();
  }

  protected toggleMark(mark: (typeof MARKS)[number]['mark']): void {
    this.editor()?.chain().focus().toggleMark(mark).run();
  }

  protected setBlock(block: Block): void {
    const chain = this.editor()?.chain().focus();
    if (!chain) return;
    if (block === 'paragraph') chain.setParagraph().run();
    else chain.setHeading({ level: Number(block.slice(1)) as 2 | 3 | 4 }).run();
  }

  protected run(
    command: 'toggleBulletList' | 'toggleOrderedList' | 'toggleBlockquote' | 'setHorizontalRule',
  ): void {
    const chain = this.editor()?.chain().focus();
    if (chain) chain[command]().run();
  }
}
