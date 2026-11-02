// Reading the Game Coordinator's answer to a requestGame call.
//
// The answer is a list of CDataGCCStrike15_v2_MatchInfo. The demo URL is in
// the `map` field of the last round's stats (an old protocol quirk: the field
// once held the map name), and matchtime is when the match was played, in
// Unix seconds.

const replayHost = /\.valve\.net$/;

// Same codec as internal/sharecode: 18 little-endian bytes (match ID,
// outcome ID, TV port) read as one big-endian number, in base 57, least
// significant digit first.
const alphabet = "ABCDEFGHJKLMNOPQRSTUVWXYZabcdefhijkmnopqrstuvwxyz23456789";

/**
 * Returns the match ID a share code points to, as a decimal string.
 *
 * @param {string} code e.g. CSGO-GADqf-jjyJ8-cSP2r-smZRo-TO2xK
 * @returns {string}
 */
export function matchIdOf(code) {
  let s = code.trim().replaceAll("-", "");
  if (s.length === 29 && s.startsWith("CSGO")) s = s.slice(4);
  if (s.length !== 25) throw new Error(`not a share code: ${code}`);
  let n = 0n;
  for (const ch of [...s].reverse()) {
    const d = alphabet.indexOf(ch);
    if (d < 0) throw new Error(`not a share code: ${code}`);
    n = n * 57n + BigInt(d);
  }
  const hex = n.toString(16).padStart(36, "0");
  // The first 8 bytes, little-endian.
  let id = 0n;
  for (let i = 7; i >= 0; i--) id = (id << 8n) | BigInt(parseInt(hex.slice(i * 2, i * 2 + 2), 16));
  return id.toString();
}

/**
 * Returns the demo of match matchId in a matchList answer, or null when the
 * answer does not have it (another request's answer, or the match is too
 * old or unknown).
 *
 * @param {Array<object>} matches
 * @param {string} matchId decimal, from matchIdOf
 * @returns {{url: string, played: number} | null}
 */
export function demoFromMatches(matches, matchId) {
  for (const m of matches ?? []) {
    if (String(m.matchid) !== matchId) continue;
    const rounds = [...(m.roundstatsall ?? [])];
    if (m.roundstats_legacy) rounds.push(m.roundstats_legacy);
    for (const r of rounds.reverse()) {
      const url = replayURL(r?.map);
      if (url) return { url, played: Number(m.matchtime ?? 0) };
    }
  }
  return null;
}

/**
 * Reports whether a matchList answer is about matchId at all.
 *
 * @param {Array<object>} matches
 * @param {string} matchId
 */
export function answers(matches, matchId) {
  return (matches ?? []).some((m) => String(m.matchid) === matchId);
}

function replayURL(s) {
  if (typeof s !== "string") return null;
  let u;
  try {
    u = new URL(s);
  } catch {
    return null;
  }
  if ((u.protocol !== "http:" && u.protocol !== "https:") || !replayHost.test(u.hostname) || u.port) return null;
  return u.href;
}
