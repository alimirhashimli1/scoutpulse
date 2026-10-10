import { InjectionToken } from '@angular/core';

import { Page, PageQuery } from './page';
import { TacticBoard, TacticSummary } from '../models/tactics';

/**
 * Members' tactics boards.
 *
 * Kept in its own file rather than in contracts.ts, the way the user-admin
 * contract is: it is a separate feature that happens to be served by the
 * football service, not part of the football record.
 */

export interface TacticFilter extends PageQuery {
  /** Narrows the list to one author: how "my boards" is asked for. */
  author_id?: string;
}

export interface TacticReader {
  /** One board with its document. Public: holding the link is the access. */
  bySlug(slug: string): Promise<TacticBoard>;
  /** Summaries grouped by author. Members only. */
  list(filter?: TacticFilter): Promise<Page<TacticSummary>>;
}

export interface TacticInput {
  title: string;
  description: string;
  content: unknown;
}

export interface TacticWriter {
  create(input: TacticInput): Promise<TacticBoard>;
  update(slug: string, input: TacticInput): Promise<TacticBoard>;
  remove(slug: string): Promise<void>;
}

export const TACTIC_READER = new InjectionToken<TacticReader>('TacticReader');
export const TACTIC_WRITER = new InjectionToken<TacticWriter>('TacticWriter');
