import { Editor, Node } from '@tiptap/core';
import type { NodeViewRendererProps } from '@tiptap/core';
import type { Node as PmNode } from '@tiptap/pm/model';
import { Placeholder } from '@tiptap/extensions';
import StarterKit from '@tiptap/starter-kit';

import {
  ArticleDoc,
  FigureAttrs,
  TacticAttrs,
  VideoAttrs,
  normaliseLink,
  parseTacticSlug,
  parseVideo,
  safeImage,
  safeLink,
  videoPoster,
} from '../doc/article-doc';
import { iconSvg } from '../ui/article-icon';

/**
 * The writing surface, built on Tiptap (ProseMirror).
 *
 * Loaded with a dynamic import from the editor page, so none of it is in any
 * bundle a reader downloads, and it never runs on the server.
 *
 * The schema is exactly what the reader can draw (see article-doc.ts and
 * football-svc's article_body.go): the starter kit without code, plus three
 * embeds. Pasted HTML is parsed into that schema, so formatting from another
 * site comes across as headings, lists and links, and anything else is
 * dropped rather than smuggled in.
 */

/** What an embed's card asks of the page: open a dialog, get new attributes. */
export interface EmbedHooks {
  editFigure(attrs: FigureAttrs): Promise<FigureAttrs | null>;
  editTactic(attrs: TacticAttrs): Promise<TacticAttrs | null>;
  editVideo(attrs: VideoAttrs): Promise<VideoAttrs | null>;
}

export interface ArticleEditorOptions {
  element: HTMLElement;
  content: ArticleDoc;
  editable: boolean;
  hooks: EmbedHooks;
  onChange(doc: ArticleDoc): void;
  onTransaction(): void;
}

export function createArticleEditor(options: ArticleEditorOptions): Editor {
  const editor: Editor = new Editor({
    element: options.element,
    editable: options.editable,
    content: options.content,
    extensions: [
      StarterKit.configure({
        heading: { levels: [2, 3, 4] },
        code: false,
        codeBlock: false,
        link: {
          openOnClick: false,
          autolink: true,
          linkOnPaste: true,
          defaultProtocol: 'https',
          // The same rule the API applies; a link it would refuse is never made.
          isAllowedUri: (url) => safeLink(url) !== null,
          HTMLAttributes: { rel: 'noopener noreferrer', target: null },
        },
      }),
      Placeholder.configure({
        placeholder: ({ node }) =>
          node.type.name === 'heading'
            ? 'Section heading'
            : 'Write your analysis… “## ” starts a heading, “> ” a quote, “- ” a list.',
      }),
      Figure.configure({ hooks: options.hooks }),
      TacticBoard.configure({ hooks: options.hooks }),
      Video.configure({ hooks: options.hooks }),
    ],
    editorProps: {
      attributes: { class: 'ae-prose', spellcheck: 'true' },
      handlePaste: (_view, event): boolean => pasteAsEmbed(editor, event),
    },
    onUpdate: ({ editor }) => options.onChange(editor.getJSON() as ArticleDoc),
    onTransaction: () => options.onTransaction(),
  });
  return editor;
}

/**
 * A bare link pasted onto an empty line becomes what it points at: a video,
 * a tactics board, or an image. Pasted anywhere else it stays a link.
 */
function pasteAsEmbed(editor: Editor, event: ClipboardEvent): boolean {
  const text = event.clipboardData?.getData('text/plain')?.trim() ?? '';
  if (!text || /\s/.test(text)) return false;
  const { $from, empty } = editor.state.selection;
  if (!empty || $from.parent.type.name !== 'paragraph' || $from.parent.content.size > 0) {
    return false;
  }

  const video = parseVideo(text);
  if (video) {
    insertEmbed(editor, 'video', { ...video, caption: '' });
    return true;
  }
  const slug = /\/tactics\//.test(text) ? parseTacticSlug(text) : null;
  if (slug) {
    insertEmbed(editor, 'tacticBoard', { slug, title: '', caption: '' });
    return true;
  }
  const image = safeImage(text);
  if (image && /\.(jpe?g|png|gif|webp|avif)$/i.test(new URL(image).pathname)) {
    insertEmbed(editor, 'figure', { src: image, alt: '', caption: '', credit: '', size: 'column' });
    return true;
  }
  return false;
}

/**
 * Inserts an embed and puts the cursor on the line after it.
 *
 * ProseMirror leaves a freshly inserted block selected, so whatever the
 * writer typed or pasted next would replace it -- an image, gone on the first
 * keystroke. The line after always exists: the trailing-node rule adds one
 * when the embed is the last thing in the article.
 */
export function insertEmbed(
  editor: Editor,
  type: 'figure' | 'tacticBoard' | 'video',
  attrs: FigureAttrs | TacticAttrs | VideoAttrs,
): void {
  editor.chain().focus().insertContent({ type, attrs }).run();
  const { selection } = editor.state;
  if ('node' in selection && selection.node) {
    editor
      .chain()
      .setTextSelection(selection.to + 1)
      .scrollIntoView()
      .run();
  }
}

