import { DOCUMENT, Injectable, inject } from '@angular/core';
import { Meta, Title } from '@angular/platform-browser';

import { SITE_URL } from '../tokens/site-url';

export interface PageDescription {
  /** Without the site name — that is appended here, once, in one place. */
  title: string;
  description: string;
  /** Absolute path, leading slash. Becomes the canonical and `og:url`. */
  path: string;
  /** `profile` for a person, `website` for a listing. */
  type?: 'website' | 'profile' | 'article';
  /**
   * Keep this page out of search results.
   *
   * For anything whose URL space is unbounded or personal: search results with
   * arbitrary queries, paginated slices, account and admin screens.
   */
  noindex?: boolean;
}

/**
 * Everything a crawler and a link preview read.
 *
 * This is the payoff for choosing SSR. A client-rendered page can set all of
 * these too — but only *after* its JavaScript has run and its data has
 * arrived, which is too late for a link unfurler and unreliable for a crawler.
 * Server-rendered, they are in the HTML of the first response.
 *
 * Called from a component's constructor or an effect on its loaded data.
 * Angular's final change detection runs before the server serialises the
 * document, so a title set from a resource is in the delivered markup.
 */
@Injectable({ providedIn: 'root' })
export class Seo {
  private readonly title = inject(Title);
  private readonly meta = inject(Meta);
  private readonly document = inject(DOCUMENT);
  private readonly origin = inject(SITE_URL);

  private static readonly SITE_NAME = 'ScoutPulse';
  private static readonly LD_ID = 'structured-data';

  describe(page: PageDescription): void {
    const url = `${this.origin}${page.path}`;
    const fullTitle = `${page.title} · ${Seo.SITE_NAME}`;

    this.title.setTitle(fullTitle);

    // updateTag matches on the selector and replaces, so navigating between
    // two players leaves one description rather than accumulating one per
    // visit — which is what addTag would do.
    this.meta.updateTag({ name: 'description', content: page.description });
    this.meta.updateTag({ property: 'og:title', content: fullTitle });
    this.meta.updateTag({ property: 'og:description', content: page.description });
    this.meta.updateTag({ property: 'og:url', content: url });
    this.meta.updateTag({ property: 'og:type', content: page.type ?? 'website' });
    this.meta.updateTag({ property: 'og:site_name', content: Seo.SITE_NAME });

    // Twitter reads og:* for everything except the card style itself.
    this.meta.updateTag({ name: 'twitter:card', content: 'summary' });

    if (page.noindex) {
      this.meta.updateTag({ name: 'robots', content: 'noindex, follow' });
    } else {
      // Removed rather than set to "index": leaving a stale noindex behind
      // after navigating from /search to a player would deindex the player.
      this.meta.removeTag("name='robots'");
    }

    this.setCanonical(url);
  }

  /**
   * Attaches JSON-LD.
   *
   * Not through `Meta`, which only manages `<meta>` elements — this is a
   * `<script type="application/ld+json">`, so the element is managed directly.
   * It carries a fixed id so a navigation replaces the previous page's data
   * instead of leaving two contradictory descriptions in the head.
   */
  structuredData(data: object | null): void {
    const head = this.document.head;
    const existing = head.querySelector(`script#${Seo.LD_ID}`);
    if (existing) existing.remove();
    if (!data) return;

    const script = this.document.createElement('script');
    script.id = Seo.LD_ID;
    script.type = 'application/ld+json';
    // textContent, never innerHTML: the values are names typed by users, and
    // this is the one place a page writes into the document as markup.
    script.textContent = JSON.stringify(data);
    head.appendChild(script);
  }

  private setCanonical(url: string): void {
    const head = this.document.head;
    let link = head.querySelector<HTMLLinkElement>("link[rel='canonical']");

    if (!link) {
      link = this.document.createElement('link');
      link.setAttribute('rel', 'canonical');
      head.appendChild(link);
    }

    link.setAttribute('href', url);
  }
}
