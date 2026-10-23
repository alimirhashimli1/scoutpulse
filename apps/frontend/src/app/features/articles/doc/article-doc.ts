/**
 * The article document, and everything the editor and the reader agree on
 * about it.
 *
 * A body is ProseMirror JSON: what the editor produces natively, and a tree
 * the reader walks with Angular templates. Nothing in it is ever handed to the
 * page as HTML. The reader draws the node types listed here and skips anything
 * else, and every address is checked again on the way out by the same rules
 * football-svc applies on the way in (`article_body.go`), so the two cannot
 * disagree about what is safe.
 */

export type MarkType = 'bold' | 'italic' | 'strike' | 'underline' | 'link';

export interface DocMark {
  type: MarkType | string;
  attrs?: { href?: string | null } & Record<string, unknown>;
}

export interface DocNode {
  type: string;
  attrs?: Record<string, unknown>;
  content?: DocNode[];
  marks?: DocMark[];
  text?: string;
}

/** The root node. */
export type ArticleDoc = DocNode;

export function emptyArticleDoc(): ArticleDoc {
  return { type: 'doc', content: [{ type: 'paragraph' }] };
}

/** Attributes of the three embeds this site adds to the editor. */
export interface FigureAttrs {
  src: string;
  alt: string;
  caption: string;
  credit: string;
  /** `wide` breaks out of the text column on a wide screen. */
  size: 'column' | 'wide';
}
export interface TacticAttrs {
  slug: string;
  title: string;
  caption: string;
}
export type VideoProvider = 'youtube' | 'vimeo';
export interface VideoAttrs {
  provider: VideoProvider;
  id: string;
  caption: string;
}

// --- addresses -------------------------------------------------------------

const MAX_URL = 2048;

/**
 * A link a reader may follow: http(s), mailto, or a path on this site. Null
 * for anything else -- javascript:, data:, a protocol-relative //host.
 */
export function safeLink(raw: unknown): string | null {
  if (typeof raw !== 'string' || !raw || raw.length > MAX_URL || /\s/.test(raw)) return null;
  if (raw.startsWith('/')) return raw.startsWith('//') || raw.startsWith('/\\') ? null : raw;
  try {
    const url = new URL(raw);
    if ((url.protocol === 'http:' || url.protocol === 'https:') && url.host) return raw;
    if (url.protocol === 'mailto:' && url.pathname) return raw;
  } catch {
    // Not an absolute URL, and not a path: refused.
  }
  return null;
}

/** An image the page may load: https only, so it is never mixed content. */
export function safeImage(raw: unknown): string | null {
  if (typeof raw !== 'string' || !raw || raw.length > MAX_URL || /\s/.test(raw)) return null;
  try {
    const url = new URL(raw);
    return url.protocol === 'https:' && url.host ? raw : null;
  } catch {
    return null;
  }
}

/** Whether a link leaves the site, and so opens in a new tab. */
export function isExternal(href: string): boolean {
  return !href.startsWith('/');
}

/**
 * Accepts what a writer is likely to paste: a full address, a bare host and
 * path, or a path on this site. Returns null when nothing safe can be made of
 * it, so the dialog can say so instead of saving a dead link.
 */
export function normaliseLink(input: string): string | null {
  const raw = input.trim();
  if (!raw) return null;
  if (raw.startsWith('/')) return safeLink(raw);
  if (/^[a-z][a-z0-9+.-]*:/i.test(raw)) return safeLink(raw);
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw)) return safeLink(`mailto:${raw}`);
  return safeLink(`https://${raw}`);
}

const YOUTUBE_ID = /^[A-Za-z0-9_-]{11}$/;
const VIMEO_ID = /^[0-9]{6,12}$/;

