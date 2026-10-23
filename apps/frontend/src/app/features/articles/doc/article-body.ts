import { ChangeDetectionStrategy, Component, computed, input, signal } from '@angular/core';
import { NgTemplateOutlet } from '@angular/common';

import {
  ArticleDoc,
  DocMark,
  DocNode,
  FigureAttrs,
  TacticAttrs,
  VideoAttrs,
  VideoProvider,
  isExternal,
  outline,
  plainText,
  safeImage,
  safeLink,
} from './article-doc';
import { ArticleTactic } from './tactic-embed';
import { ArticleVideo } from './video-embed';

interface InlineMark {
  type: string;
  href: string | null;
  external: boolean;
}

/**
 * Draws an article body.
 *
 * A recursive walk over the document in templates, one case per node type
 * the site knows; anything else is skipped. That is the whole of the safety
 * story: nothing here builds HTML from the document, so text a writer pasted
 * is always text, and the only addresses that reach an attribute are the ones
 * `safeLink` and `safeImage` let through.
 *
 * Whitespace matters in the inline templates. Angular drops whitespace that
 * stands alone between tags, but collapses whitespace *next to text* to a
 * space -- so a line break beside the text would put a space either side of
 * every bold word. That is why the text sits alone inside an ng-container and
 * the closing tags hug their content (`/></strong>`). The spec checks it.
 *
 * Used by the article page and by the editor's preview, so what a writer
 * previews is what a reader gets.
 */
