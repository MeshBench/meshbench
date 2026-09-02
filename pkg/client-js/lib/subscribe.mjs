// Being told, rather than asking.
//
// The socket is request/reply, and stays that way: a script sends a verb and
// reads its answer. A subscription is the other shape - the workbench writing a
// line when something changes, unbidden - and it does not fit a call, so it is
// given a connection of its own to stream on. A client that never subscribes
// sees exactly the request/reply protocol it always did.
//
// Each notification is {"event": ..., "data": ...} with no id. The absent id is
// the whole distinction: a reply carries the id it answered, a notification
// never does, so the two can never be confused for one another on the wire.

import { Connection } from "./socket.mjs";

/** The verb that opens a subscription on a connection. */
export const SUBSCRIBE = "session.subscribe";

/** A live stream of notifications on a connection of its own.
 *
 *  Iterate it with `for await`: it waits until the next notification arrives and
 *  ends when the workbench hangs up. Close it, so the extra connection does not
 *  outlive the interest.
 *
 *  Each notification is `{topic, data, dropped}`, where `dropped` is how many
 *  snapshot notifications the server coalesced away before this one - zero for
 *  every other topic. */
export class Subscription {
  constructor(conn) {
    this._conn = conn;
    this._queue = [];
    this._waiting = [];
    this._done = false;
    conn.onNotification = (msg) => this._push(msg);
  }

  /** Open one to the given topics - "status", "snapshot", and whatever else the
   *  workbench publishes. */
  static async open(address, topics = []) {
    // No call timeout: a stream waits as long as it must between events, where
    // a call would rather fail than hang.
    const conn = await Connection.open({ address, callTimeoutMs: 0 });
    const sub = new Subscription(conn);
    try {
      await conn.call(SUBSCRIBE, { topics }, 30_000);
    } catch (e) {
      conn.close();
      throw e;
    }
    return sub;
  }

  _push(msg) {
    const note = msg === null ? null : {
      topic: msg.event || "", data: msg.data, dropped: msg.dropped || 0,
    };
    if (note === null) this._done = true;
    const waiter = this._waiting.shift();
    if (waiter) {
      waiter(note);
      return;
    }
    this._queue.push(note);
  }

  /** The next notification, or null once the workbench has hung up. */
  next() {
    if (this._queue.length) return Promise.resolve(this._queue.shift());
    if (this._done) return Promise.resolve(null);
    return new Promise((resolve) => this._waiting.push(resolve));
  }

  async *[Symbol.asyncIterator]() {
    for (;;) {
      const note = await this.next();
      if (note === null) return;
      yield note;
    }
  }

  /** Hang up the stream. An iterator waiting on the next notification is
   *  released rather than left waiting for one that is not coming. */
  close() {
    this._conn.close();
    this._push(null);
  }
}
