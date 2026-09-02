// Which workbench this client may drive.
//
// A client and the workbench it drives must be the same release. The protocol
// number beside this rule says whether two ends can understand each other's
// frames; it moves rarely and on purpose, so it cannot answer the question a
// script actually has, which is whether the package in this node_modules is the
// one that came with the workbench on the PATH. Two releases apart with no
// protocol bump between them connect happily and then disagree about a verb's
// parameters, forty calls in, looking like the simulation misbehaving.

import fs from "node:fs";

/** The wire version this client speaks. A workbench answering anything else is
 *  refused rather than failing halfway through a script. */
export const PROTOCOL = 1;

/** The release this client belongs to, as npm spells it.
 *
 *  Read from its own package.json rather than kept as a second literal here:
 *  the release workflow runs `npm version`, which rewrites that file and
 *  nothing else, and a copy in this module would be a copy somebody has to
 *  remember. Empty if it cannot be read, which is the same thing a build from a
 *  working copy would say and is treated the same way. */
export const RELEASE = readRelease();

function readRelease() {
  try {
    const p = new URL("../package.json", import.meta.url);
    return JSON.parse(fs.readFileSync(p, "utf8")).version || "";
  } catch {
    return "";
  }
}

/** Whether these two releases may be used together: an exact match, or one of
 *  the two ends not being a release at all.
 *
 *  The second half of the rule is what keeps the tree usable by the people
 *  working on it: a workbench built from a working copy has no release stamped
 *  in it, so insisting on equality would refuse every pair a developer has, for
 *  a disagreement that does not exist. Nothing is lost, because what the rule
 *  catches is a released client meeting a released workbench of another number,
 *  and both ends of that pair carry their stamp. */
export function pairedRelease(ours, theirs) {
  return !ours || !theirs || ours === theirs;
}

/** What to say about a check that did not compare anything, so a pair nothing
 *  verified is visible rather than quietly assumed sound. Returned rather than
 *  logged: a client is a library, and a script that wants the line can print
 *  it. */
export function pairingNote(ours, theirs) {
  if (!ours && !theirs) {
    return "release check skipped: neither this client nor the workbench is a release build";
  }
  if (!ours) {
    return "release check skipped: this client is a development build; " +
      `the workbench is ${theirs}`;
  }
  if (!theirs) {
    return "release check skipped: the workbench is a development build; " +
      `this client is ${ours}`;
  }
  return "";
}
