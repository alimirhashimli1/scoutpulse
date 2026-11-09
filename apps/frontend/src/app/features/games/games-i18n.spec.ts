import { GAME_LANGS, GAME_STRINGS, clubName, nationName } from './games-i18n';

describe('games i18n', () => {
  it('is English only', () => {
    expect([...GAME_LANGS]).toEqual(['en']);
    expect(GAME_STRINGS.en.round(1)).toBe('Round 1');
  });

  it('names nations and clubs as the dataset has them', () => {
    expect(nationName({ nameEN: 'Germany' }, 'en')).toBe('Germany');
    expect(clubName({ id: 'bay', name: 'Bayern Munich' }, 'en')).toBe('Bayern Munich');
  });
});
