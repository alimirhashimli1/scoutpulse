import { Injectable } from '@angular/core';

import { Page } from '../page';
import { TacticFilter, TacticInput, TacticReader, TacticWriter } from '../tactics';
import { TacticBoard, TacticSummary } from '../../models/tactics';
import { HttpRepository } from './http-repository';

@Injectable()
export class HttpTacticRepository extends HttpRepository implements TacticReader, TacticWriter {
  protected readonly base = this.api.football;

  bySlug(slug: string): Promise<TacticBoard> {
    return this.getOne<TacticBoard>(this.url('tactics', slug));
  }

  list(filter?: TacticFilter): Promise<Page<TacticSummary>> {
    return this.getPage<TacticSummary>(this.url('tactics'), filter);
  }

  create(input: TacticInput): Promise<TacticBoard> {
    return this.post<TacticBoard>(this.url('tactics'), input);
  }

  update(slug: string, input: TacticInput): Promise<TacticBoard> {
    return this.put<TacticBoard>(this.url('tactics', slug), input);
  }

  remove(slug: string): Promise<void> {
    return this.deleteAt(this.url('tactics', slug));
  }
}
