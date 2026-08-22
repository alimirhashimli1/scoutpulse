import { TestBed } from '@angular/core/testing';

import { PLAYER_READER, PlayerFilter, PlayerReader } from './contracts';
import { Page } from './page';
import { Player } from '../models/football';
import { PlayerNames } from './player-names';

/**
 * The transfer feed's player column.
 *
 * Every row carries a `player_id` and no name, so the names have to be fetched
 * separately. The whole point of this class is that "separately" means **one**
 * request for the page, not one per row — twenty-five round trips during the
 * server render of the landing page is the failure it exists to prevent.
 */

function player(id: string, name: string): Player {
  return {
    id,
    team_id: null,
    name,
    position: 'Centre-Forward',
    // The list fields are non-optional and the API always sends them, so the
    // stub does too — a test fixture that is not a valid Player would be
    // testing something the app never sees.
    nationalities: [],
    secondary_positions: [],
    strengths: [],
    weaknesses: [],
    market_value_minor: 0,
    currency: 'EUR',
    created_at: '2026-01-01T00:00:00Z',
  };
}

/** Records every call, so the request count itself can be asserted. */
class RecordingReader implements PlayerReader {
  readonly calls: PlayerFilter[] = [];
  constructor(private readonly known: Map<string, string>) {}

  list(filter?: PlayerFilter): Promise<Page<Player>> {
    this.calls.push(filter ?? {});
    const items = (filter?.ids ?? [])
      .filter((id) => this.known.has(id))
      .map((id) => player(id, this.known.get(id)!));
    return Promise.resolve({ items, limit: 100, offset: 0, has_more: false });
  }

  byId(): Promise<Player> {
    // Failing loudly rather than returning something: reaching for this is the
    // N+1 regression, and a silent fallback would hide it.
    return Promise.reject(new Error('byId must not be used to resolve a feed page'));
  }
  transfers(): Promise<Page<never>> {
    return Promise.reject(new Error('not used'));
  }
  marketValues(): Promise<Page<never>> {
    return Promise.reject(new Error('not used'));
  }
}

describe('PlayerNames', () => {
  let reader: RecordingReader;
  let names: PlayerNames;

  beforeEach(() => {
    reader = new RecordingReader(
      new Map([
        ['p1', 'Erling Haaland'],
        ['p2', 'Phil Foden'],
        ['p3', 'Rodri'],
      ]),
    );
    TestBed.configureTestingModule({
      providers: [{ provide: PLAYER_READER, useValue: reader }],
    });
    names = TestBed.inject(PlayerNames);
  });

  afterEach(() => TestBed.resetTestingModule());

  it('resolves a whole page in one request', async () => {
    await names.resolve(['p1', 'p2', 'p3']);

    expect(reader.calls.length).toBe(1);
    expect(reader.calls[0].ids).toEqual(['p1', 'p2', 'p3']);
    expect(names.name('p1')).toBe('Erling Haaland');
    expect(names.name('p3')).toBe('Rodri');
  });

  it('does not re-request ids it already knows', async () => {
    await names.resolve(['p1', 'p2']);
    await names.resolve(['p1', 'p2']);

    expect(reader.calls.length).toBe(1);
  });

  it('asks only for the ids it is missing', async () => {
    await names.resolve(['p1']);
    await names.resolve(['p1', 'p2']);

    expect(reader.calls.length).toBe(2);
    expect(reader.calls[1].ids).toEqual(['p2']);
  });

  it('de-duplicates repeats within one page', async () => {
    // A player can appear twice in a feed — a loan out and the return.
    await names.resolve(['p1', 'p1', 'p2']);

    expect(reader.calls[0].ids).toEqual(['p1', 'p2']);
  });

  it('makes no request when there is nothing to resolve', async () => {
    await names.resolve([]);
    await names.resolve([null, undefined]);

    expect(reader.calls.length).toBe(0);
  });

  it('issues one request when the same ids are asked for concurrently', async () => {
    // Two components rendering the same feed must not both fetch.
    await Promise.all([names.resolve(['p1', 'p2']), names.resolve(['p1', 'p2'])]);

    expect(reader.calls.length).toBe(1);
  });

  it('renders a fallback for an id the API did not return', async () => {
    await names.resolve(['p1', 'missing']);

    expect(names.name('missing')).toBe('Unknown player');
    expect(names.name(null)).toBe('Unknown player');
  });

  it('survives a failed lookup without breaking the page', async () => {
    const failing = new RecordingReader(new Map());
    failing.list = () => Promise.reject(new Error('gateway down'));

    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [{ provide: PLAYER_READER, useValue: failing }],
    });
    const store = TestBed.inject(PlayerNames);

    await expect(store.resolve(['p1'])).resolves.toBeUndefined();
    expect(store.name('p1')).toBe('Unknown player');
  });
});
