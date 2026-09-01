#!/usr/bin/env python3
"""Regenerate docs/cli-reference.md from the flags the binary actually declares.

    tools/flagdoc/flagdoc.py            write the document
    tools/flagdoc/flagdoc.py --check    fail if it is out of date

It builds the binary and asks it: every command takes -h and prints its flags,
their types and their defaults, so the page cannot describe a flag that is not
there. Reading the flag declarations out of the source with a regular
expression would have been quicker and would have been a second implementation
of the flag package, wrong in the places that matter - a default computed at
startup, a flag registered inside a helper, a type only the printer knows.

Two things the binary cannot say are authored in flags.json beside this file:
what each flag is for, and whether the command refuses to run without it. Data
rather than code, so it reads as the manifest somebody reviews, and a flag with
no entry fails the run.

The examples are the third part, and they live in cmd/meshbench/commands.go
because they are Go's to keep beside the commands. Nothing here can check that
an example still produces what its note claims - a person runs it - but every
flag an example names is checked against the flag set, so an example cannot go
on documenting a flag that has gone.
"""
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(os.path.dirname(HERE))
DOC = os.path.join(ROOT, "docs", "cli-reference.md")
MANIFEST = os.path.join(HERE, "flags.json")
SOURCE = os.path.join(ROOT, "cmd", "meshbench", "commands.go")

BEGIN = "<!-- BEGIN GENERATED CLI -->"
END = "<!-- END GENERATED CLI -->"

# The counts the hand-written prose above the tables states, each marked with
# an HTML comment against the number it belongs to. A stale digit in front of
# the marker is overwritten, which is what lets --check catch it.
COUNTS = ("commands", "flags", "capture")
MARKER = re.compile(r"<!--flagdoc:([a-z]+)-->")

# An absolute path, because os.UserCacheDir refuses a relative XDG_CACHE_HOME
# and the command would then print a bare "meshbench/terrain". Substituted out
# again below: the page says <cache>, and the binary prints the real path.
SENTINEL = "/meshbench-cache-sentinel"

# The usage line parse() prints, whose separator is an em dash. Spelled as an
# escape because the house style bans the character in source.
DESCRIBE = re.compile("^\\S+ (\\S+) \\u2014 (.+)$")

DEFAULT = re.compile(r"\s*\(default (.*)\)$")

# What a flag of each printed type is when nothing was given. PrintDefaults
# omits the default when it is the type's zero, so this is the only place the
# difference between an unset string and an unset duration survives.
ZERO = {
    "": "`false`", "float": "`0`", "int": "`0`", "uint": "`0`",
    "duration": "`0s`", "string": "none",
}

ENTRY = re.compile(r'name:\s*"([a-z]+)",\s*\n\s*summary:\s*"((?:[^"\\]|\\.)*)"')
SHELL = re.compile(r"shell:\s*`([^`]*)`")
NOTE = re.compile(r"note:\s*`([^`]*)`")


def commands():
    """The command table, its summaries and its examples, from the Go source.

    Read here rather than asked of the binary because the examples are not
    printed by anything: they exist to be put on this page and to be run.
    """
    src = open(SOURCE).read()
    entries = list(ENTRY.finditer(src))
    out = []
    for i, m in enumerate(entries):
        end = entries[i + 1].start() if i + 1 < len(entries) else len(src)
        chunk = src[m.end():end]
        shells = SHELL.findall(chunk)
        notes = NOTE.findall(chunk)
        if len(shells) != len(notes):
            sys.exit(f"{m.group(1)}: {len(shells)} examples but {len(notes)} notes")
        out.append({
            "name": m.group(1),
            "summary": json.loads('"' + m.group(2) + '"'),
            "examples": list(zip(shells, notes)),
        })
    if not out:
        sys.exit(f"no commands found in {SOURCE}")
    return out


