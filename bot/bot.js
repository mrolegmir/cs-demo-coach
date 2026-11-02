#!/usr/bin/env node
// csdc-bot turns share codes into demo URLs through the CS2 Game
// Coordinator (ADR 12). It signs in to a dedicated Steam account, "plays" CS2
// so the Game Coordinator talks to it, and asks for one match at a time.
//
//   node bot.js login   sign in once: asks for the account name, password and
//                       the Steam Guard code from email, and saves a refresh
//                       token (no password is stored)
//   node bot.js run     resolve waiting share codes until stopped
//
// Environment for run: CSDC_API_URL, CSDC_WORKER_TOKEN. CSDC_BOT_SESSION is
// where the refresh token lives (default .session.json next to this file).

import { readFile, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { createInterface } from "node:readline";
import { once } from "node:events";
import { fileURLToPath } from "node:url";
import { setTimeout as sleep } from "node:timers/promises";

import SteamUser from "steam-user";
import GlobalOffensive from "globaloffensive";

import { API } from "./api.js";
import { resolve } from "./resolve.js";

const CS2 = 730;
const sessionFile = process.env.CSDC_BOT_SESSION || join(dirname(fileURLToPath(import.meta.url)), ".session.json");

// Pacing: the Game Coordinator throttles accounts that ask too fast.
const between = 5_000;
const idle = 60_000;
const answerTimeout = 30_000;
const drain = 60_000;

// Errors that mean the saved session is no good and login must run again.
const authErrors = new Set([
  SteamUser.EResult.InvalidPassword,
  SteamUser.EResult.AccessDenied,
  SteamUser.EResult.Revoked,
  SteamUser.EResult.Expired,
  SteamUser.EResult.AccountLoginDeniedNeedTwoFactor,
]);

function log(msg, fields = {}) {
  const extra = Object.entries(fields).map(([k, v]) => ` ${k}=${JSON.stringify(v)}`).join("");
  console.log(`time=${new Date().toISOString()} msg=${JSON.stringify(msg)}${extra}`);
}

async function saveSession(accountName, refreshToken) {
  await writeFile(sessionFile, JSON.stringify({ accountName, refreshToken }, null, 2) + "\n", { mode: 0o600 });
}

// ask reads a line from the terminal; hidden input is not echoed.
function ask(question, hidden = false) {
  return new Promise((resolve) => {
    const rl = createInterface({ input: process.stdin, output: process.stdout, terminal: true });
    if (hidden) {
      rl._writeToOutput = (s) => {
        if (s.startsWith(question)) rl.output.write(question);
      };
    }
    rl.question(question, (answer) => {
      rl.close();
      if (hidden) process.stdout.write("\n");
      resolve(answer.trim());
    });
  });
}

async function login() {
  const accountName = await ask("Steam account name: ");
  const password = await ask("Password (not shown, not saved): ", true);
  const client = new SteamUser();
  client.on("steamGuard", async (domain, callback, lastCodeWrong) => {
    if (lastCodeWrong) console.log("That code was wrong.");
    const where = domain ? `the email sent to ...@${domain}` : "the Steam Guard app";
    callback(await ask(`Steam Guard code from ${where}: `));
  });
  const token = new Promise((done) => client.once("refreshToken", done));
  client.logOn({ accountName, password });
  try {
    await Promise.race([
      once(client, "loggedOn"),
      once(client, "error").then(([err]) => Promise.reject(err)),
    ]);
  } catch (err) {
    console.error(`Sign-in failed: ${err.message}`);
    process.exit(1);
  }
  await saveSession(accountName, await token);
  console.log(`Signed in as ${accountName}. Session saved to ${sessionFile}; run "node bot.js run" now.`);
  client.logOff();
}

async function run() {
  const base = process.env.CSDC_API_URL;
  const token = process.env.CSDC_WORKER_TOKEN;
  if (!base || !token) {
    console.error("CSDC_API_URL and CSDC_WORKER_TOKEN are required");
    process.exit(1);
  }
  let session;
  try {
    session = JSON.parse(await readFile(sessionFile, "utf8"));
  } catch {
    console.error(`No session in ${sessionFile}: run "node bot.js login" first.`);
    process.exit(2);
  }
  const api = new API(base, token);
  const client = new SteamUser({ renewRefreshTokens: true });
  const csgo = new GlobalOffensive(client);

  client.on("refreshToken", (t) => {
    saveSession(session.accountName, t).then(
      () => log("refresh token renewed"),
      (err) => log("could not save the renewed refresh token", { err: err.message }),
    );
  });
  client.on("error", (err) => {
    if (authErrors.has(err.eresult)) {
      log("the saved session was rejected; run login again", { eresult: err.eresult });
      process.exit(2);
    }
    log("steam error", { err: err.message, eresult: err.eresult });
    process.exit(1);
  });
  client.on("loggedOn", () => {
    log("signed in", { account: session.accountName });
    client.gamesPlayed([CS2]);
  });
  client.on("disconnected", (eresult, msg) => log("disconnected from steam", { eresult, msg }));
  csgo.on("connectedToGC", () => log("connected to the game coordinator"));
  csgo.on("disconnectedFromGC", (reason) => log("lost the game coordinator", { reason }));

  client.logOn({ refreshToken: session.refreshToken });

  for (;;) {
    if (!csgo.haveGCSession) {
      await sleep(5_000);
      continue;
    }
    let job;
    try {
      job = await api.claim();
    } catch (err) {
      log("claim failed", { err: err.message });
      await sleep(idle);
      continue;
    }
    if (!job) {
      await sleep(idle);
      continue;
    }
    const result = await resolve(api, csgo, job, { timeout: answerTimeout, log });
    // After a timeout, let a late answer arrive before the next request.
    await sleep(result === "no answer" ? drain : between);
  }
}

const cmd = process.argv[2];
if (cmd === "login") await login();
else if (cmd === "run") await run();
else {
  console.error("usage: node bot.js login | run");
  process.exit(1);
}
