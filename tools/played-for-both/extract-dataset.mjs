#!/usr/bin/env node
// One-off: pulls the game dataset out of the standalone reference page and
// writes it where football-svc embeds it.
//
//   node tools/played-for-both/extract-dataset.mjs [input.html] [output.json]
//
// The reference keeps `const DATA = {...};` on a single line. Everything after
// `const DATA = ` up to the `;` that ends that line is the JSON. The base64
// FLAGS constant on the line before is deliberately ignored: the app uses the
// flag-icons package instead.
//
// The script refuses to write anything that fails the sanity checks below, so
// a broken reference cannot silently ship a broken game.

import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, '../..');
const input = resolve(process.argv[2] ?? resolve(here, 'index.html'));
const output = resolve(
  process.argv[3] ??
    resolve(root, 'apps/football-svc/internal/games/playedforboth/data/dataset.json'),
);

const html = readFileSync(input, 'utf8');
const line = html.split(/\r?\n/).find((l) => l.trimStart().startsWith('const DATA = '));
if (!line) fail('no line starting with "const DATA = " in ' + input);

const start = line.indexOf('const DATA = ') + 'const DATA = '.length;
const end = line.lastIndexOf(';');
if (end <= start) fail('the DATA line does not end with ";"');

let data;
try {
  data = JSON.parse(line.slice(start, end));
} catch (e) {
  fail('DATA is not valid JSON: ' + e.message);
}

validate(data);

mkdirSync(dirname(output), { recursive: true });
// Compact, as in the reference: the file is served as-is.
writeFileSync(output, JSON.stringify(data) + '\n');
console.log(
  `wrote ${output}\n  ${data.leagues.length} leagues, ${data.clubs.length} clubs, ` +
    `${data.nations.length} nations, ${data.players.length} players`,
);

// ---------------------------------------------------------------------------

function validate(d) {
  const { leagues, clubs, nations, players } = d;
  check(Array.isArray(leagues) && leagues.length === 20, `20 leagues (got ${leagues?.length})`);
  check(Array.isArray(clubs) && clubs.length === 359, `359 clubs (got ${clubs?.length})`);
  check(Array.isArray(nations) && nations.length === 60, `60 nations (got ${nations?.length})`);
  check(
    Array.isArray(players) && players.length > 10_500 && players.length < 12_000,
    `about 11,200 players (got ${players?.length})`,
  );

  for (const l of leagues) check(l.length === 3 && l.every((x) => typeof x === 'string'), 'league shape ' + JSON.stringify(l));
  for (const c of clubs) {
    check(c.length === 5 && typeof c[0] === 'string' && typeof c[1] === 'string', 'club shape ' + JSON.stringify(c));
    check(Number.isInteger(c[4]) && c[4] >= 0 && c[4] < leagues.length, 'club league index ' + c[0]);
  }
  for (const n of nations) check(n.length === 4 && n.every((x) => typeof x === 'string'), 'nation shape ' + JSON.stringify(n));
  for (const p of players) {
    check(p.length === 4 && typeof p[0] === 'string', 'player shape ' + JSON.stringify(p));
    check(Number.isInteger(p[1]) && p[1] >= -1 && p[1] < nations.length, 'player nation index ' + p[0]);
    check(Array.isArray(p[2]) && p[2].every((i) => Number.isInteger(i) && i >= 0 && i < clubs.length), 'player clubs ' + p[0]);
    check(p[3] === 0 || p[3] === 1, 'player famous flag ' + p[0]);
  }

  const club = (name) => {
    const i = clubs.findIndex((c) => c[1] === name);
    check(i >= 0, 'club ' + name + ' exists');
    return i;
  };
  const nation = (name) => {
    const i = nations.findIndex((n) => n[1] === name);
    check(i >= 0, 'nation ' + name + ' exists');
    return i;
  };
  const both = (pred) => players.filter(pred).map((p) => p[0]);
  const clubs2 = (a, b) => both((p) => p[2].includes(a) && p[2].includes(b));

  const togoSpurs = both((p) => p[1] === nation('Togo') && p[2].includes(club('Tottenham Hotspur')));
  check(
    togoSpurs.length === 1 && togoSpurs[0] === 'Emmanuel Adebayor',
    'Togo + Tottenham Hotspur is exactly Adebayor (got ' + JSON.stringify(togoSpurs) + ')',
  );

  const rmPsg = clubs2(club('Real Madrid'), club('Paris Saint-Germain'));
  for (const n of ['Kylian Mbappé', 'Sergio Ramos', 'David Beckham'])
    check(rmPsg.includes(n), 'Real Madrid + PSG includes ' + n);

  const galInter = clubs2(club('Galatasaray'), club('Inter Milan'));
  for (const n of ['Wesley Sneijder', 'Hakan Şükür'])
    check(galInter.includes(n), 'Galatasaray + Inter Milan includes ' + n);
}

function check(ok, what) {
  if (!ok) fail('sanity check failed: ' + what);
}

function fail(msg) {
  console.error('extract-dataset: ' + msg);
  process.exit(1);
}
