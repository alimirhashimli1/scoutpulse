import { DOCUMENT, isPlatformBrowser } from '@angular/common';
import { Injectable, PLATFORM_ID, computed, inject, signal } from '@angular/core';

/**
 * English, Turkish and German for the games section.
 *
 * The rest of ScoutPulse is English only and has no i18n system, so this is
 * deliberately small and local: one dictionary per language, a signal for the
 * current one, and the EN/TR/DE toggle the games show in their own header. It
 * is not a platform-wide mechanism and should not grow into one; if the site
 * adopts real i18n, these strings move there and the toggle goes.
 *
 * Adding a language: a dictionary below, an entry in GAME_LANGS, and — since
 * the dataset itself only carries English and Turkish nation names — a
 * NATION_xx map keyed by flag code, used by `nationName`.
 */

export const GAME_LANGS = ['en', 'tr', 'de'] as const;
export type GameLang = (typeof GAME_LANGS)[number];

export const GAME_LANG_STORAGE_KEY = 'scoutpulse.games.lang';

const isGameLang = (value: unknown): value is GameLang =>
  (GAME_LANGS as readonly unknown[]).includes(value);

// prettier-ignore
const COUNTRY_TR: Readonly<Record<string, string>> = {
  England: 'İngiltere', Spain: 'İspanya', Italy: 'İtalya', Germany: 'Almanya', France: 'Fransa',
  Portugal: 'Portekiz', Netherlands: 'Hollanda', Turkey: 'Türkiye', Belgium: 'Belçika',
  Scotland: 'İskoçya', Brazil: 'Brezilya', Argentina: 'Arjantin', USA: 'ABD',
  'Saudi Arabia': 'Suudi Arabistan', Russia: 'Rusya', Mexico: 'Meksika', Greece: 'Yunanistan',
  Austria: 'Avusturya', Switzerland: 'İsviçre', Ukraine: 'Ukrayna',
};

// prettier-ignore
const COUNTRY_DE: Readonly<Record<string, string>> = {
  England: 'England', Spain: 'Spanien', Italy: 'Italien', Germany: 'Deutschland',
  France: 'Frankreich', Portugal: 'Portugal', Netherlands: 'Niederlande', Turkey: 'Türkei',
  Belgium: 'Belgien', Scotland: 'Schottland', Brazil: 'Brasilien', Argentina: 'Argentinien',
  USA: 'USA', 'Saudi Arabia': 'Saudi-Arabien', Russia: 'Russland', Mexico: 'Mexiko',
  Greece: 'Griechenland', Austria: 'Österreich', Switzerland: 'Schweiz', Ukraine: 'Ukraine',
};

/** Club names that differ in Turkish, by club id. */
const CLUB_TR: Readonly<Record<string, string>> = {
  bay: 'Bayern Münih',
  int: 'Inter',
  m60: '1860 Münih',
  bsc: 'Hertha BSC',
  rso: 'Real Sociedad',
};

/** Club names as German football usage has them, by club id. */
// prettier-ignore
const CLUB_DE: Readonly<Record<string, string>> = {
  bay: 'Bayern München', m60: '1860 München', koe: '1. FC Köln', int: 'Inter Mailand',
  mil: 'AC Mailand', rom: 'AS Rom', nap: 'SSC Neapel', fio: 'AC Florenz', tor: 'FC Turin',
  gen: 'CFC Genua', bar: 'FC Barcelona', sev: 'FC Sevilla', val: 'FC Valencia',
  ben: 'Benfica Lissabon', spo: 'Sporting Lissabon', sta: 'Standard Lüttich',
  aek: 'AEK Athen', spm: 'Spartak Moskau', csk: 'ZSKA Moskau', lmo: 'Lokomotive Moskau',
  dmo: 'Dynamo Moskau', tmo: 'Torpedo Moskau', zen: 'Zenit St. Petersburg',
  dyn: 'Dynamo Kiew', sha2: 'Schachtar Donezk', mtl2: 'Metalist Charkiw',
  mdo: 'Metalurh Donezk', fcz: 'FC Zürich',
};

