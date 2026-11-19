// Tests for cover-retry.ts, run by `npm test` with Node's test runner (type
// stripping, Node >= 22.18). Nothing in the app imports this file, so it is
// not in the bundle.
/// <reference types="node" />
import assert from "node:assert/strict";
import { afterEach, beforeEach, mock, test } from "node:test";
import {
  __resetCoverRetryForTests,
  bumpCover,
  coverRevision,
  onCoverRevision,
  queueCoverBump,
} from "./cover-retry.ts";

const SERIES = "/api/v1/series/s1/thumbnail";

function advance(ms: number): void {
  mock.timers.tick(ms);
}

beforeEach(() => {
  mock.timers.enable({ apis: ["setTimeout", "Date"] });
  __resetCoverRetryForTests();
});

afterEach(() => {
  __resetCoverRetryForTests();
  mock.timers.reset();
});

test("a burst of queued bumps for one series bumps the series cover once, after about 1 s", () => {
  for (let i = 0; i < 20; i++) queueCoverBump(`b${i}`, "s1");
  advance(999);
  assert.equal(coverRevision(SERIES), 0);
  advance(1);
  assert.equal(coverRevision(SERIES), 1);
  advance(30_000);
  assert.equal(coverRevision(SERIES), 1);
});

test("queued bumps within 10 s of a bump give exactly one trailing bump, 10 s after it", () => {
  queueCoverBump("b1", "s1");
  advance(1000); // first bump at t=1000
  assert.equal(coverRevision(SERIES), 1);
  for (let i = 0; i < 5; i++) {
    advance(1000);
    queueCoverBump("b1", "s1");
  }
  advance(4000); // t=10000
  assert.equal(coverRevision(SERIES), 1);
  advance(999); // t=10999
  assert.equal(coverRevision(SERIES), 1);
  advance(1); // t=11000: 10 s after the bump at t=1000
  assert.equal(coverRevision(SERIES), 2);
  advance(30_000);
  assert.equal(coverRevision(SERIES), 2);
});

test("a queued bump right after bumpCover is dropped; a later one waits for the 10 s gap", () => {
  bumpCover("b1", "s1"); // t=0, immediate
  assert.equal(coverRevision(SERIES), 1);

  advance(1000);
  queueCoverBump("b1", "s1"); // flushes at t=2000, within 3 s of the immediate bump
  advance(1000);
  assert.equal(coverRevision(SERIES), 1);

  advance(1000);
  queueCoverBump("b1", "s1"); // flushes at t=4000: past 3 s, inside the 10 s gap
  advance(1000);
  assert.equal(coverRevision(SERIES), 1);

  advance(5999); // t=9999
  assert.equal(coverRevision(SERIES), 1);
  advance(1); // t=10000: 10 s after the immediate bump
  assert.equal(coverRevision(SERIES), 2);
});

test("each watcher is called once per bump, and the revision counts up", () => {
  let calls = 0;
  const off = onCoverRevision(SERIES, () => {
    calls++;
  });

  bumpCover("b1", "s1");
  assert.equal(calls, 1);
  assert.equal(coverRevision(SERIES), 1);

  advance(10_000);
  queueCoverBump("b2", "s1");
  advance(1000); // flush at t=11000: the gap since the bump at t=0 has passed
  assert.equal(calls, 2);
  assert.equal(coverRevision(SERIES), 2);

  off();
  bumpCover("b1", "s1");
  assert.equal(calls, 2);
  assert.equal(coverRevision(SERIES), 3);
});
