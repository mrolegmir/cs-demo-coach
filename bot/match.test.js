import { test } from "node:test";
import assert from "node:assert/strict";

import { answers, demoFromMatches, matchIdOf } from "./match.js";

const url = "http://replay183.valve.net/730/003748329417234423978_1234567890.dem.bz2";
const id = "3230642215713767580";

test("decodes the match ID like internal/sharecode", () => {
  // Vector from the csgo-sharecode library's README.
  assert.equal(matchIdOf("CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK"), id);
  assert.equal(matchIdOf("CSGOGADqfjjyJ8cSP2rsmZRoTO2xK"), id);
  assert.throws(() => matchIdOf("CSGO-nope"));
  assert.throws(() => matchIdOf("CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2x0"));
});

test("takes the URL from the last round", () => {
  const got = demoFromMatches(
    [{ matchid: id, matchtime: 1761681600, roundstatsall: [{ map: "" }, { map: "de_mirage" }, { map: url }] }],
    id,
  );
  assert.deepEqual(got, { url, played: 1761681600 });
});

test("falls back to the legacy round stats", () => {
  const got = demoFromMatches([{ matchid: id, roundstatsall: [], roundstats_legacy: { map: url } }], id);
  assert.equal(got.url, url);
});

test("only the requested match counts", () => {
  const other = [{ matchid: "42", roundstatsall: [{ map: url }] }];
  assert.equal(demoFromMatches(other, id), null);
  assert.equal(answers(other, id), false);
  assert.equal(answers([{ matchid: id }], id), true);
});

test("no demo when the GC has none", () => {
  assert.equal(demoFromMatches([], id), null);
  assert.equal(demoFromMatches(undefined, id), null);
  assert.equal(demoFromMatches([{ matchid: id, roundstatsall: [{ map: "de_mirage" }] }], id), null);
});

test("ignores URLs outside Valve's replay servers", () => {
  for (const bad of ["http://evil.example/x.dem.bz2", "http://replay1.valve.net:8080/x.dem.bz2"]) {
    assert.equal(demoFromMatches([{ matchid: id, roundstatsall: [{ map: bad }] }], id), null);
  }
});
