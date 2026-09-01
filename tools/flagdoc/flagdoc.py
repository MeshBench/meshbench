#!/usr/bin/env python3
"""Regenerate the generated half of docs/cli-reference.md from the binary's own
flag declarations, and fill the counts the prose above it states.

    tools/flagdoc/flagdoc.py            write the document
    tools/flagdoc/flagdoc.py --check    fail if it is out of date
    tools/flagdoc/flagdoc.py --stdout   print it

It builds cmd/meshbench and asks it: `meshbench flagdump` walks every command,
takes the flag.FlagSet each one declares at the moment it declares it, and
prints the lot. That costs a build, and it buys truth. The defaults a command
computes at startup - a cache directory under the user's home, a tile zoom that
comes from a constant in another package - are values no reader of
cmd/meshbench can work out, and the workbench's thirty-eight flags are not
declared in cmd/meshbench at all. Parsing the source would have been cheaper
and would have had to guess at all of it.

Three things extraction cannot do, and each is authored in roles.json beside
this script, as data rather than as code so it reads as the manifest somebody
reviews:

  - what a flag is *for*. The reference has to separate a flag that changes an
    answer from one that only reaches a panel without a click, and nothing in a
    declaration says which it is. A flag with no entry fails this script.
  - which flags are required. Required-ness is decided after parsing, by a
    requireAll() the dump never reaches, so it is stated here and then checked
    against the binary: the command is run bare and has to fail naming every
    flag the manifest claims it needs.
  - the examples, which live beside the commands in cmd/meshbench/commands.go
    rather than here, because an example is about one command. What is checked
    of them here is that every flag they name still exists. That an example
    was run is a thing a person does; that it still refers to real flags is a
    thing this catches.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
DOC = os.path.join(ROOT, "docs", "cli-reference.md")
MANIFEST = os.path.join(HERE, "roles.json")

BEGIN = "<!-- BEGIN GENERATED CLI -->"
END = "<!-- END GENERATED CLI -->"
MARKER = re.compile(r"<!--flagdoc:([a-z]+)-->")

# Which counts the prose states. Filled here rather than typed, for the reason
# the verb counts are: a number nobody can check is worse than no number, and
# the one this page opened with had been wrong since the page was written.
COUNTS = ("commands", "flags", "capture")

# What a default of "" is shown as. An empty pair of backticks reads as a
# rendering fault rather than as an absent value.
NO_DEFAULT = "none"


def manifest():
    with open(MANIFEST) as f:
        return json.load(f)


def build(into):
    """Build the binary this document describes."""
    out = os.path.join(into, "meshbench")
    r = subprocess.run(["go", "build", "-o", out, "./cmd/meshbench"],
                       cwd=ROOT, capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit("building cmd/meshbench failed:\n" + r.stderr)
    return out


def dump(binary):
    r = subprocess.run([binary, "flagdump"], capture_output=True, text=True)
    if r.returncode != 0:
        sys.exit("%s flagdump failed:\n%s" % (binary, r.stderr))
    return json.loads(r.stdout)


def role_of(spec, command, flag):
    per = spec["commands"].get(command, {})
    if flag in per:
        return per[flag]
    return spec["shared"].get(flag)


def check_roles(spec, commands):
    """Every flag has a decision about what it is for, and none is left over."""
    missing = [(c["name"], f["name"]) for c in commands
               for f in c["flags"] or []
               if role_of(spec, c["name"], f["name"]) is None]
    if missing:
        sys.exit("no role for: " + ", ".join("%s -%s" % m for m in missing) +
                 "\nadd each to roles.json - one of: " +
                 ", ".join(sorted(spec["roles"])))
    declared = {(c["name"], f["name"]) for c in commands for f in c["flags"] or []}
    stale = [(c, f) for c, flags in spec["commands"].items() for f in flags
             if (c, f) not in declared]
    if stale:
        sys.exit("roles.json names flags that no longer exist: " +
                 ", ".join("%s -%s" % s for s in stale))
    unknown = {r for c in commands for f in c["flags"] or []
               for r in [role_of(spec, c["name"], f["name"])]
               if r not in spec["roles"]}
    if unknown:
        sys.exit("roles.json uses undefined roles: " + ", ".join(sorted(unknown)))


def check_required(spec, binary, commands):
    """The manifest says which flags a command refuses to run without. Ask it.

    Run bare, a command with required flags must fail and must name every one
    of them. That turns an authored list into a checked one: a flag that stops
    being required, or is renamed, fails here rather than in the reader's
    terminal.
    """
    by_name = {c["name"]: c for c in commands}
    for command, names in sorted(spec["required"].items()):
        if command not in by_name:
            sys.exit("roles.json requires flags of %r, which is not a command" % command)
        have = {f["name"] for f in by_name[command]["flags"] or []}
        for n in names:
            if n not in have:
                sys.exit("roles.json says %s needs -%s, which it does not declare" % (command, n))
        r = subprocess.run([binary, command], capture_output=True, text=True)
        said = r.stdout + r.stderr
        if r.returncode == 0:
            sys.exit("roles.json says %s needs %s, but it runs with no arguments" %
                     (command, ", ".join("-" + n for n in names)))
        for n in names:
            if "-" + n not in said:
                sys.exit("roles.json says %s needs -%s, but running it bare said:\n%s" %
                         (command, n, said.strip()))


def check_examples(commands):
    """Every flag an example names is still a flag that command has."""
    for c in commands:
        have = {f["name"] for f in c["flags"] or []}
        for ex in c["examples"] or []:
            head = ex["line"].split()
            if len(head) < 2 or head[0] != "meshbench" or head[1] != c["name"]:
                sys.exit("%s: an example must start 'meshbench %s': %s" %
                         (c["name"], c["name"], ex["line"]))
            for token in head[2:]:
                if not token.startswith("-") or _is_negative_number(token):
                    continue
                name = token.lstrip("-").split("=")[0]
                if name not in have:
                    sys.exit("%s: the example names -%s, which it does not declare" %
                             (c["name"], name))
        if not (c["examples"] or []):
            sys.exit("%s has no worked example; add one in cmd/meshbench/commands.go" % c["name"])


def _is_negative_number(token):
    """A value, not a flag: -3.4260 is a longitude and -120 is a signal level."""
    return re.fullmatch(r"-\d+(\.\d+)?", token) is not None


def opening(line):
    """One line of a command's own -h, as the first sentence of its section.

    Only the first character is raised. str.capitalize() lowers the rest, which
    turned "what an SDR observer captures" into "an sdr observer" and made the
    generator look like it had never been read.
    """
    return line[:1].upper() + line[1:]


def portable(value, cache_dir):
    """A default that names this machine's cache, said in a way that travels."""
    if cache_dir and value.startswith(cache_dir):
        return "<cache>" + value[len(cache_dir):]
    return value