/** Applies a link to the selection, or inserts linked text where there is none. */
export function applyLink(editor: Editor, href: string | null, text?: string): void {
  const chain = editor.chain().focus().extendMarkRange('link');
  if (!href) {
    chain.unsetLink().run();
    return;
  }
  const safe = normaliseLink(href);
  if (!safe) return;
  if (editor.state.selection.empty && text) {
    chain
      .insertContent({ type: 'text', text, marks: [{ type: 'link', attrs: { href: safe } }] })
      .run();
  } else {
    chain.setLink({ href: safe }).run();
  }
}

// --- the embeds --------------------------------------------------------------

interface EmbedOptions {
  hooks: EmbedHooks | null;
}

/** Attributes that live in the document, not as HTML attributes on the node. */
function attrs<T extends object>(defaults: T) {
  return () =>
    Object.fromEntries(
      Object.entries(defaults).map(([k, v]) => [k, { default: v, rendered: false }]),
    );
}

const Figure = Node.create<EmbedOptions>({
  name: 'figure',
  group: 'block',
  atom: true,
  draggable: true,
  selectable: true,
  addOptions: () => ({ hooks: null }),
  addAttributes: attrs<FigureAttrs>({ src: '', alt: '', caption: '', credit: '', size: 'column' }),
  parseHTML: () => [
    {
      tag: 'figure[data-figure]',
      getAttrs: (el) => {
        const src = safeImage(el.querySelector('img')?.getAttribute('src'));
        return src
          ? {
              src,
              alt: el.querySelector('img')?.getAttribute('alt') ?? '',
              caption: el.querySelector('figcaption')?.textContent ?? '',
              credit: el.getAttribute('data-credit') ?? '',
              size: el.getAttribute('data-size') === 'wide' ? 'wide' : 'column',
            }
          : false;
      },
    },
    {
      // An image pasted from another page comes in as a figure, if it is https.
      tag: 'img[src]',
      getAttrs: (el) => {
        const src = safeImage(el.getAttribute('src'));
        return src ? { src, alt: el.getAttribute('alt') ?? '' } : false;
      },
    },
  ],
  renderHTML: ({ node }) => [
    'figure',
    { 'data-figure': '', 'data-size': node.attrs['size'], 'data-credit': node.attrs['credit'] },
    ['img', { src: node.attrs['src'], alt: node.attrs['alt'] }],
    ['figcaption', {}, node.attrs['caption']],
  ],
  addNodeView() {
    const hooks = this.options.hooks;
    return (props) =>
      embedView(props, {
        className: 'ae-figure',
        draw: (node, body) => {
          const a = node.attrs as FigureAttrs;
          const img = document.createElement('img');
          img.src = a.src;
          img.alt = a.alt;
          img.referrerPolicy = 'no-referrer';
          img.draggable = false;
          img.onerror = () => {
            body.replaceChildren(
              note('This image could not be loaded. Check the address, or try another.'),
            );
          };
          body.replaceChildren(img);
          body.parentElement?.classList.toggle('wide', a.size === 'wide');
          return [a.caption, a.credit].filter(Boolean).join(' · ');
        },
        actions: (node, update) => [
          [
            'pen',
            'Edit image',
            async () => {
              const next = await hooks?.editFigure(node().attrs as FigureAttrs);
              if (next) update(next);
            },
          ],
          [
            node().attrs['size'] === 'wide' ? 'narrow' : 'wide',
            node().attrs['size'] === 'wide' ? 'Fit the text column' : 'Make it wide',
            () => update({ size: node().attrs['size'] === 'wide' ? 'column' : 'wide' }),
          ],
        ],
      });
  },
});

const TacticBoard = Node.create<EmbedOptions>({
  name: 'tacticBoard',
  group: 'block',
  atom: true,
  draggable: true,
  selectable: true,
  addOptions: () => ({ hooks: null }),
  addAttributes: attrs<TacticAttrs>({ slug: '', title: '', caption: '' }),
  parseHTML: () => [
    {
      tag: 'div[data-tactic-board]',
      getAttrs: (el) => {
        const slug = parseTacticSlug(el.getAttribute('data-tactic-board') ?? '');
        return slug
          ? {
              slug,
              title: el.getAttribute('data-title') ?? '',
              caption: el.getAttribute('data-caption') ?? '',
            }
          : false;
      },
    },
  ],
  renderHTML: ({ node }) => [
    'div',
    {
      'data-tactic-board': node.attrs['slug'],
      'data-title': node.attrs['title'],
      'data-caption': node.attrs['caption'],
    },
  ],
  addNodeView() {
    const hooks = this.options.hooks;
    return (props) =>
      embedView(props, {
        className: 'ae-board',
        draw: (node, body) => {
          const a = node.attrs as TacticAttrs;
          const card = document.createElement('div');
          card.className = 'ae-board-card';
          card.innerHTML = iconSvg('board');
          const label = document.createElement('div');
          const title = document.createElement('strong');
          title.textContent = a.title || 'Tactics board';
          const sub = document.createElement('span');
          sub.textContent = 'Plays here for readers · preview to watch it';
          label.append(title, sub);
          card.append(label);
          body.replaceChildren(card);
          return a.caption;
        },
        actions: (node, update) => [
          [
            'pen',
            'Change board or caption',
            async () => {
              const next = await hooks?.editTactic(node().attrs as TacticAttrs);
              if (next) update(next);
            },
          ],
        ],
      });
  },
});

