// Every example, imported.
//
// `go build ./...` compiles the Go examples, so a broken one is a red build
// rather than something somebody finds by trying it. Node has no compiler, so
// this is the peer: importing a module parses it, links it, and evaluates it,
// which catches a syntax error, a helper that has been renamed, and an export
// this client no longer has.
//
// It is stronger than a syntax check and only possible because each example
// exports `main` and runs itself only when it is the file node was given. An
// example that did its work at import time would start a workbench here.

import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const dir = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "examples");
const files = fs.readdirSync(dir).filter((f) => f.endsWith(".mjs")).sort();

test("there are examples to check", () => {
  assert.ok(files.length >= 7, `only ${files.length} examples found in ${dir}`);
});

for (const file of files) {
  test(`examples/${file} imports and exports main`, async () => {
    const mod = await import(pathToFileURL(path.join(dir, file)).href);
    assert.equal(typeof mod.main, "function",
      "an example exports main so this can import it without running it");
  });
}