/**
 * German nation names, by flag-icons code. Spelled out rather than taken from
 * Intl.DisplayNames: the home nations have no region code, and the server's
 * ICU data and a browser's can disagree, which would break hydration.
 */
// prettier-ignore
export const NATION_DE: Readonly<Record<string, string>> = {
  es: 'Spanien', fr: 'Frankreich', 'gb-eng': 'England', it: 'Italien', br: 'Brasilien',
  de: 'Deutschland', ar: 'Argentinien', nl: 'Niederlande', pt: 'Portugal', be: 'Belgien',
  dk: 'Dänemark', ng: 'Nigeria', sn: 'Senegal', hr: 'Kroatien', uy: 'Uruguay',
  ch: 'Schweiz', ci: 'Elfenbeinküste', tr: 'Türkei', us: 'USA', ie: 'Irland', rs: 'Serbien',
  'gb-sct': 'Schottland', ma: 'Marokko', jp: 'Japan', mx: 'Mexiko', co: 'Kolumbien',
  cz: 'Tschechien', se: 'Schweden', 'gb-wls': 'Wales', pl: 'Polen', at: 'Österreich',
  cm: 'Kamerun', gr: 'Griechenland', no: 'Norwegen', 'gb-nir': 'Nordirland', gh: 'Ghana',
  ru: 'Russland', dz: 'Algerien', au: 'Australien', hu: 'Ungarn', sk: 'Slowakei',
  ro: 'Rumänien', cl: 'Chile', ca: 'Kanada', ml: 'Mali', is: 'Island', al: 'Albanien',
  si: 'Slowenien', py: 'Paraguay', ua: 'Ukraine', kr: 'Südkorea', bg: 'Bulgarien',
  ec: 'Ecuador', tg: 'Togo', cr: 'Costa Rica', eg: 'Ägypten', za: 'Südafrika', ga: 'Gabun',
  ir: 'Iran', lr: 'Liberia', xk: 'Kosovo',
};

const CLUB_NAMES: Readonly<Record<GameLang, Readonly<Record<string, string>>>> = {
  en: {},
  tr: CLUB_TR,
  de: CLUB_DE,
};

/** A club's name in this language; the dataset's English name otherwise. */
export function clubName(club: { id: string; name: string }, lang: GameLang): string {
  return CLUB_NAMES[lang][club.id] ?? club.name;
}

/** A nation's name in this language, falling back to English. */
export function nationName(
  nation: { nameEN: string; nameTR: string; flag: string },
  lang: GameLang,
): string {
  if (lang === 'tr') return nation.nameTR;
  if (lang === 'de') return NATION_DE[nation.flag] ?? nation.nameEN;
  return nation.nameEN;
}

export interface NoteCounts {
  players: number;
  clubs: number;
  nations: number;
  leagues: number;
}