@Component({
  selector: 'app-article-body',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [NgTemplateOutlet, ArticleTactic, ArticleVideo],
  template: `
    <ng-template #blocks let-nodes>
      @for (n of nodes ?? []; track $index) {
        @switch (n.type) {
          @case ('paragraph') {
            @if (hasText(n)) {
              <p><ng-container *ngTemplateOutlet="inline; context: { $implicit: n.content }" /></p>
            }
          }
          @case ('heading') {
            @switch (n.attrs?.level) {
              @case (3) {
                <h3 [id]="idFor(n)">
                  <ng-container *ngTemplateOutlet="inline; context: { $implicit: n.content }" />
                </h3>
              }
              @case (4) {
                <h4 [id]="idFor(n)">
                  <ng-container *ngTemplateOutlet="inline; context: { $implicit: n.content }" />
                </h4>
              }
              @default {
                <h2 [id]="idFor(n)">
                  <ng-container *ngTemplateOutlet="inline; context: { $implicit: n.content }" />
                </h2>
              }
            }
          }
          @case ('bulletList') {
            <ul>
              <ng-container *ngTemplateOutlet="blocks; context: { $implicit: n.content }" />
            </ul>
          }
          @case ('orderedList') {
            <ol [attr.start]="startOf(n)">
              <ng-container *ngTemplateOutlet="blocks; context: { $implicit: n.content }" />
            </ol>
          }
          @case ('listItem') {
            <li><ng-container *ngTemplateOutlet="blocks; context: { $implicit: n.content }" /></li>
          }
          @case ('blockquote') {
            <blockquote>
              <ng-container *ngTemplateOutlet="blocks; context: { $implicit: n.content }" />
            </blockquote>
          }
          @case ('horizontalRule') {
            <hr />
          }
          @case ('figure') {
            @if (figure(n); as f) {
              <figure class="embed figure" [class.wide]="f.size === 'wide'">
                @if (failed().has(f.src)) {
                  <div class="broken">The image at this address could not be loaded.</div>
                } @else {
                  <img
                    [src]="f.src"
                    [alt]="f.alt"
                    loading="lazy"
                    decoding="async"
                    referrerpolicy="no-referrer"
                    (error)="markFailed(f.src)"
                  />
                }
                @if (f.caption || f.credit) {
                  <figcaption>
                    @if (f.caption) {
                      <span>{{ f.caption }}</span>
                    }
                    @if (f.credit) {
                      <span class="credit">{{ f.credit }}</span>
                    }
                  </figcaption>
                }
              </figure>
            }
          }
          @case ('tacticBoard') {
            @if (tactic(n); as t) {
              <figure class="embed board wide">
                @defer (on viewport) {
                  <app-article-tactic [slug]="t.slug" />
                } @placeholder {
                  <div class="board-placeholder">
                    <span>{{ t.title || 'Tactics board' }}</span>
                  </div>
                }
                @if (t.caption) {
                  <figcaption>
                    <span>{{ t.caption }}</span>
                  </figcaption>
                }
              </figure>
            }
          }
          @case ('video') {
            @if (video(n); as v) {
              <figure class="embed video">
                <app-article-video [provider]="v.provider" [videoId]="v.id" [caption]="v.caption" />
                @if (v.caption) {
                  <figcaption>
                    <span>{{ v.caption }}</span>
                  </figcaption>
                }
              </figure>
            }
          }
        }
      }
    </ng-template>

    <ng-template #inline let-nodes>
      @for (n of nodes ?? []; track $index) {
        @if (n.type === 'text') {
          <ng-container
            *ngTemplateOutlet="marked; context: { $implicit: n.text, marks: marksOf(n), i: 0 }"
          />
        } @else if (n.type === 'hardBreak') {
          <br />
        }
      }
    </ng-template>

    <ng-template #marked let-text let-marks="marks" let-i="i">
      @if (i >= marks.length) {
        <ng-container>{{ text }}</ng-container>
      } @else {
        @switch (marks[i].type) {
          @case ('bold') {
            <strong
              ><ng-container
                *ngTemplateOutlet="marked; context: { $implicit: text, marks: marks, i: i + 1 }"
            /></strong>
          }
          @case ('italic') {
            <em
              ><ng-container
                *ngTemplateOutlet="marked; context: { $implicit: text, marks: marks, i: i + 1 }"
            /></em>
          }
          @case ('strike') {
            <s
              ><ng-container
                *ngTemplateOutlet="marked; context: { $implicit: text, marks: marks, i: i + 1 }"
            /></s>
          }
          @case ('underline') {
            <u
              ><ng-container
                *ngTemplateOutlet="marked; context: { $implicit: text, marks: marks, i: i + 1 }"
            /></u>
          }
          @case ('link') {
            <a
              [href]="marks[i].href"
              [attr.target]="marks[i].external ? '_blank' : null"
              [attr.rel]="marks[i].external ? 'noopener noreferrer' : null"
              ><ng-container
                *ngTemplateOutlet="marked; context: { $implicit: text, marks: marks, i: i + 1 }"
            /></a>
          }
        }
      }
    </ng-template>

    <div class="prose">
      <ng-container *ngTemplateOutlet="blocks; context: { $implicit: doc().content }" />
    </div>
  `,
  styles: `
    :host {
      display: block;
    }
    .prose {
      font-family: var(--font-display);
      font-size: 1.1875rem;
      line-height: 1.7;
      color: var(--ink);
    }
    .prose > * {
      max-width: var(--measure);
      margin-inline: auto;
    }
    p,
    ul,
    ol,
    blockquote,
    figure,
    hr {
      margin-block: 0 1.35em;
    }
    p {
      max-width: var(--measure);
    }
    h2,
    h3,
    h4 {
      margin-block: 1.8em 0.6em;
      scroll-margin-top: 6rem;
    }
    h2 {
      font-size: var(--text-2xl);
    }
    h3 {
      font-size: var(--text-xl);
    }
    h4 {
      font-size: var(--text-xs);
    }
    ul,
    ol {
      padding-left: 1.4em;
    }
    li > p {
      margin-bottom: 0.4em;
    }
    li::marker {
      color: var(--accent);
    }
    blockquote {
      border-left: 3px solid var(--accent);
      padding: 0.1em 0 0.1em 1.1em;
      color: var(--ink-soft);
      font-size: 1.3rem;
      font-style: italic;
    }
    blockquote p:last-child {
      margin-bottom: 0;
    }
    hr {
      border: 0;
      text-align: center;
      color: var(--muted);
      height: 1.5em;
    }
    hr::before {
      content: '· · ·';
      letter-spacing: 0.6em;
    }
    a {
      color: var(--accent);
      text-decoration-thickness: 1px;
    }
    .embed {
      margin-inline: auto;
    }
    .embed.wide {
      max-width: min(100%, 60rem);
    }
    .embed img {
      display: block;
      width: 100%;
      height: auto;
      border-radius: var(--radius-lg);
      background: var(--surface-2);
    }
    figcaption {
      display: flex;
      flex-wrap: wrap;
      gap: 0.25em 0.75em;
      margin-top: var(--space-2);
      font-family: var(--font-body);
      font-size: var(--text-sm);
      line-height: 1.5;
      color: var(--muted);
    }
    .credit {
      font-size: var(--text-xs);
      text-transform: uppercase;
      letter-spacing: 0.06em;
    }
    .broken,
    .board-placeholder {
      display: grid;
      place-items: center;
      aspect-ratio: 16 / 9;
      border-radius: var(--radius-lg);
      font-family: var(--font-body);
      font-size: var(--text-sm);
    }
    .broken {
      aspect-ratio: 3 / 1;
      border: 1px dashed var(--line);
      color: var(--muted);
    }
    .board-placeholder {
      background: radial-gradient(ellipse at center, #3f9b4c 0%, #24572c 100%);
      color: #ecfdf5;
    }
  `,
})
export class ArticleBody {
  readonly doc = input.required<ArticleDoc>();

