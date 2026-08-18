import { TestBed } from '@angular/core/testing';

import { TEAM_EDITOR_READER, TeamEditorReader } from '../api/contracts';
import { Role, TokenPair, User } from '../models/identity';
import { Permissions } from './permissions';
import { SessionStore } from './session-store';

/**
 * The write rules, mirrored from `Authorizer` in football-svc.
 *
 * These are asserted because getting one wrong is invisible until someone hits
 * a 403 on a button the UI offered them — or, worse, never sees a control they
 * were entitled to use and concludes the feature does not exist.
 */

/** Stands in for the HTTP repository. The point of the token indirection (§4). */
function editorsHolding(...teamIds: string[]): TeamEditorReader {
  return {
    teamsFor: () => Promise.resolve(teamIds),
    editors: () => Promise.reject(new Error('not used')),
  };
}

const tokens: TokenPair = {
  access_token: 'header.body.signature',
  refresh_token: 'refresh',
  token_type: 'Bearer',
  expires_in: 900,
};

function userWith(role: Role): User {
  return {
    id: 'user-1',
    username: 'someone',
    email: 'someone@example.com',
    role,
    created_at: '2026-01-01T00:00:00Z',
  };
}

async function signedInAs(role: Role, grants: string[] = []): Promise<Permissions> {
  TestBed.configureTestingModule({
    providers: [{ provide: TEAM_EDITOR_READER, useValue: editorsHolding(...grants) }],
  });

  const session = TestBed.inject(SessionStore);
  const permissions = TestBed.inject(Permissions);

  session.start(tokens, userWith(role));

  // The grants load is triggered by an effect on the session, so the effect has
  // to run and its request settle before the answers are meaningful.
  TestBed.tick();
  await Promise.resolve();
  await Promise.resolve();
  TestBed.tick();

  return permissions;
}

describe('Permissions', () => {
  afterEach(() => TestBed.resetTestingModule());

  describe('an administrator', () => {
    it('may edit any club without holding a grant', async () => {
      const permissions = await signedInAs('admin');

      expect(permissions.canEditTeam('any-club')).toBe(true);
      expect(permissions.canAdminister()).toBe(true);
    });

    it('may act on records belonging to no club', async () => {
      // A free agent or an unattached coach. RequireTargetTeam(nil) falls
      // through to RequireAdmin, because no club grant can cover "no club".
      const permissions = await signedInAs('admin');

      expect(permissions.canEditTeam(null)).toBe(true);
      expect(permissions.canEditPlayer({ team_id: null })).toBe(true);
    });
  });

  describe('an editor', () => {
    it('may edit a club they hold', async () => {
      const permissions = await signedInAs('editor', ['club-a']);

      expect(permissions.canEditTeam('club-a')).toBe(true);
    });

    it('may not edit a club they do not hold', async () => {
      const permissions = await signedInAs('editor', ['club-a']);

      expect(permissions.canEditTeam('club-b')).toBe(false);
    });

    it('may not touch a record belonging to no club', async () => {
      // The rule that shapes the player form: an editor cannot create a free
      // agent, because there is no club for their grant to apply to. It is why
      // the club field is restricted rather than merely offered.
      const permissions = await signedInAs('editor', ['club-a']);

      expect(permissions.canEditTeam(null)).toBe(false);
      expect(permissions.canEditPlayer({ team_id: null })).toBe(false);
    });

    it('may file a move out of a club they hold', async () => {
      const permissions = await signedInAs('editor', ['club-a']);

      expect(permissions.canMoveBetween('club-a', 'club-b')).toBe(true);
    });

    it('may file a move into a club they hold', async () => {
      // Either end authorises the move: releasing a player and signing one are
      // the same event, so the buying club's editor can record it even though
      // they hold nothing at the selling end.
      const permissions = await signedInAs('editor', ['club-b']);

      expect(permissions.canMoveBetween('club-a', 'club-b')).toBe(true);
    });

    it('may not file a move between two clubs they hold neither of', async () => {
      const permissions = await signedInAs('editor', ['club-c']);

      expect(permissions.canMoveBetween('club-a', 'club-b')).toBe(false);
    });

    it('may not administer', async () => {
      const permissions = await signedInAs('editor', ['club-a']);

      expect(permissions.canAdminister()).toBe(false);
    });
  });

  describe('an ordinary user', () => {
    it('may write nothing', async () => {
      const permissions = await signedInAs('user');

      expect(permissions.canEditTeam('club-a')).toBe(false);
      expect(permissions.canEditTeam(null)).toBe(false);
      expect(permissions.canMoveBetween('club-a', 'club-b')).toBe(false);
      expect(permissions.canAdminister()).toBe(false);
    });
  });

  describe('nobody signed in', () => {
    it('may write nothing, and asks the API for nothing', async () => {
      TestBed.configureTestingModule({
        providers: [
          {
            provide: TEAM_EDITOR_READER,
            useValue: {
              teamsFor: () => Promise.reject(new Error('must not be called')),
              editors: () => Promise.reject(new Error('not used')),
            } satisfies TeamEditorReader,
          },
        ],
      });

      const permissions = TestBed.inject(Permissions);
      TestBed.tick();

      expect(permissions.canEditTeam('club-a')).toBe(false);
      expect(permissions.grantedTeamIds()).toEqual([]);
    });
  });

  describe('when the grant lookup fails', () => {
    it('hides write controls rather than offering ones that would 403', async () => {
      TestBed.configureTestingModule({
        providers: [
          {
            provide: TEAM_EDITOR_READER,
            useValue: {
              teamsFor: () => Promise.reject(new Error('gateway down')),
              editors: () => Promise.reject(new Error('not used')),
            } satisfies TeamEditorReader,
          },
        ],
      });

      const session = TestBed.inject(SessionStore);
      const permissions = TestBed.inject(Permissions);

      session.start(tokens, userWith('editor'));
      TestBed.tick();
      await Promise.resolve();
      await Promise.resolve();
      TestBed.tick();

      expect(permissions.canEditTeam('club-a')).toBe(false);
      // The failure is not fatal: the page still renders, read-only, and the
      // API remains the authority on anything the user does attempt.
      expect(permissions.ready()).toBe(true);
    });
  });
});
