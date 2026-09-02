// What went wrong, in a shape a script can catch.
//
// The workbench answers with a sentence and a code. The sentence is the good
// part - "no node is running firmware, so there is nothing to send to" - and it
// survives untouched; the code decides which class it becomes, because
// `instanceof` is what JavaScript has where Go has errors.Is and Python has an
// exception hierarchy. A caller that matched on prose would break the day the
// workbench reworded a refusal.

import { PROTOCOL, RELEASE } from "./pairing.mjs";

/** Anything this client throws. */
export class MeshbenchError extends Error {
  constructor(message) {
    super(message);
    this.name = "MeshbenchError";
  }
}

/** A verb the workbench declined, with its own words kept.
 *
 *  Named for the workbench rather than for the refusal because that is what a
 *  Node script has caught since the first release, and the name is the API. */
export class WorkbenchError extends MeshbenchError {
  /** Carries the code beside the message rather than folding it into the
   *  prose, because a caller that has to match on prose breaks the day the
   *  workbench rewords a refusal. */
  constructor(verb, message, code) {
    super(verb ? `${verb}: ${message}` : message);
    this.name = "WorkbenchError";
    /** Which verb was refused, so a log line says what was asked as well as
     *  what came back. Empty for a refusal that answered no verb - the
     *  connection itself being turned away. */
    this.verb = verb || "";
    /** The workbench's own words, unaltered and without the verb in front. */
    this.detail = message;
    /** How the refusal was classified, so a caller can branch on it instead of
     *  on prose: the workbench's own code (`not_found`, `conflict`, `closing`
     *  and the rest the control socket defines) when a verb was refused, and
     *  `protocol_mismatch` or `version_mismatch` when this client was the end
     *  that refused, at the handshake - the same two the workbench uses, so a
     *  script branching on the code need not care which end noticed. Empty when
     *  a refusal arrived without one, so test it rather than assume it is set. */
    this.code = code || "";
  }
}

/** Not a method this build has.
 *
 *  Nearly always a client older or newer than the workbench, which connecting
 *  is supposed to have caught first, so seeing this is worth looking into. */
export class UnknownVerb extends WorkbenchError {}

/** The verb refused what it was given. */
export class BadParams extends WorkbenchError {}

/** No node, build, area or job of that name. */
export class NotFound extends WorkbenchError {}

/** The right request in the wrong state: nothing loaded, nothing running to
 *  send to, no import preview to commit. */
export class Conflict extends WorkbenchError {}

/** A request this session cannot serve at all - a window verb with no window,
 *  or hardware that is not here. */
export class Unavailable extends WorkbenchError {}

/** The workbench is shutting down. Retry against a new session rather than
 *  report a bug. */
export class Closing extends WorkbenchError {}

const BY_CODE = {
  unknown_verb: UnknownVerb,
  bad_params: BadParams,
  not_found: NotFound,
  conflict: Conflict,
  unavailable: Unavailable,
  closing: Closing,
};

/** The right class for a code.
 *
 *  An unrecognised code becomes a plain WorkbenchError rather than an error
 *  about the error: a workbench newer than this client may classify something
 *  in a way this version has never heard of, and swallowing that would be worse
 *  than passing it on. */
export function refusal(verb, message, code) {
  const Cls = BY_CODE[code] || WorkbenchError;
  const e = new Cls(verb, message, code);
  e.name = Cls.name;
  return e;
}

/** A client and a workbench that cannot speak to each other's frames.
 *
 *  Its own class rather than a WorkbenchError carrying a code, because a
 *  script has to be able to tell "these two cannot talk" from "this build
 *  declined what I asked" with `instanceof`, and the two remedies have nothing
 *  in common. */
export class ProtocolMismatch extends WorkbenchError {
  constructor(message, { client = PROTOCOL, workbench = 0 } = {}) {
    super("", message, "protocol_mismatch");
    this.name = "ProtocolMismatch";
    /** The wire version each end speaks. `workbench` is 0 when the workbench
     *  refused the connection before it would say what it was. */
    this.client = client;
    this.workbench = workbench;
  }
}

/** A released client driving a workbench from a different release.
 *
 *  Separate from ProtocolMismatch: two ends can understand each other's frames
 *  perfectly and still be a pair nobody ever built or tested together. */
export class VersionMismatch extends WorkbenchError {
  constructor(message, { client = RELEASE, workbench = "" } = {}) {
    super("", message, "version_mismatch");
    this.name = "VersionMismatch";
    /** The release each end belongs to. `workbench` is empty when the
     *  workbench refused before it would say what it was. */
    this.client = client;
    this.workbench = workbench;
  }
}

/** A wait that ran out, saying what it wanted and what it last saw.
 *
 *  Not a bare deadline: "timeout" in a CI log tells whoever reads it nothing,
 *  and the state at the moment it gave up is the only thing that does. */
export class Timeout extends MeshbenchError {
  constructor(what, afterMs, last = "") {
    super(`waited ${Math.round(afterMs / 1000)}s for ${what}` +
      (last ? `; last saw: ${last}` : ""));
    this.name = "Timeout";
    this.what = what;
    this.afterMs = afterMs;
    this.last = last;
  }
}

/** The workbench's refusal of what this client declared, as the mismatch it is
 *  rather than as whichever call happened to be in flight failing - which is
 *  the confusion the declaration exists to end. Everything else is left alone. */
export function asMismatch(e) {
  if (e instanceof ProtocolMismatch || e instanceof VersionMismatch) return e;
  if (!(e instanceof WorkbenchError)) return e;
  if (e.code === "protocol_mismatch") return new ProtocolMismatch(e.detail);
  if (e.code === "version_mismatch") return new VersionMismatch(e.detail);
  return e;
}