/** A YouTube or Vimeo address, in any of the forms their share buttons produce. */
export function parseVideo(input: string): { provider: VideoProvider; id: string } | null {
  let url: URL;
  try {
    url = new URL(input.trim());
  } catch {
    return null;
  }
  const host = url.hostname.replace(/^(www\.|m\.)/, '');
  const parts = url.pathname.split('/').filter(Boolean);

  if (host === 'youtu.be' && YOUTUBE_ID.test(parts[0] ?? '')) {
    return { provider: 'youtube', id: parts[0] };
  }
  if (host === 'youtube.com' || host === 'youtube-nocookie.com') {
    const v = url.searchParams.get('v');
    if (v && YOUTUBE_ID.test(v)) return { provider: 'youtube', id: v };
    if (['embed', 'shorts', 'live', 'v'].includes(parts[0]) && YOUTUBE_ID.test(parts[1] ?? '')) {
      return { provider: 'youtube', id: parts[1] };
    }
  }
  if (host === 'vimeo.com' || host === 'player.vimeo.com') {
    const id = parts.find((p) => VIMEO_ID.test(p));
    if (id) return { provider: 'vimeo', id };
  }
  return null;
}

/** Where the player loads from. YouTube's no-cookie host sets nothing until play. */
export function videoEmbedUrl(provider: VideoProvider, id: string): string | null {
  if (provider === 'youtube' && YOUTUBE_ID.test(id)) {
    return `https://www.youtube-nocookie.com/embed/${id}?autoplay=1&rel=0`;
  }
  if (provider === 'vimeo' && VIMEO_ID.test(id)) {
    return `https://player.vimeo.com/video/${id}?autoplay=1&dnt=1`;
  }
  return null;
}

/** A still for the click-to-play poster, where the provider offers one by id. */
export function videoPoster(provider: VideoProvider, id: string): string | null {
  return provider === 'youtube' && YOUTUBE_ID.test(id)
    ? `https://i.ytimg.com/vi/${id}/hqdefault.jpg`
    : null;
}

/** Mirrors football-svc's slug alphabet for tactics boards. */
const TACTIC_SLUG = /^[2-9a-kmnp-zA-HJ-NP-Z]{12}$/;

/** A board's slug from its share link, its edit link, or the slug itself. */
export function parseTacticSlug(input: string): string | null {
  const raw = input.trim();
  if (TACTIC_SLUG.test(raw)) return raw;
  const match = raw.match(/\/tactics\/([^/?#]+)/);
  return match && TACTIC_SLUG.test(match[1]) ? match[1] : null;
}

// --- reading the tree ------------------------------------------------------

export function plainText(node: DocNode): string {
  if (node.type === 'text') return node.text ?? '';
  if (node.type === 'hardBreak') return ' ';
  // Inside a line of text, pieces join as they are; between blocks there is a
  // break, or the last word of one paragraph runs into the first of the next.
  const inline = node.type === 'paragraph' || node.type === 'heading';
  return (node.content ?? []).map(plainText).join(inline ? '' : ' ');
}

export function wordCount(doc: ArticleDoc): number {
  return plainText(doc).split(/\s+/).filter(Boolean).length;
}

export interface OutlineEntry {
  id: string;
  text: string;
  level: number;
}

/**
 * The article's sections, for the contents list and for heading anchors.
 *
 * Ids come from the heading text, so a link to a section survives edits
 * elsewhere in the article; a repeated heading gets a number.
 */
export function outline(doc: ArticleDoc): { entries: OutlineEntry[]; ids: Map<DocNode, string> } {
  const entries: OutlineEntry[] = [];
  const ids = new Map<DocNode, string>();
  const used = new Map<string, number>();
  for (const node of doc.content ?? []) {
    if (node.type !== 'heading') continue;
    const text = plainText(node).trim();
    if (!text) continue;
    const base =
      text
        .toLowerCase()
        .normalize('NFKD')
        .replace(/[̀-ͯ]/g, '')
        .replace(/[^\p{L}\p{N}]+/gu, '-')
        .replace(/^-+|-+$/g, '')
        .slice(0, 60) || 'section';
    const n = (used.get(base) ?? 0) + 1;
    used.set(base, n);
    const id = n === 1 ? base : `${base}-${n}`;
    ids.set(node, id);
    entries.push({ id, text, level: Number(node.attrs?.['level'] ?? 2) });
  }
  return { entries, ids };
}

/** True when the document has nothing a reader would see. */
export function isEmptyDoc(doc: ArticleDoc): boolean {
  return !(doc.content ?? []).some(
    (n) => plainText(n).trim() || ['figure', 'tacticBoard', 'video'].includes(n.type),
  );
}
