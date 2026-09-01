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

## Mirrored for installing elsewhere (#242)

So they can be installed into any agent, not only one working in this repo, the
skills are mirrored into two standalone repositories:

- **[meshbench-scripting-skills](https://github.com/MeshBench/meshbench-scripting-skills)**:
  `meshbench-scripting` and `meshcoresim`, driving and scripting the tool. The
  second is installed there under the directory name `meshbench-driving`,
  which is the one difference between the two trees.
- **[meshbench-dev-skills](https://github.com/MeshBench/meshbench-dev-skills)**:
  `wb2-design-language`, developing the tool.

**The copies here are canonical.** The mirroring is a manual copy: nothing
generates it, nothing checks it, and a mirror has already been found a line
behind. When one of these skills changes, update the mirror repository in the
same breath, and diff the two before believing they agree.

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
