import { test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";

import { resolve } from "./resolve.js";

const url = "http://replay1.valve.net/730/1.dem.bz2";
const code = "CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK";
const id = "3230642215713767580";

// fakeGC answers requestGame with the given lists, in order, or not at all.
function fakeGC(...lists) {
  const gc = new EventEmitter();
  gc.requested = [];
  gc.requestGame = (c) => {
    gc.requested.push(c);
    for (const l of lists) setImmediate(() => gc.emit("matchList", l, {}));
  };
  return gc;
}

function fakeAPI({ broken = false } = {}) {
  const calls = [];
  const record = (...args) => {
    calls.push(args);
    return broken ? Promise.reject(new Error("api down")) : Promise.resolve();
  };
  return {
    calls,
    resolve: (...a) => record("resolve", ...a),
    fail: (...a) => record("fail", ...a),
  };
}

test("reports the demo URL and match time", async () => {
  const gc = fakeGC([{ matchid: id, matchtime: 1761681600, roundstatsall: [{ map: url }] }]);
  const api = fakeAPI();
  assert.equal(await resolve(api, gc, { id: 7, code }), "resolved");
  assert.deepEqual(gc.requested, [code]);
  assert.deepEqual(api.calls, [["resolve", 7, url, 1761681600]]);
});

test("a late answer about another match is ignored", async () => {
  const other = [{ matchid: "42", roundstatsall: [{ map: "http://replay9.valve.net/730/other.dem.bz2" }] }];
  const gc = fakeGC(other, [{ matchid: id, roundstatsall: [{ map: url }] }]);
  const api = fakeAPI();
  assert.equal(await resolve(api, gc, { id: 7, code }), "resolved");
  assert.equal(api.calls[0][2], url);
});

test("an empty answer marks the code expired", async () => {
  const api = fakeAPI();
  assert.equal(await resolve(api, fakeGC([]), { id: 7, code }), "expired");
  assert.deepEqual(api.calls, [["fail", 7, "", { expired: true }]]);
});

test("no answer is a transient failure", async () => {
  const api = fakeAPI();
  assert.equal(await resolve(api, fakeGC(), { id: 7, code }, { timeout: 20 }), "no answer");
  assert.equal(api.calls.length, 1);
  assert.equal(api.calls[0][0], "fail");
  assert.equal(api.calls[0][3], undefined); // neither expired nor permanent
});

test("a malformed code fails for good without asking the GC", async () => {
  const gc = fakeGC();
  const api = fakeAPI();
  assert.equal(await resolve(api, gc, { id: 7, code: "CSGO-nope" }), "bad code");
  assert.deepEqual(gc.requested, []);
  assert.equal(api.calls[0][3].permanent, true);
});

test("an api error does not throw", async () => {
  const gc = fakeGC([{ matchid: id, roundstatsall: [{ map: url }] }]);
  assert.equal(await resolve(fakeAPI({ broken: true }), gc, { id: 7, code }), "resolved");
});
