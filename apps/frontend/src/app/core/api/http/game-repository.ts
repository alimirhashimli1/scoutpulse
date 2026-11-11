import { Injectable } from '@angular/core';

import { GameReader } from '../games';
import { GameSummary, PlayedForBothWire } from '../../models/games';
import { HttpRepository } from './http-repository';

/** Bumped when the dataset gains or loses a field. */
const DATASET_SHAPE = 2;

@Injectable()
export class HttpGameRepository extends HttpRepository implements GameReader {
  protected readonly base = this.api.football;

  /**
   * The dataset is a static file that only changes with a deploy, so one
   * download serves the whole session: leaving the game and coming back does
   * not fetch it again. A failure is not cached, so Retry really retries.
   * (Across sessions the browser's HTTP cache and the ETag take over.)
   */
  private dataset: Promise<PlayedForBothWire> | null = null;

  list(): Promise<GameSummary[]> {
    return this.getOne<GameSummary[]>(this.url('games'));
  }

  playedForBothDataset(): Promise<PlayedForBothWire> {
    if (!this.dataset) {
      // `v` names the shape of the file, not its contents: a browser keeps
      // the file for a day without asking, so a copy from before the
      // difficulty levels were added would otherwise outlive the deploy.
      this.dataset = this.getOne<PlayedForBothWire>(
        this.url('games', 'played-for-both', 'dataset'),
        { v: DATASET_SHAPE },
      ).catch((err: unknown) => {
        this.dataset = null;
        throw err;
      });
    }
    return this.dataset;
  }
}