def build():
    """The binary, in a directory this script owns and removes."""
    d = tempfile.mkdtemp(prefix="flagdoc")
    binary = os.path.join(d, "meshbench")
    r = subprocess.run(["go", "build", "-o", binary, "./cmd/meshbench"],
                       cwd=ROOT, capture_output=True, text=True)
    if r.returncode != 0:
        shutil.rmtree(d, ignore_errors=True)
        sys.exit("go build failed:\n" + r.stderr)
    return binary


def ask(binary, name):
    """One command's own help, as the flag package prints it."""
    env = dict(os.environ, XDG_CACHE_HOME=SENTINEL)
    r = subprocess.run([binary, name, "-h"], capture_output=True, text=True, env=env)
    text = (r.stdout + r.stderr).rstrip("\n")
    if not text:
        sys.exit(f"{name} -h printed nothing")
    return text


def parse_help(text, summary):
    """The description and the flag set, out of the help text.

    The description is preferred over the command table's summary because it is
    what the command says about itself when asked; workbench parses with the
    default flag set and prints no such line, and falls back.
    """
    lines = text.splitlines()
    m = DESCRIBE.match(lines[0])
    describe = m.group(2) if m else summary
    flags, cur = [], None
    for line in lines[1:]:
        if line.startswith("  -"):
            head, tab, rest = line[2:].partition("\t")
            parts = head.split()
            cur = {"name": parts[0][1:],
                   "type": parts[1] if len(parts) > 1 else "",
                   "usage": rest.strip() if tab else ""}
            flags.append(cur)
        elif cur is not None and line.startswith("    \t"):
            cur["usage"] = (cur["usage"] + " " + line.strip()).strip()
    for f in flags:
        f["default"] = shown_default(f)
    return describe, flags


def shown_default(f):
    """What the page prints in the default column, and what it leaves in usage.

    PrintDefaults appends the default to the usage text, so the two have to be
    separated here; a flag sitting at its type's zero has nothing appended at
    all, which is where ZERO comes in.
    """
    m = DEFAULT.search(f["usage"])
    if not m:
        return ZERO.get(f["type"], "`" + f["type"] + " zero value`")
    f["usage"] = f["usage"][:m.start()]
    value = m.group(1)
    if len(value) > 1 and value.startswith('"') and value.endswith('"'):
        value = value[1:-1]
    value = value.replace(SENTINEL, "<cache>")
    if os.path.expanduser("~") in value:
        sys.exit(f"-{f['name']} names this machine's home directory in its "
                 "default; XDG_CACHE_HOME did not reach os.UserCacheDir")
    return "`" + value + "`"


def check_manifest(cmds, spec):
    """Every flag has a purpose, and the manifest names nothing that has gone."""
    vocab = spec["purposes"]
    named, have = set(spec["commands"]), {c["name"] for c in cmds}
    if named != have:
        sys.exit("flags.json and the binary disagree about the commands: "
                 f"only in flags.json {sorted(named - have)}, "
                 f"only in the binary {sorted(have - named)}")
    for c in cmds:
        entry = spec["commands"][c["name"]]
        flags = {f["name"] for f in c["flags"]}
        missing = sorted(flags - set(entry["for"]))
        if missing:
            sys.exit(f"{c['name']}: no purpose in flags.json for " +
                     ", ".join("-" + n for n in missing) +
                     "\nadd one of: " + ", ".join(sorted(vocab)))
        stale = sorted(set(entry["for"]) - flags)
        if stale:
            sys.exit(f"{c['name']}: flags.json names flags that have gone: " +
                     ", ".join("-" + n for n in stale))
        for n, p in entry["for"].items():
            if p not in vocab:
                sys.exit(f"{c['name']} -{n}: no such purpose {p!r}")
        for n in entry["required"]:
            if n not in flags:
                sys.exit(f"{c['name']}: -{n} is marked required and does not exist")


def check_examples(cmds):
    """Every flag an example names is one the command still declares."""
    for c in cmds:
        flags = {f["name"] for f in c["flags"]}
        for shell, _ in c["examples"]:
            for line in shell.splitlines():
                if not line.startswith("meshbench " + c["name"]):
                    continue
                for token in shlex.split(line)[2:]:
                    if not (token.startswith("-") and len(token) > 1
                            and token[1].isalpha()):
                        continue
                    named = token[1:].split("=")[0]
                    if named not in flags:
                        sys.exit(f"{c['name']}: the example uses -{named}, "
                                 "which that command does not declare")


