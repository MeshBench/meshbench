#!/usr/bin/env python3
"""Regenerate docs/scripting-verbs.md from the verbs the tree actually
registers.

    tools/verbdoc/verbdoc.py            write the document
    tools/verbdoc/verbdoc.py --check    fail if it is out of date

Three of the four columns are read out of `internal/app/session`: the verb
name, what its handler reads out of its parameters, and the keys of what it
returns. The fourth - which scripting-API call covers it - is authored, in
facade.json beside this file, because it is a design decision and not a fact
about the tree. Data rather than code so it reads as the manifest it is.

A verb with no entry in facade.json fails this script, which is the point: a
verb no client can reach is a verb nobody outside the workbench can use, and
that is exactly how a hand-written surface can come to call a verb that had
been deleted.

This is deliberately a prototype of the manifest in #213 rather than the
manifest itself. It reads the handler bodies with a brace matcher and regular
expressions, which is good enough to document a surface and not good enough to
generate a client - 77 verbs read parameters in ways nothing outside the
handler can see. When #213 lands, the descriptions come from the registration
and this script goes away.
"""
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
SESSION = os.path.join(ROOT, "internal", "app", "session")
DOC = os.path.join(ROOT, "docs", "scripting-verbs.md")
MAP = os.path.join(HERE, "facade.json")

# The authored half, as data rather than as code: which call covers each verb,
# why the ones with none have none, and how the table is grouped. Beside the
# script rather than inside it because it is a manifest somebody reviews, and
# because it is what #213 replaces.
_map = json.load(open(MAP))
FACADE = _map["facade"]
WHY_NONE = _map["no_facade"]
GROUPS = [(g["title"], g["prefixes"]) for g in _map["groups"]]

# Both spellings: HandleSpec is the one that carries a description, and verbs
# are being moved onto it a file at a time (#213). Until they all are, this has
# to see either, or a described verb silently drops out of the document.
HANDLE = re.compile(r'st\.Handle(?:Spec)?\(\s*"([a-z0-9_.]+)"\s*,')

PARAM_PATTERNS = [
    (re.compile(r'(?:session\.String|string)Field\(\s*p\s*,\s*"([a-z0-9_]+)"'), "string"),
    # namedField is stringField's sibling for a verb's non-primary parameters -
    # added once a verb learned that a bare value must fill only one field, not
    # every field asked of it (#235). Missing this pattern is not cosmetic: it
    # silently dropped every secondary parameter a verb converted, which is
    # exactly the kind of surface #213 exists to keep honest.
    (re.compile(r'(?:session\.Named|named)Field\(\s*p\s*,\s*"([a-z0-9_]+)"'), "string"),
    (re.compile(r'(?:session\.Num|num)Field\(\s*p\s*,\s*"([a-z0-9_]+)"'), "number"),
    (re.compile(r'boolField\(\s*p\s*,\s*"([a-z0-9_]+)"'), "bool"),
    (re.compile(r'm\["([a-z0-9_]+)"\]\.\(string\)'), "string"),
    (re.compile(r'm\["([a-z0-9_]+)"\]\.\(bool\)'), "bool"),
    (re.compile(r'm\["([a-z0-9_]+)"\]\.\(float64\)'), "number"),
    (re.compile(r'm\["([a-z0-9_]+)"\]\.\(\[\]any\)'), "list"),
    (re.compile(r'm\["([a-z0-9_]+)"\]'), "any"),
]
RESULT = re.compile(r"return\s+map\[string\]any\{(.*?)\}\s*,\s*nil", re.S)
RESULT_KEY = re.compile(r'"([a-z0-9_]+)"\s*:')


def body_of(src, start):
    """The handler body, by brace matching from the func literal.

    Started at the `func(` rather than at the next brace, because on a
    HandleSpec call the next brace opens the description and brace-matching
    from there reads the spec where the handler should be - which showed up as
    a described verb losing its parameters and its interface marker in this
    table, the opposite of what describing it was for.
    """
    f = src.find("func(", start)
    if f >= 0:
        start = f
    i = src.find("{", start)
    if i < 0:
        return ""
    depth, j = 0, i
    while j < len(src):
        c = src[j]
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return src[i : j + 1]
        elif c == '"':
            j += 1
            while j < len(src) and src[j] != '"':
                if src[j] == "\\":
                    j += 1
                j += 1
        elif c == "`":
            j = src.find("`", j + 1)
        j += 1
    return src[i:]


MANIFEST = os.path.join(os.path.dirname(__file__), "..", "..", "docs", "verbs.json")


def described():
    """What the manifest says, for the verbs that have learned to say it.

    Preferred over the regexes below wherever it exists. The regexes read a
    handler body and guess; the manifest is what whoever wrote the verb said it
    takes, which is the whole point of #213. As verbs move onto HandleSpec this
    table gets better a row at a time instead of worse.
    """
    try:
        with open(MANIFEST) as f:
            return json.load(f).get("verbs", {})
    except FileNotFoundError:
        return {}


