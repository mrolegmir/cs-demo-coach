// One share code: ask the Game Coordinator, report the answer to the api.

import { answers, demoFromMatches, matchIdOf } from "./match.js";

/**
 * Resolves job.code through gc and reports the result. Answers about other
 * matches are ignored, so a late answer to an earlier request is never taken
 * for this one. An empty answer means the Game Coordinator has no such match
 * any more (it carries no match ID, so the caller pauses after a timeout to
 * let late answers drain). No answer at all is a transient failure.
 *
 * @param {{resolve: Function, fail: Function}} api
 * @param {import("node:events").EventEmitter & {requestGame: Function}} gc
 * @param {{id: number, code: string}} job
 * @param {{timeout?: number, log?: Function}} [opts]
 * @returns {Promise<"resolved" | "expired" | "no answer" | "bad code">}
 */
export async function resolve(api, gc, job, { timeout = 30_000, log = () => {} } = {}) {
  let matchId;
  try {
    matchId = matchIdOf(job.code);
  } catch (err) {
    await report(log, () => api.fail(job.id, "Not a valid share code.", { permanent: true }));
    return "bad code";
  }
  const matches = await answer(gc, matchId, timeout, () => gc.requestGame(job.code));
  if (matches === null) {
    log("no answer from the game coordinator", { id: job.id });
    await report(log, () => api.fail(job.id, "The game coordinator did not answer."));
    return "no answer";
  }
  const demo = demoFromMatches(matches, matchId);
  if (!demo) {
    log("no demo for this match", { id: job.id });
    await report(log, () => api.fail(job.id, "", { expired: true }));
    return "expired";
  }
  log("demo found", { id: job.id, played: demo.played });
  await report(log, () => api.resolve(job.id, demo.url, demo.played));
  return "resolved";
}

// answer sends the request and waits for the matchList about matchId (or an
// empty one), or null after timeout.
function answer(gc, matchId, timeout, request) {
  return new Promise((done) => {
    const onList = (matches) => {
      if (matches?.length && !answers(matches, matchId)) return;
      finish(matches ?? []);
    };
    const timer = setTimeout(() => finish(null), timeout);
    function finish(v) {
      clearTimeout(timer);
      gc.off("matchList", onList);
      done(v);
    }
    gc.on("matchList", onList);
    request();
  });
}

// report sends a result to the api. If that fails, the claim runs out and the
// code is handed out again.
async function report(log, send) {
  try {
    await send();
  } catch (err) {
    log("report failed", { err: err.message });
  }
}