const en = {
  // --- hub
  gamesTitle: 'Games',
  gamesLede: 'Football trivia to play with a friend on one screen.',
  playersTag: (n: number) => `${n} players`,
  play: 'Play',
  soon: 'Coming soon',
  backToGames: 'All games',
  hubError: 'The games could not be loaded.',
  datasetError: 'The game data could not be loaded.',
  retry: 'Retry',
  loading: 'Loading the game…',
  language: 'Language',
  'games.playedForBoth.title': 'Who Played For Both?',
  'games.playedForBoth.description':
    'Name a footballer who played for both teams. First right answer takes the point.',

  // --- game (from the reference)
  title: 'Who played for both?',
  sub: 'Name a footballer who played for the club on the left and the club or country on the right. First correct answer wins the point.',
  reveal: 'Reveal answer',
  next: 'Next pair',
  start: 'Start game',
  idle: 'Press Start for the first pair',
  mystery: 'Mystery team',
  space: 'Space',
  fit: 'Players who fit',
  settings: 'Game settings',
  optKnown:
    'Only show pairs with at least one known answer. Turn off for fully random pairs, which can be impossible.',
  optNation: 'Allow countries (club vs country). Turn off for club vs club only.',
  optFamous:
    'Build pairs from well-known players only. Turn off for a much harder game with lesser-known names.',
  leaguesLbl: 'Leagues in play',
  allL: 'All leagues',
  top5: 'Top 5 only',
  noneL: 'Clear',
  timer: 'Round timer',
  off: 'Off',
  s20: '20 s',
  s30: '30 s',
  s60: '60 s',
  reset: 'Reset scores',
  scoreboard: 'Scoreboard',
  timeLeft: 'Time left',
  round: (n: number) => `Round ${n}`,
  player: (n: number) => `Player ${n}`,
  playerName: (n: number) => `Player ${n} name`,
  point: (name: string) => `+1 ${name}`,
  club: 'Club',
  nation: 'National team',
  flagOf: (n: string) => `Flag of ${n}`,
  badgeOf: (n: string) => `${n} badge`,
  country: (c: string) => c,
  more: (n: number) => `+${n} more`,
  needLeague: 'Pick at least one league to play.',
  none: (a: string, b: string) =>
    `No player in this game's list played for both ${a} and ${b}. If someone named a real one, give them the point.`,
  note: (c: NoteCounts) =>
    `Country flags are real. Clubs are shown as badges in club colours rather than official crests, which are trademarks. The game knows ${c.players.toLocaleString('en')} players across ${c.clubs} clubs in ${c.leagues} leagues and ${c.nations} national teams. Careers come from historic transfer records (1992–2022), big-five league squads (2010–2026) and hand-checked legends, and include loan spells. The other player may still know a correct name that isn't listed.`,
  sources:
    'Sources: Transfermarkt transfer records 1992–2022 (github.com/ewenme/transfers), Transfermarkt player profiles (github.com/salimt/football-datasets), FBref big-five league squads 2010–2026 (worldfootballR_data), plus hand-checked legends. Flags: flag-icons (MIT).',
};

export type GameStrings = typeof en;

