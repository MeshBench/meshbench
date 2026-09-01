# MeshBench development skills

Agent skills for **developing**
[MeshBench](https://github.com/MeshBench/meshbench) itself: the house rules a
contributor's agent should load before changing the code. For *using* MeshBench
from a script, see
[meshbench-scripting-skills](https://github.com/MeshBench/meshbench-scripting-skills).

| skill | when it loads |
|---|---|
{{.Table}}

## Installing

A skill is a directory with a `SKILL.md` whose front matter carries a `name`
and a `description`; an agent loads it by that description when a task matches.

- **Claude Code**: copy `skills/<name>` into your project's `.claude/skills/`,
  or into `~/.claude/skills/` for every project.
- **Other agents**: the content is plain Markdown; point your rules or skills
  directory at `skills/`, or paste a `SKILL.md` into the project rules.

```bash
git clone https://github.com/MeshBench/meshbench-dev-skills
cp -r meshbench-dev-skills/skills/* .claude/skills/
```

## Where to send a change

**This repository is generated.** It is rendered from `.claude/skills/` in
[MeshBench/meshbench](https://github.com/MeshBench/meshbench) by
`tools/skillmirror` and pushed when those skills change, so an edit made here
is overwritten by the next publish. Open the issue or the pull request against
MeshBench instead.

That is the arrangement rather than an inconvenience of it: a skill is only
worth loading if it can be corrected in the same commit as the change that made
it wrong, and that is only possible while it sits beside the code.

Published from MeshBench commit `{{.Commit}}`. Compare it against
[the canonical skills](https://github.com/MeshBench/meshbench/tree/main/.claude/skills)
if you want to know whether your copy is current.

Licensed GPL-3.0-or-later, with MeshBench itself. See `LICENSE`.
