# MeshBench agent skills

Agent skills for **driving and scripting** a running
[MeshBench](https://github.com/MeshBench/meshbench) workbench from outside: the
Python, Go or Node client, the control socket, or raw verbs. These are for
people *using* MeshBench. The ones for developing MeshBench itself are not
published, because the only place that work happens is a checkout of
[the repository](https://github.com/MeshBench/meshbench).

| skill | when it loads |
|---|---|
{{.Table}}

Both encode what silent failures taught, which is the part a fresh reading of
the verb list will not give you.

## Installing

### Claude Code

This repository is also a plugin marketplace offering one plugin, so Claude
Code can install and update it for you:

```
/plugin marketplace add {{.Repo}}
/plugin install {{.Plugin}}@{{.Marketplace}}
```

Plugin skills are namespaced by the plugin, so once it is installed they are
called `{{.Plugin}}:meshbench-driving` and `{{.Plugin}}:meshbench-scripting`,
not by the bare directory names. Claude loads either on its own when a task
matches its description; the namespaced form is what you type to ask for one
directly.

**This repository is private today**, so both commands fail for anyone outside
the MeshBench organisation until that changes. Said here rather than left to be
read as a broken install.

### By hand, and for other agents

A skill is a directory with a `SKILL.md` whose front matter carries a `name`
and a `description`; an agent loads it by that description when a task matches.
The format is portable, so drop the directories where your agent looks. Copied
this way the skills keep their bare names, with no plugin to namespace them.

- **Claude Code**: copy `skills/<name>` into `.claude/skills/` in your project,
  or into `~/.claude/skills/` for every project. It is read on the next run.
- **Cursor, Windsurf and other agents**: point your agent's rules or skills
  directory at `skills/`, or paste a `SKILL.md` into the project rules. The
  content is plain Markdown; nothing here is specific to one agent bar the
  directory convention.

```bash
git clone https://github.com/{{.Repo}}
cp -r {{.RepoName}}/skills/* ~/.claude/skills/
```

The plugin sits at the repository root, so `skills/` is one tree serving both
routes rather than a second copy of every file.

## Where to send a change

**This repository is generated.** It is rendered from `.claude/skills/` in
[MeshBench/meshbench](https://github.com/MeshBench/meshbench) by
`tools/skillmirror` and pushed when those skills change, so an edit made here
is overwritten by the next publish. Open the issue or the pull request against
MeshBench instead.

That is the arrangement rather than an inconvenience of it: a skill is only
worth loading if it can be corrected in the same commit as the change that made
it wrong, and that is only possible while it sits beside the code.
`meshbench-driving` is this repository's name for the skill MeshBench calls
`meshcoresim`; the content is the same file.

Published from MeshBench commit `{{.Commit}}`. Compare it against
[the canonical skills](https://github.com/MeshBench/meshbench/tree/main/.claude/skills)
if you want to know whether your copy is current.

Licensed GPL-3.0-or-later, with MeshBench itself. See `LICENSE`.