const tr: GameStrings = {
  gamesTitle: 'Oyunlar',
  gamesLede: 'Tek ekranda bir arkadaşınla oynayabileceğin futbol bilgi yarışmaları.',
  playersTag: (n) => `${n} oyuncu`,
  play: 'Oyna',
  soon: 'Yakında',
  backToGames: 'Tüm oyunlar',
  hubError: 'Oyunlar yüklenemedi.',
  datasetError: 'Oyun verileri yüklenemedi.',
  retry: 'Tekrar dene',
  loading: 'Oyun yükleniyor…',
  language: 'Dil',
  'games.playedForBoth.title': 'İkisinde de kim oynadı?',
  'games.playedForBoth.description':
    'İki takımda da oynamış bir futbolcu söyleyin. İlk doğru cevabı veren puanı alır.',

  title: 'İkisinde de kim oynadı?',
  sub: 'Soldaki kulüpte ve sağdaki kulüpte ya da milli takımda oynamış bir futbolcu söyleyin. İlk doğru cevabı veren puanı alır.',
  reveal: 'Cevabı göster',
  next: 'Sonraki eşleşme',
  start: 'Oyunu başlat',
  idle: "İlk eşleşme için Başlat'a basın",
  mystery: 'Gizli takım',
  space: 'Boşluk',
  fit: 'Uyan oyuncular',
  settings: 'Oyun ayarları',
  optKnown:
    'Yalnızca en az bir bilinen cevabı olan eşleşmeleri göster. Kapatırsanız eşleşmeler tamamen rastgele olur ve bazılarının cevabı olmayabilir.',
  optNation:
    'Ülkelere izin ver (kulüp - milli takım). Kapatırsanız yalnızca kulüp - kulüp eşleşmeleri gelir.',
  optFamous:
    'Eşleşmeleri yalnızca tanınmış oyunculardan oluştur. Kapatırsanız daha az bilinen isimlerle çok daha zor bir oyun olur.',
  leaguesLbl: 'Oyundaki ligler',
  allL: 'Tüm ligler',
  top5: 'Sadece 5 büyük lig',
  noneL: 'Temizle',
  timer: 'Tur süresi',
  off: 'Kapalı',
  s20: '20 sn',
  s30: '30 sn',
  s60: '60 sn',
  reset: 'Skorları sıfırla',
  scoreboard: 'Skor tabelası',
  timeLeft: 'Kalan süre',
  round: (n) => `Tur ${n}`,
  player: (n) => `Oyuncu ${n}`,
  playerName: (n) => `Oyuncu ${n} adı`,
  point: (name) => `+1 ${name}`,
  club: 'Kulüp',
  nation: 'Milli takım',
  flagOf: (n) => `${n} bayrağı`,
  badgeOf: (n) => `${n} rozeti`,
  country: (c) => COUNTRY_TR[c] ?? c,
  more: (n) => `+${n} oyuncu daha`,
  needLeague: 'Oynamak için en az bir lig seçin.',
  none: (a, b) =>
    `Bu oyunun listesinde hem ${a} hem de ${b} formasını giymiş bir oyuncu yok. Gerçek bir isim söyleyen olursa puanı ona verin.`,
  note: (c) =>
    `Ülke bayrakları gerçektir. Kulüpler, tescilli marka olan resmi armalar yerine kulüp renklerindeki rozetlerle gösterilir. Oyun ${c.leagues} ligdeki ${c.clubs} kulüp ve ${c.nations} milli takımdan ${c.players.toLocaleString('tr')} oyuncuyu tanır. Kariyerler 1992–2022 transfer kayıtlarından, 2010–2026 beş büyük lig kadrolarından ve elle kontrol edilmiş efsanelerden gelir; kiralık dönemler de dahildir. Rakibiniz listede olmayan doğru bir isim de bilebilir.`,
  sources:
    'Kaynaklar: Transfermarkt transfer kayıtları 1992–2022 (github.com/ewenme/transfers), Transfermarkt oyuncu profilleri (github.com/salimt/football-datasets), FBref beş büyük lig kadroları 2010–2026 (worldfootballR_data) ve elle kontrol edilmiş efsaneler. Bayraklar: flag-icons (MIT).',
};