const Video = Node.create<EmbedOptions>({
  name: 'video',
  group: 'block',
  atom: true,
  draggable: true,
  selectable: true,
  addOptions: () => ({ hooks: null }),
  addAttributes: attrs<VideoAttrs>({ provider: 'youtube', id: '', caption: '' }),
  parseHTML: () => [
    {
      tag: 'div[data-video]',
      getAttrs: (el) => {
        const provider = el.getAttribute('data-video');
        const id = el.getAttribute('data-id') ?? '';
        return provider === 'youtube' || provider === 'vimeo'
          ? { provider, id, caption: el.getAttribute('data-caption') ?? '' }
          : false;
      },
    },
  ],
  renderHTML: ({ node }) => [
    'div',
    {
      'data-video': node.attrs['provider'],
      'data-id': node.attrs['id'],
      'data-caption': node.attrs['caption'],
    },
  ],
  addNodeView() {
    const hooks = this.options.hooks;
    return (props) =>
      embedView(props, {
        className: 'ae-video',
        draw: (node, body) => {
          const a = node.attrs as VideoAttrs;
          const poster = videoPoster(a.provider, a.id);
          const frame = document.createElement('div');
          frame.className = 'ae-video-frame';
          if (poster) {
            const img = document.createElement('img');
            img.src = poster;
            img.alt = '';
            img.referrerPolicy = 'no-referrer';
            img.draggable = false;
            frame.append(img);
          }
          const badge = document.createElement('span');
          badge.className = 'ae-video-badge';
          badge.textContent = a.provider === 'youtube' ? 'YouTube' : 'Vimeo';
          frame.append(badge);
          body.replaceChildren(frame);
          return a.caption;
        },
        actions: (node, update) => [
          [
            'pen',
            'Edit video',
            async () => {
              const next = await hooks?.editVideo(node().attrs as VideoAttrs);
              if (next) update(next);
            },
          ],
        ],
      });
  },
});

type Action = [icon: string, label: string, run: () => void];

/**
 * The card an embed shows in the editor: its content, its caption, and a
 * small toolbar. Plain DOM, since it lives inside ProseMirror's view rather
 * than Angular's.
 */
function embedView(
  props: NodeViewRendererProps,
  spec: {
    className: string;
    draw: (node: PmNode, body: HTMLElement) => string;
    actions: (node: () => PmNode, update: (attrs: object) => void) => Action[];
  },
) {
  let node = props.node;
  const { editor } = props;

  const dom = document.createElement('div');
  dom.className = `ae-embed ${spec.className}`;
  dom.contentEditable = 'false';
  dom.setAttribute('data-drag-handle', '');
  const body = document.createElement('div');
  body.className = 'ae-embed-body';
  const caption = document.createElement('p');
  caption.className = 'ae-embed-caption';
  const bar = document.createElement('div');
  bar.className = 'ae-embed-bar';
  dom.append(body, caption, bar);

  const update = (attrs: object) => {
    const pos = props.getPos();
    if (typeof pos !== 'number') return;
    editor.view.dispatch(
      editor.state.tr.setNodeMarkup(pos, undefined, { ...node.attrs, ...attrs }),
    );
  };
  const remove = () => {
    const pos = props.getPos();
    if (typeof pos !== 'number') return;
    editor.view.dispatch(editor.state.tr.delete(pos, pos + node.nodeSize));
    editor.commands.focus();
  };

  const render = () => {
    const text = spec.draw(node, body);
    caption.textContent = text || 'No caption';
    caption.classList.toggle('empty', !text);
    const actions: Action[] = [...spec.actions(() => node, update), ['trash', 'Remove', remove]];
    bar.replaceChildren(
      ...actions.map(([icon, label, run]) => {
        const b = document.createElement('button');
        b.type = 'button';
        b.title = label;
        b.setAttribute('aria-label', label);
        b.innerHTML = iconSvg(icon);
        b.addEventListener('click', (e) => {
          e.preventDefault();
          if (editor.isEditable) run();
        });
        return b;
      }),
    );
  };
  render();

  return {
    dom,
    update(next: PmNode) {
      if (next.type !== node.type) return false;
      node = next;
      render();
      return true;
    },
    selectNode: () => dom.classList.add('selected'),
    deselectNode: () => dom.classList.remove('selected'),
    // The toolbar's clicks are the card's own, not a selection in the text.
    stopEvent: (event: Event) => bar.contains(event.target as globalThis.Node),
    ignoreMutation: () => true,
  };
}

function note(text: string): HTMLElement {
  const p = document.createElement('p');
  p.className = 'ae-embed-error';
  p.textContent = text;
  return p;
}
