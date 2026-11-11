import { GameLang, GameStrings, clubName, nationName } from '../games-i18n';
import { GameIndex, Player, isClub, matches, shortCode } from './played-for-both.engine';

/**
 * Turning entities into what the page shows. Shared by the same-screen game
 * and the online one, which draw the same pitch from the same dataset.
 */

/** How many answer chips to show before "+N more". */
export const SHOWN_ANSWERS = 40;

/** One side of the pitch, ready to render. */
export interface SideView {
  kind: 'club' | 'nation';
  label: string;
  /** "Club · England" or "National team". */
  pill: string;
  /** For clubs. */
  code?: string;
  colour1?: string;
  colour2?: string;
  /** For nations: the flag-icons code. */
  flag?: string;
  /** Accessible name of the badge or flag. */
  alt: string;
}

export interface AnswerView {
  shown: Player[];
  more: number;
  /** The "no known answer" message, when there are none. */
  none: string | null;
}

export function entityLabel(ix: GameIndex, e: number, lang: GameLang): string {
  return isClub(ix, e)
    ? clubName(ix.clubs[e], lang)
    : nationName(ix.nations[e - ix.clubCount], lang);
}

/** Whether an entity index from the server exists in this copy of the data. */
export function knownEntity(ix: GameIndex, e: number): boolean {
  return Number.isInteger(e) && e >= 0 && e < ix.clubCount + ix.nations.length;
}

export function sideView(ix: GameIndex, e: number, t: GameStrings, lang: GameLang): SideView {
  const label = entityLabel(ix, e, lang);
  if (isClub(ix, e)) {
    const c = ix.clubs[e];
    return {
      kind: 'club',
      label,
      pill: `${t.club} · ${t.country(ix.leagues[c.league].country)}`,
      code: shortCode(c),
      colour1: c.colour1,
      colour2: c.colour2,
      alt: t.badgeOf(label),
    };
  }
  return {
    kind: 'nation',
    label,
    pill: t.nation,
    flag: ix.nations[e - ix.clubCount].flag,
    alt: t.flagOf(label),
  };
}

export function answerView(
  ix: GameIndex,
  left: number,
  right: number,
  t: GameStrings,
  lang: GameLang,
): AnswerView {
  const list = matches(ix, left, right);
  return {
    shown: list.slice(0, SHOWN_ANSWERS),
    more: Math.max(0, list.length - SHOWN_ANSWERS),
    none: list.length ? null : t.none(entityLabel(ix, left, lang), entityLabel(ix, right, lang)),
  };
}