  private readonly ids = computed(() => outline(this.doc()).ids);
  protected readonly failed = signal<ReadonlySet<string>>(new Set());

  protected idFor(node: DocNode): string | null {
    return this.ids().get(node) ?? null;
  }

  protected hasText(node: DocNode): boolean {
    return !!node.content?.length && plainText(node).trim().length > 0;
  }

  protected startOf(node: DocNode): number | null {
    const start = Number(node.attrs?.['start'] ?? 1);
    return Number.isInteger(start) && start !== 1 ? start : null;
  }

  /** The marks on a text node, link addresses checked, unknown marks dropped. */
  protected marksOf(node: DocNode): InlineMark[] {
    const out: InlineMark[] = [];
    for (const m of node.marks ?? ([] as DocMark[])) {
      if (m.type === 'link') {
        const href = safeLink(m.attrs?.href);
        // A link that fails the check is drawn as its text, not dropped.
        if (href) out.push({ type: 'link', href, external: isExternal(href) });
      } else if (['bold', 'italic', 'strike', 'underline'].includes(m.type)) {
        out.push({ type: m.type, href: null, external: false });
      }
    }
    // The link outermost, so a part-bold link is still one link.
    return out.sort((a, b) => Number(b.type === 'link') - Number(a.type === 'link'));
  }

  protected figure(node: DocNode): FigureAttrs | null {
    const a = node.attrs ?? {};
    const src = safeImage(a['src']);
    if (!src) return null;
    return {
      src,
      alt: text(a['alt']),
      caption: text(a['caption']),
      credit: text(a['credit']),
      size: a['size'] === 'wide' ? 'wide' : 'column',
    };
  }

  protected tactic(node: DocNode): TacticAttrs | null {
    const slug = text(node.attrs?.['slug']);
    if (!/^[2-9a-kmnp-zA-HJ-NP-Z]{12}$/.test(slug)) return null;
    return { slug, title: text(node.attrs?.['title']), caption: text(node.attrs?.['caption']) };
  }

  protected video(node: DocNode): VideoAttrs | null {
    const provider = text(node.attrs?.['provider']) as VideoProvider;
    const id = text(node.attrs?.['id']);
    if (provider !== 'youtube' && provider !== 'vimeo') return null;
    return { provider, id, caption: text(node.attrs?.['caption']) };
  }

  protected markFailed(src: string): void {
    this.failed.update((s) => new Set(s).add(src));
  }
}

function text(v: unknown): string {
  return typeof v === 'string' ? v : '';
}