def sentence(s):
    """A help line as the page states it: one sentence, ended."""
    s = s[:1].upper() + s[1:]
    return s if s.endswith((".", "!", "?")) else s + "."


def render(cmds, spec):
    """Everything between the fences."""
    vocab = spec["purposes"]
    out = ["| for | meaning |", "|---|---|"]
    out += [f"| {k} | {vocab[k]} |" for k in sorted(vocab)]
    out += ["", "## The commands", "",
            "| command | what it does | flags |", "|---|---|---|"]
    out += [f"| `{c['name']}` | {c['summary']} | {len(c['flags'])} |" for c in cmds]
    for c in cmds:
        out += ["", "## `meshbench " + c["name"] + "`", "",
                sentence(c["describe"]), ""]
        for shell, note in c["examples"]:
            out += ["```console"] + shell.splitlines() + ["```", "", note, ""]
        entry = spec["commands"][c["name"]]
        if not c["flags"]:
            out.append("It takes no flags.")
            continue
        out += ["| flag | default | for | meaning |", "|---|---|---|---|"]
        for f in c["flags"]:
            default = f["default"]
            if f["name"] in entry["required"]:
                default = "**required**"
            out.append(f"| `-{f['name']}` | {default} | "
                       f"{entry['for'][f['name']]} | {f['usage']} |")
    return "\n".join(out)


def counts(cmds, spec):
    """The three figures the prose states, none of them typed by hand."""
    flags = sum(len(c["flags"]) for c in cmds)
    capture = sum(1 for c in spec["commands"].values()
                  for p in c["for"].values() if p == "capture")
    return {"commands": len(cmds), "flags": flags, "capture": capture}


def header(figures):
    """The prose above the tables, kept in the document and edited there.

    Only the figures inside it are filled in, so that a number nobody can check
    cannot sit in a sentence looking checked.
    """
    if not os.path.exists(DOC):
        sys.exit(f"{DOC} does not exist; its prose is written by hand")
    text = open(DOC).read()
    i = text.find(BEGIN)
    if i < 0:
        sys.exit(f"{DOC} has no {BEGIN} line to generate below")
    text = text[:i]
    for key in MARKER.findall(text):
        if key not in COUNTS:
            sys.exit(f"{DOC} carries a <!--flagdoc:{key}--> marker, which is "
                     "not one of the counts it states: " + ", ".join(COUNTS))
    for key in COUNTS:
        mark = f"<!--flagdoc:{key}-->"
        pattern = re.compile(r"\*\*\d+\*\*" + re.escape(mark))
        text, filled = pattern.subn(f"**{figures[key]}**{mark}", text)
        if filled == 0:
            sys.exit(f"{DOC} prose is missing the {mark} marker")
    return text


def main():
    cmds = commands()
    spec = json.load(open(MANIFEST))
    binary = build()
    try:
        for c in cmds:
            c["describe"], c["flags"] = parse_help(ask(binary, c["name"]), c["summary"])
    finally:
        shutil.rmtree(os.path.dirname(binary), ignore_errors=True)

    check_manifest(cmds, spec)
    check_examples(cmds)
    figures = counts(cmds, spec)
    page = header(figures) + BEGIN + "\n\n" + render(cmds, spec) + "\n\n" + END + "\n"

    if "--check" in sys.argv:
        if open(DOC).read() != page:
            sys.exit(f"{DOC} is out of date; run tools/flagdoc/flagdoc.py")
        print(f"{DOC} is current ({figures['commands']} commands, "
              f"{figures['flags']} flags)")
        return
    open(DOC, "w").write(page)
    print(f"{DOC}: {figures['commands']} commands, {figures['flags']} flags, "
          f"{figures['capture']} of them for capture and scripting")


if __name__ == "__main__":
    main()