def scan():
    """Every registered verb, with what it reads and what it returns."""
    verbs = {}
    for dirpath, _, names in os.walk(SESSION):
        for n in sorted(names):
            if not n.endswith(".go") or n.endswith("_test.go"):
                continue
            src = open(os.path.join(dirpath, n)).read()
            for m in HANDLE.finditer(src):
                body = body_of(src, m.end())
                params, seen = [], set()
                for pat, kind in PARAM_PATTERNS:
                    for pm in pat.finditer(body):
                        if pm.group(1) in seen:
                            continue
                        seen.add(pm.group(1))
                        params.append((pm.group(1), kind))
                results, rseen = [], set()
                for rm in RESULT.finditer(body):
                    for k in RESULT_KEY.findall(rm.group(1)):
                        if k not in rseen:
                            rseen.add(k)
                            results.append(k)
                verbs[m.group(1)] = {
                    "params": params,
                    "sole": bool(re.search(r"(?:session\.Sole|sole)String\(\s*p\s*\)", body)),
                    "strlist": bool(re.search(r"p\.\(\[\]string\)", body)),
                    "returns": results,
                }
    # Registered only inside a test, so not part of the surface.
    verbs.pop("test.residuals", None)

    # What a verb says about itself beats what a regular expression made of it.
    for name, spec in described().items():
        if name not in verbs:
            continue
        params, sole = [], False
        for prm in spec.get("params", []):
            if prm.get("primary"):
                sole = True
            params.append((prm["name"], prm.get("type", "")))
        verbs[name] = {
            "params": params,
            "sole": sole,
            "strlist": verbs[name]["strlist"],
            "returns": spec.get("returns", []),
            "what": spec.get("what", ""),
        }
    return verbs


def ui_only():
    """The verbs that refuse when no interface is attached.

    Read rather than listed, so a verb that grows or loses the guard moves in
    this table by itself.
    """
    out = set()
    # Every file in the package, not ui.go alone. The guard is what makes a
    # verb interface-only, and which file its handler happens to sit in is a
    # question of length limits: node.window, firmware.window and
    # node.output_window all guard and all live elsewhere, and reading one
    # file silently dropped the mark from the table.
    for name in sorted(os.listdir(SESSION)):
        if not name.endswith(".go") or name.endswith("_test.go"):
            continue
        src = open(os.path.join(SESSION, name)).read()
        for m in HANDLE.finditer(src):
            body = body_of(src, m.end())
            if "needUI()" in body or "need()" in body:
                out.add(m.group(1))
    return out


def takes(v):
    parts = []
    if v["sole"]:
        parts.append("*a bare string*")
    if v["strlist"]:
        parts.append("*a list of strings*")
    parts += [f"`{n}` {k}" for n, k in v["params"]]
    return ", ".join(parts) if parts else "—"


def returns(v):
    return ", ".join(f"`{r}`" for r in v["returns"]) if v["returns"] else "—"


def render(verbs, header):
    win = ui_only()
    out, seen = [header.rstrip("\n"), ""], set()
    for title, prefixes in GROUPS:
        rows = []
        for name in sorted(verbs):
            if name in seen or name.split(".")[0] not in prefixes:
                continue
            seen.add(name)
            v = verbs[name]
            f = FACADE[name]
            f = "`" + f + "`" if f else "*none* — " + WHY_NONE[name]
            flag = " 🪟" if name in win else ""
            rows.append(f"| `{name}`{flag} | {takes(v)} | {returns(v)} | {f} |")
        if rows:
            out += [f"### {title}", "", "| verb | takes | returns | façade |",
                    "|---|---|---|---|"] + rows + [""]
    return "\n".join(out)


def header_from_doc():
    """The prose above the first table, kept in the document and edited there.

    The tables are generated and the prose is written; splitting them into two
    files would mean nobody editing the prose ever sees the tables.
    """
    if not os.path.exists(DOC):
        sys.exit(f"{DOC} does not exist; its prose is written by hand")
    text = open(DOC).read()
    i = text.find("### ")
    if i < 0:
        sys.exit(f"{DOC} has no generated section to replace")
    return text[:i]


def main():
    verbs = scan()
    missing = sorted(set(verbs) - set(FACADE))
    if missing:
        sys.exit("no façade decision for: " + ", ".join(missing) +
                 "\nadd it to facade.json - a call, or \"\" plus a reason "
                 "in no_facade.")
    stale = sorted(set(FACADE) - set(verbs))
    if stale:
        sys.exit("facade.json names verbs that no longer exist: " +
                 ", ".join(stale))
    unexplained = sorted(k for k, v in FACADE.items()
                         if not v and k not in WHY_NONE)
    if unexplained:
        sys.exit("no reason given for having no façade: " +
                 ", ".join(unexplained))
    ungrouped = sorted(set(verbs) -
                       {n for n in verbs
                        for _, pre in GROUPS if n.split(".")[0] in pre})
    if ungrouped:
        sys.exit("ungrouped verbs: " + ", ".join(ungrouped))

    doc = render(verbs, header_from_doc())
    if "--check" in sys.argv:
        if open(DOC).read() != doc:
            sys.exit(f"{DOC} is out of date; run tools/verbdoc/verbdoc.py")
        print(f"{DOC} is current ({len(verbs)} verbs)")
        return
    open(DOC, "w").write(doc)
    print(f"{DOC}: {len(verbs)} verbs, "
          f"{sum(1 for v in FACADE.values() if not v)} deliberately unmapped")


if __name__ == "__main__":
    main()
