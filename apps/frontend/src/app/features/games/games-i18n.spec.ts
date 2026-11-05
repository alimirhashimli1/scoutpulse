import dataset from '../../../../../football-svc/internal/games/playedforboth/data/dataset.json';

import { PlayedForBothWire } from '../../core/models/games';
import { GAME_LANGS, GAME_STRINGS, NATION_DE, clubName, nationName } from './games-i18n';

const wire = dataset as unknown as PlayedForBothWire;
const nations = wire.nations.map(([, nameEN, nameTR, flag]) => ({ nameEN, nameTR, flag }));

describe('games i18n', () => {
  it('has a German name for every nation in the dataset', () => {
    // A fallback to English would be silent; a regenerated dataset with a new
    // nation should fail here instead.
    expect(nations.filter((n) => !(n.flag in NATION_DE)).map((n) => n.nameEN)).toEqual([]);
  });

  it('names nations and clubs per language', () => {
    const germany = nations.find((n) => n.flag === 'de')!;
    expect(nationName(germany, 'en')).toBe('Germany');
    expect(nationName(germany, 'tr')).toBe('Almanya');
    expect(nationName(germany, 'de')).toBe('Deutschland');

    const bayern = { id: 'bay', name: 'Bayern Munich' };
    expect(clubName(bayern, 'en')).toBe('Bayern Munich');
    expect(clubName(bayern, 'tr')).toBe('Bayern Münih');
    expect(clubName(bayern, 'de')).toBe('Bayern München');
    expect(clubName({ id: 'ars', name: 'Arsenal' }, 'de')).toBe('Arsenal');
  });

  it('translates every league country', () => {
    for (const [, country] of wire.leagues) {
      expect(GAME_STRINGS.de.country(country), country).toBeTruthy();
      if (!['England', 'Portugal', 'USA', 'Ukraine'].includes(country)) {
        expect(GAME_STRINGS.de.country(country), country).not.toBe(country);
      }
    }
  });

  it('defines every string in every language', () => {
    const keys = Object.keys(GAME_STRINGS.en).sort();
    for (const lang of GAME_LANGS) expect(Object.keys(GAME_STRINGS[lang]).sort()).toEqual(keys);
  });
});
