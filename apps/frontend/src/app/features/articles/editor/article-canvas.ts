import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  ViewEncapsulation,
  afterNextRender,
  effect,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import type { Editor } from '@tiptap/core';

import { ArticleDoc } from '../doc/article-doc';
import type { EmbedHooks } from './tiptap-setup';

/**
 * Hosts the Tiptap editor.
 *
 * Its stylesheet is unencapsulated because ProseMirror builds its DOM itself,
 * outside Angular's view, where emulated encapsulation never stamps the
 * attributes its selectors would need. Every rule is scoped under `.ae-` to
 * keep that from leaking.
 *
 * The typography mirrors the reader's (article-body.ts) closely enough that
 * writing here already looks like the published piece.
 */
@Component({
  selector: 'app-article-canvas',
  changeDetection: ChangeDetectionStrategy.OnPush,
  encapsulation: ViewEncapsulation.None,
  template: `
    <div class="ae-canvas" #host>
      @if (!editor()) {
        <p class="ae-booting">Opening the editor…</p>
      }
    </div>
  `,
  styleUrl: './article-canvas.css',
})
export class ArticleCanvas {
  /** Read once, when the editor is created. Later changes come from the editor. */
  readonly initial = input.required<ArticleDoc>();
  readonly editable = input(true);
  readonly hooks = input.required<EmbedHooks>();

  readonly changed = output<ArticleDoc>();

  readonly editor = signal<Editor | null>(null);
  /** Bumped on every transaction, so a toolbar can recompute what is active. */
  readonly version = signal(0);

  private readonly host = viewChild.required<ElementRef<HTMLElement>>('host');

  constructor() {
    const destroyRef = inject(DestroyRef);
    let destroyed = false;
    destroyRef.onDestroy(() => {
      destroyed = true;
      this.editor()?.destroy();
    });

    afterNextRender(async () => {
      const { createArticleEditor } = await import('./tiptap-setup');
      if (destroyed) return;
      const editor = createArticleEditor({
        element: this.host().nativeElement,
        content: this.initial(),
        editable: this.editable(),
        hooks: this.hooks(),
        onChange: (doc) => this.changed.emit(doc),
        onTransaction: () => this.version.update((v) => v + 1),
      });
      this.editor.set(editor);
    });

    effect(() => {
      const editor = this.editor();
      const editable = this.editable();
      if (editor && editor.isEditable !== editable) editor.setEditable(editable, false);
    });
  }

  focus(): void {
    this.editor()?.commands.focus();
  }
}
