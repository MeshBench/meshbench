// The study area: which nodes are in the question being asked.
//
// Not the firmware's region concept. A boundary decides what is studied; a
// region decides what is forwarded. Both words are in this application, and
// confusing them is how somebody concludes the RF model is broken.
//
// Set it before importing. The import filters at fetch time, so a boundary set
// afterwards prunes what has already been paid for rather than never fetching
// it.

import fs from "node:fs";

import { MeshbenchError, NotFound } from "./errors.mjs";

/** The study area, however you have it. Live. */
export class Boundary {
  constructor(wb) { this._wb = wb; }

  /** Take a study area from a place name or from GeoJSON.
   *
   *  The one to call. A path to a .geojson file is loaded; anything else is
   *  searched for by name and the best match accepted. Both end with the area in
   *  the study, which is the only thing the caller wanted to say.
   *
   *  `name` renames a single loaded polygon, so a file called `export(3).geojson`
   *  can still join the study as "Tay catchment". */
  async use(area, { name = "" } = {}) {
    if (isGeoJSONPath(area)) return this.load(area, { name });
    const found = await this.search(String(area));
    return [await this.accept(found[0])];
  }

  /** Places matching a name, best first. Needs the network.
   *
   *  Names rather than geometry: the geometry stays at the workbench, and the
   *  name is what `accept` takes. */
  async search(query) {
    const got = (await this._wb.call("boundary.set", { query })) || {};
    const found = got.names || [];
    if (found.length === 0) {
      throw new NotFound("boundary.set", `nothing is called "${query}"`, "not_found");
    }
    return found;
  }

  /** Take one of the search results into the study area.
   *
   *  Areas union rather than replace: a study is often two council areas rather
   *  than one. */
  async accept(name) {
    const got = (await this._wb.call("boundary.accept", { name })) || {};
    return got.accepted || name;
  }

  /** Take a study area from GeoJSON: a path, the document itself, or an object.
   *
   *  A Polygon, a MultiPolygon, a Feature or a FeatureCollection. Each polygon
   *  becomes an area named from its "name" property, or from `name`, or from the
   *  file.
   *
   *  The one way to study an area nothing has an administrative name for - a
   *  catchment, a valley, the bit north of the river - and the only one that
   *  works with no network at all. */
  async load(source, { name = "" } = {}) {
    const params = {};
    if (source && typeof source === "object") {
      params.geojson = JSON.stringify(source);
    } else if (isGeoJSONPath(source)) {
      params.path = String(source);
    } else if (isJSONDocument(source)) {
      params.geojson = String(source);
    } else {
      // Said here rather than at the workbench, which would report it as a parse
      // failure on a document that is really a mistyped path.
      throw new MeshbenchError(
        `"${source}" is neither a .geojson path that exists nor a GeoJSON document`);
    }
    if (name) params.name = name;
    return ((await this._wb.call("boundary.load", params)) || {}).loaded || [];
  }

  /** What the study area is made of - the names, not a count of them. */
  async list() {
    return ((await this._wb.call("boundary.list")) || {}).names || [];
  }

  /** Take one area back out.
   *
   *  Changes what is measured, never what is loaded: the nodes stay until
   *  something prunes them. */
  async remove(name) { await this._wb.call("boundary.remove", { name }); }

  /** Delete the nodes outside the study area, and say how many went.
   *
   *  For a mesh that was imported before the boundary was set. The margin is
   *  kept on purpose and zero means the session's own: a node just outside still
   *  interferes with one just inside, and dropping it makes the inside look
   *  quieter than it is. */
  async prune({ marginKm = 0 } = {}) {
    const params = marginKm > 0 ? { margin_km: marginKm } : {};
    return ((await this._wb.call("boundary.prune", params)) || {}).removed || 0;
  }
}

function isJSONDocument(s) {
  return typeof s === "string" && s.trimStart().startsWith("{");
}

/** A path, rather than a place name or a GeoJSON document.
 *
 *  Judged by extension as well as by existence, so a mistyped path is reported
 *  as a missing file rather than searched for as a place - which answers
 *  "nothing is called ./bounds/fife.geojson" and sends the reader looking in
 *  entirely the wrong direction. */
function isGeoJSONPath(s) {
  if (typeof s !== "string" || isJSONDocument(s)) return false;
  if (s.endsWith(".geojson") || s.endsWith(".json")) return true;
  try {
    return fs.statSync(s).isFile();
  } catch {
    return false;
  }
}