const de: GameStrings = {
  gamesTitle: 'Spiele',
  gamesLede: 'Fußball-Quiz für zwei – mit Freunden an einem Bildschirm.',
  playersTag: (n) => `${n} Spieler`,
  play: 'Spielen',
  soon: 'Demnächst',
  backToGames: 'Alle Spiele',
  hubError: 'Die Spiele konnten nicht geladen werden.',
  datasetError: 'Die Spieldaten konnten nicht geladen werden.',
  retry: 'Erneut versuchen',
  loading: 'Spiel wird geladen…',
  language: 'Sprache',
  'games.playedForBoth.title': 'Wer spielte für beide?',
  'games.playedForBoth.description':
    'Nenne einen Fußballer, der für beide Teams gespielt hat. Die erste richtige Antwort holt den Punkt.',

  title: 'Wer spielte für beide?',
  sub: 'Nenne einen Fußballer, der für den Verein links und für den Verein oder das Land rechts gespielt hat. Die erste richtige Antwort holt den Punkt.',
  reveal: 'Antwort zeigen',
  next: 'Nächstes Paar',
  start: 'Spiel starten',
  idle: 'Drücke „Spiel starten“ für das erste Paar',
  mystery: 'Unbekanntes Team',
  space: 'Leertaste',
  fit: 'Passende Spieler',
  settings: 'Spieleinstellungen',
  optKnown:
    'Nur Paare mit mindestens einer bekannten Antwort zeigen. Ausgeschaltet sind die Paare völlig zufällig und manchmal unlösbar.',
  optNation: 'Länder erlauben (Verein gegen Land). Ausgeschaltet nur Verein gegen Verein.',
  optFamous:
    'Paare nur aus bekannten Spielern bilden. Ausgeschaltet wird es mit weniger bekannten Namen deutlich schwerer.',
  leaguesLbl: 'Ligen im Spiel',
  allL: 'Alle Ligen',
  top5: 'Nur Top 5',
  noneL: 'Leeren',
  timer: 'Rundentimer',
  off: 'Aus',
  s20: '20 s',
  s30: '30 s',
  s60: '60 s',
  reset: 'Punkte zurücksetzen',
  scoreboard: 'Anzeigetafel',
  timeLeft: 'Verbleibende Zeit',
  round: (n) => `Runde ${n}`,
  player: (n) => `Spieler ${n}`,
  playerName: (n) => `Name von Spieler ${n}`,
  point: (name) => `+1 ${name}`,
  club: 'Verein',
  nation: 'Nationalmannschaft',
  flagOf: (n) => `Flagge von ${n}`,
  badgeOf: (n) => `Wappen von ${n}`,
  country: (c) => COUNTRY_DE[c] ?? c,
  more: (n) => `+${n} weitere`,
  needLeague: 'Wähle mindestens eine Liga aus.',
  none: (a, b) =>
    `Kein Spieler in der Liste dieses Spiels hat sowohl für ${a} als auch für ${b} gespielt. Wenn jemand einen echten genannt hat, gib ihm den Punkt.`,
  note: (c) =>
    `Die Länderflaggen sind echt. Vereine werden als Abzeichen in den Vereinsfarben gezeigt statt mit offiziellen Wappen, die Markenzeichen sind. Das Spiel kennt ${c.players.toLocaleString('de')} Spieler aus ${c.clubs} Vereinen in ${c.leagues} Ligen und ${c.nations} Nationalmannschaften. Die Karrieren stammen aus historischen Transferdaten (1992–2022), Kadern der fünf großen Ligen (2010–2026) und von Hand geprüften Legenden, Leihen inklusive. Dein Gegner kennt vielleicht trotzdem einen richtigen Namen, der nicht aufgeführt ist.`,
  sources:
    'Quellen: Transfermarkt-Transferdaten 1992–2022 (github.com/ewenme/transfers), Transfermarkt-Spielerprofile (github.com/salimt/football-datasets), FBref-Kader der fünf großen Ligen 2010–2026 (worldfootballR_data) sowie von Hand geprüfte Legenden. Flaggen: flag-icons (MIT).',
};

export const GAME_STRINGS: Readonly<Record<GameLang, GameStrings>> = { en, tr, de };

@Injectable({ providedIn: 'root' })
export class GamesI18n {
  private readonly document = inject(DOCUMENT);
  private readonly isBrowser = isPlatformBrowser(inject(PLATFORM_ID));

  private readonly _lang = signal<GameLang>(this.read());
  readonly lang = this._lang.asReadonly();
  readonly t = computed(() => GAME_STRINGS[this._lang()]);

  set(lang: GameLang): void {
    this._lang.set(lang);
    if (!this.isBrowser) return;
    try {
      this.document.defaultView?.localStorage.setItem(GAME_LANG_STORAGE_KEY, lang);
    } catch {
      // Storage blocked: the choice holds for this visit only.
    }
  }

  /** A catalog key from the server, or the key itself if this build lacks it. */
  catalog(key: string): string {
    const value = (this.t() as unknown as Record<string, unknown>)[key];
    return typeof value === 'string' ? value : key;
  }

  private read(): GameLang {
    if (!this.isBrowser) return 'en';
    try {
      const stored = this.document.defaultView?.localStorage.getItem(GAME_LANG_STORAGE_KEY);
      if (isGameLang(stored)) return stored;
    } catch {
      // fall through to the browser language
    }
    const browser = (this.document.defaultView?.navigator.language ?? '').slice(0, 2);
    return isGameLang(browser) ? browser : 'en';
  }
}
