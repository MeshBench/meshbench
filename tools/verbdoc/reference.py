"""The per-verb reference: what each verb is for, what it takes, what it
answers, and a call that has been run.

Separate from verbdoc.py because it renders a different document from the same
scan. verbdoc.py's table is the whole surface at a glance, one row per verb,
and is what a reviewer reads to see that nothing has fallen off it. This is the
entry a person reads when they are about to call one verb and need to know what
happens.

Everything here comes from docs/verbs.json, which is written from the
registrations themselves. A verb that has not been described yet still gets an
entry, marked as undescribed and filled from what the handler could be read to
do, because leaving it out would make the page look finished when it is not.
"""
import json


def _spec(v):
    """The authored spec for a verb, or None where it has none yet."""
    return v.get("spec")


def _params_table(spec):
    rows = ["| parameter | type | | what |", "|---|---|---|---|"]
    for p in spec.get("params", []):
        need = "required" if p.get("required") else "optional"
        if p.get("primary"):
            need += ", primary"
        rows.append("| `%s` | %s | %s | %s |" %
                    (p["name"], p.get("type", ""), need, p.get("what", "")))
    return rows


def _example_block(name, ex):
    line = json.dumps({"id": 1, "method": name, "params": ex["params"]},
                      sort_keys=True, separators=(",", ":"))
    out = []
    if ex.get("what"):
        out.append("**Example** - %s" % ex["what"])
    else:
        out.append("**Example**")
    out += ["", "```json", line, "```"]
    if not ex.get("runnable"):
        out += ["", "Not made by the test suite: this call needs more than the "
                    "two-node headless session the runnable examples go to."]
    return out


def _takes_line(v):
    """What the handler could be read to take, for a verb with no spec."""
    parts = []
    if v["sole"]:
        parts.append("a bare string")
    if v["strlist"]:
        parts.append("a list of strings")
    parts += ["`%s` (%s)" % (n, k) for n, k in v["params"]]
    return ", ".join(parts)


def _described_entry(name, spec):
    # Capitalised here rather than at the registration: the same sentence is a
    # table cell in scripting-verbs.md, where the imperative reads better
    # lower case, and a paragraph here, where it does not.
    what = spec["what"].rstrip(".")
    out = ["", what[:1].upper() + what[1:] + "."]
    if spec.get("params"):
        out += ["", "**Takes**", ""] + _params_table(spec)
    else:
        out += ["", "**Takes** nothing."]
    if spec.get("returns"):
        out += ["", "**Answers** " +
                ", ".join("`%s`" % r for r in spec["returns"]) +
                ("" if not spec.get("answers") else ". " + spec["answers"])]
    elif spec.get("answers"):
        out += ["", "**Answers** " + spec["answers"]]
    if spec.get("example"):
        out += [""] + _example_block(name, spec["example"])
    return out


def _undescribed_entry(v):
    takes = _takes_line(v)
    answers = ", ".join("`%s`" % r for r in v["returns"])
    said = "**Not described yet.** "
    said += ("Reads " + takes + "." if takes else "Reads no parameters.")
    if answers:
        said += " Answers " + answers + "."
    said += (" Read out of the handler rather than said by it, so it is what"
             " the code does and not what it is for.")
    return ["", said]


def entry(name, v, facade, why_none, needs_window, internal):
    """One verb, as the reference prints it."""
    flags = []
    if name in internal:
        flags.append("The workbench's own callback. The socket refuses it")
    if name in needs_window:
        flags.append("Refuses when no window is attached")
    # The heading is the verb and nothing else: the documentation site turns a
    # heading into an anchor, and a flag in the heading puts the flag in the
    # anchor, so every link to the verb breaks the day it grows or loses one.
    out = ["### `%s`" % name]
    if flags:
        out += ["", "**" + ". ".join(flags) + ".**"]
    spec = _spec(v)
    if spec and spec.get("what"):
        out += _described_entry(name, spec)
    else:
        out += _undescribed_entry(v)
    call = facade.get(name)
    if call:
        out += ["", "**Client** `%s`" % call]
    elif name in why_none:
        out += ["", "**Client** none: " + why_none[name]]
    return out + [""]


def render(verbs, groups, facade, why_none, needs_window, internal, counts):
    """The whole document."""
    out = [HEADER % counts, ""]
    seen = set()
    for title, prefixes in groups:
        names = sorted(n for n in verbs
                       if n not in seen and n.split(".")[0] in prefixes)
        if not names:
            continue
        seen.update(names)
        out += ["## " + title, ""]
        for n in names:
            out += entry(n, verbs[n], facade, why_none, needs_window, internal)
    return "\n".join(out).rstrip("\n") + "\n"


HEADER = """# The control socket, verb by verb

Generated. Run `tools/verbdoc/verbdoc.py` to rewrite it and
`tools/verbdoc/verbdoc.py --check` to fail when it is stale.

The store registers %(total)d verbs: %(public)d a script may call and
%(internal)d the workbench calls on itself, which the socket refuses. Of those,
%(described)d say what they are for and %(undescribed)d do not yet; the ones that
do not are marked, and what is printed for them is read out of the handler
rather than said by it.

Each entry is written where the verb is registered, in the `state.Spec` handed
to `st.HandleSpec`, so it cannot go stale without the code changing. Every
example is a request line for the socket. The ones not marked otherwise are
made against a live session by the test suite, so an example that has stopped
working fails the build rather than the reader.

A call is one line of newline-delimited JSON:

```json
{"id":1,"method":"sim.state","params":{}}
```
"""
