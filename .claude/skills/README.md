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

So they can be installed into any agent, not only one working in this repo, the
skills are published as two standalone repositories, split by audience:

- **[meshbench-scripting-skills](https://github.com/MeshBench/meshbench-scripting-skills)**:
  `meshbench-scripting` and `meshcoresim`, driving and scripting the tool. The
  second is installed there under the directory name `meshbench-driving`,
  which is the one difference between the two trees.
- **[meshbench-dev-skills](https://github.com/MeshBench/meshbench-dev-skills)**:
  `wb2-design-language`, developing the tool.

**Both mirror repositories are still private**, so those two links are a 404 to
anyone outside the organisation even though this repository is public. The
pipeline that fills them works; what has not happened is the decision to open
them. Said here rather than left as a broken link, which reads as a mistake.

**The copies here are canonical, and a mirror is an output.**
`tools/skillmirror` renders both trees from this directory, performs that
rename, and writes the front page and licence each mirror needs and this one
does not; `.github/workflows/publish-skills.yml` pushes what it rendered when a
skill changes on `main`. Nothing is hand-copied and no second version of a
skill is kept anywhere, so there is nothing for these files to drift from. What
a change here can still get wrong is the mapping, and `tools/skillmirror`'s own
test fails without touching the network when a skill is added that no mirror
publishes, when a mirror names one that has gone, or when front matter stops
naming its own directory.

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
