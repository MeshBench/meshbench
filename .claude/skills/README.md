# MeshBench agent skills

The skills an agent loads when working in this repository. Each is a directory
with a `SKILL.md` whose front-matter `description` says when it applies. The
front-matter `name` matches the directory, because the directory is what an
agent invokes and a `name` that says something else is a label nobody sees.

| skill | for |
|---|---|
| `meshbench-scripting` | writing and debugging scripts that drive the workbench |
| `meshcoresim` | driving the simulator to answer an RF or mesh question |
| `wb2-design-language` | building or changing the Gio interface |

## Published for installing elsewhere

Somebody driving MeshBench is not necessarily working in it, so the two skills
that are about using the tool are published as one standalone repository:

- **[agent-skills](https://github.com/MeshBench/agent-skills)**:
  `meshbench-scripting` and `meshcoresim`, driving and scripting the tool. The
  second is installed there under the directory name `meshbench-driving`,
  which is the one difference between the two trees.

That repository is also a **Claude Code plugin marketplace**, so it installs
the way Claude Code installs everything else rather than by copying directories
out of a clone:

```
/plugin marketplace add MeshBench/agent-skills
/plugin install meshbench@meshbench-skills
```

The plugin sits at the repository root and its one marketplace entry names
`./`, so the `skills/` tree the manual copy route uses is the same tree the
plugin installs and no `SKILL.md` is written twice. Plugin skills are
namespaced by the plugin, so an install gives `meshbench:meshbench-driving` and
`meshbench:meshbench-scripting`; a hand-copied directory keeps its bare name.
The marketplace is called `meshbench-skills` rather than after the repository,
because marketplace names share one namespace with every other marketplace a
user has added and `agent-skills` there says nothing about whose they are.

`wb2-design-language` is deliberately not published. It is for changing
MeshBench's own interface, and the only place that happens is a checkout of
this repository, so a standalone copy would be a second thing to keep current
for an audience that does not exist. `tools/skillmirror` records that reason
beside the mapping rather than leaving the absence to be read as an oversight.

**The copies here are canonical, and a mirror is an output.**
`tools/skillmirror` renders that tree from this directory, performs that
rename, and writes the front page, the licence and the two plugin manifests the
mirror needs and this one does not; `.github/workflows/publish-skills.yml`
pushes what it rendered when a skill changes on `main`. Nothing is hand-copied
and no second version of a skill is kept anywhere, so there is nothing for
these files to drift from. What a change here can still get wrong is the
mapping, and `tools/skillmirror`'s own tests fail without touching the network
when a skill is added that is neither published nor recorded as deliberately
unpublished, when a mirror names one that has gone, when front matter stops
naming its own directory, when the marketplace offers a plugin at a path
holding no skills, or when the front page tells people to type a name the
manifests do not carry.

Every published copy carries the commit it was rendered from, so somebody
holding an install can tell whether it is current instead of guessing.

The docs site explains what each skill is for and how to install it into
Claude Code, Cursor, VS Code, Gemini CLI and Codex:
<https://meshbench.github.io/docs/agent-skills.html>.

## The register they are written in

A skill is not reference material. `docs/scripting-verbs.md` already lists every
verb; a skill earns its place by saying what an agent would get wrong without
it, with the reason attached, so the rule survives the case it did not
anticipate.

Two consequences worth keeping. **A line that states a fact must name what
settles it**, a file, a verb or a command, because a stale skill is acted on
with more confidence than no skill at all. And **a count nobody generates does
not go in**: cite `tools/verbdoc/verbdoc.py`, or ask the running session, rather
than typing a number that will be wrong in a month.
