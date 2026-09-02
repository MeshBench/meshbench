"""Which namespaces the three clients actually put on a workbench.

facade.json names the call that covers each verb, and until this was written
nothing held those names against anything. The four-way check binds a verb's
registration, its description, the manifest and the generated document, and a
facade entry sat outside all four: `wb.rf.mode` was printed in the reference for
five verbs, in a document a reader is expected to script from, and no client had
an `rf` at all - not Go, not Python, not Node.

Namespaces rather than whole calls, deliberately. A namespace is spelled the
same in all three clients, because that is how they were brought to parity:
`wb.nodes`, `wb.sim`, `wb.firmware`. The method after it is not - Python's
`close()` is Go's `Stop()` and Node's `close()`, and a check over method names
would spend its life reporting differences that are correct. The namespace is
the unit the fault occurred at, and it is the unit that can be checked exactly.
"""
import os
import re

# The shape each client declares a namespace in. One pattern each, because each
# client has exactly one way of doing it and a second way would be worth
# noticing rather than absorbing.
PYTHON = re.compile(r"def ([a-z_][a-z0-9_]*)\(self\)\s*->\s*[A-Z]")
GO = re.compile(r"func \(w \*Workbench\) ([A-Z][A-Za-z0-9]*)\(\)\s+[A-Z]")
NODE = re.compile(r"get ([a-z][A-Za-z0-9]*)\(\)\s*\{\s*return new ")


def _read(paths):
    out = []
    for path in paths:
        if os.path.isdir(path):
            for name in sorted(os.listdir(path)):
                if os.path.isfile(os.path.join(path, name)):
                    out.append(open(os.path.join(path, name)).read())
        elif os.path.exists(path):
            out.append(open(path).read())
    return "\n".join(out)


def namespaces(root):
    """The namespaces each client defines, keyed by client name.

    Go's are lowered, since the same namespace is `Nodes()` there and `nodes`
    in the other two and the difference is a language convention rather than a
    difference in the surface.
    """
    py = _read([os.path.join(root, "pkg", "client-python", "meshbench")])
    go = _read([os.path.join(root, "pkg", "client-go", "meshbench")])
    js = _read([os.path.join(root, "pkg", "client-js", "lib"),
                os.path.join(root, "pkg", "client-js", "meshbench.mjs")])
    return {
        "Python": set(PYTHON.findall(py)),
        "Go": {n.lower() for n in GO.findall(go)},
        "Node": set(js_name for js_name in NODE.findall(js)),
    }


# The namespace part of a facade entry: `wb.<name>.` and nothing else.
#
# Only wb: an entry rooted at `node.` is a method on a node handle and one at
# `meshbench.` is a module function, and neither is a namespace this can read.
# `wb.jobs[id]` and a bare `wb.nodes` are not namespace uses either - they are
# the object itself, which every client that has it answers with.
NAMESPACE = re.compile(r"\bwb\.([a-z_][a-z0-9_]*)\.")


def used_by(facade):
    """Every namespace facade.json spells, with the verbs that spell it."""
    out = {}
    for verb, call in sorted(facade.items()):
        for name in set(NAMESPACE.findall(call or "")):
            out.setdefault(name, []).append(verb)
    return out