def flag_table(spec, command, cache_dir):
    required = set(spec["required"].get(command["name"], []))
    rows = ["| flag | default | for | meaning |", "|---|---|---|---|"]
    for f in command["flags"] or []:
        if f["name"] in required:
            default = "**required**"
        elif f["default"] == "":
            default = NO_DEFAULT
        else:
            default = "`%s`" % portable(f["default"], cache_dir)
        rows.append("| `-%s` | %s | %s | %s |" %
                    (f["name"], default, role_of(spec, command["name"], f["name"]),
                     f["usage"]))
    return rows


def render(spec, data):
    cache_dir = data.get("cache_dir", "")
    commands = data["commands"]
    out = [BEGIN, "", "## What a flag is for", "",
           "Every flag below carries one of these, because a flag that arranges "
           "a screenshot and a flag that changes an answer are not the same kind "
           "of thing and a reference that lists them together is misleading.", "",
           "| for | meaning |", "|---|---|"]
    for name in sorted(spec["roles"]):
        out.append("| %s | %s |" % (name, spec["roles"][name]))
    out += ["", "## The commands", "",
            "| command | what it does | flags |", "|---|---|---|"]
    for c in commands:
        out.append("| `%s` | %s | %d |" % (c["name"], c["summary"], len(c["flags"] or [])))
    out.append("")

    for c in commands:
        out += ["## `meshbench %s`" % c["name"], "",
                opening(c["describe"] or c["summary"]) + ".", ""]
        for ex in c["examples"] or []:
            body = [ex["setup"]] if ex.get("setup") else []
            body.append(ex["line"])
            out += ["```console"] + body + ["```", "", ex["why"], ""]
        if c["flags"]:
            out += flag_table(spec, c, cache_dir) + [""]
        else:
            out += ["It takes no flags.", ""]
    out.append(END)
    return "\n".join(out)


def counts(spec, data):
    flags = [(c["name"], f["name"]) for c in data["commands"] for f in c["flags"] or []]
    return {
        "commands": len(data["commands"]),
        "flags": len(flags),
        "capture": sum(1 for c, f in flags if role_of(spec, c, f) == "capture"),
    }


def substitute_counts(text, values):
    """Fill the counts marked in the hand-written prose above the block.

    Each is marked with an HTML comment straight after the bold number it
    belongs to, so the sentence still reads as prose wherever it renders. What
    makes it generated is that a stale digit in front of the marker is
    overwritten, which is what lets --check catch one.
    """
    for key in MARKER.findall(text):
        if key not in values:
            sys.exit("%s carries a <!--flagdoc:%s--> marker, which is not one of "
                     "the counts it states: %s" % (DOC, key, ", ".join(COUNTS)))
    for key in COUNTS:
        marker = "<!--flagdoc:%s-->" % key
        text, n = re.subn(r"\*\*\d+\*\*" + re.escape(marker),
                          "**%d**%s" % (values[key], marker), text)
        if n == 0:
            sys.exit("%s prose is missing the %s marker" % (DOC, marker))
    return text


def page(spec, data):
    if not os.path.exists(DOC):
        sys.exit("%s does not exist; its prose above the marker is written by hand" % DOC)
    text = open(DOC).read()
    if BEGIN not in text or END not in text:
        sys.exit("%s has no generated block; add the markers first" % DOC)
    text = substitute_counts(text, counts(spec, data))
    return re.sub(re.escape(BEGIN) + r".*?" + re.escape(END),
                  lambda _: render(spec, data), text, flags=re.S)


def main():
    spec = manifest()
    tmp = tempfile.mkdtemp(prefix="flagdoc-")
    try:
        binary = build(tmp)
        data = dump(binary)
        check_roles(spec, data["commands"])
        check_required(spec, binary, data["commands"])
        check_examples(data["commands"])
        want = page(spec, data)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    if "--stdout" in sys.argv:
        sys.stdout.write(want)
        return
    n = counts(spec, data)
    if "--check" in sys.argv:
        if open(DOC).read() != want:
            sys.exit("%s is out of date; run tools/flagdoc/flagdoc.py" % DOC)
        print("%s is current (%d commands, %d flags, %d for capture and scripting)" %
              (DOC, n["commands"], n["flags"], n["capture"]))
        return
    open(DOC, "w").write(want)
    print("%s: %d commands, %d flags, %d for capture and scripting" %
          (DOC, n["commands"], n["flags"], n["capture"]))


if __name__ == "__main__":
    main()
