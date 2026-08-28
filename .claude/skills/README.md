# MeshBench agent skills

The skills an agent loads when working in this repository. Each is a directory
with a `SKILL.md` whose front-matter `description` says when it applies.

| skill | for |
|---|---|
| `meshbench-scripting` | writing and debugging scripts that drive the workbench |
| `meshcoresim` | driving the simulator to answer an RF or mesh question |
| `wb2-design-language` | building or changing the Gio interface |

## Mirrored for installing elsewhere (#242)

So they can be installed into any agent, not only one working in this repo, the
skills are mirrored into two standalone repositories:

- **[meshbench-scripting-skills](https://github.com/MeshBench/meshbench-scripting-skills)**
  — `meshbench-scripting` and `meshcoresim`: driving and scripting the tool.
- **[meshbench-dev-skills](https://github.com/MeshBench/meshbench-dev-skills)**
  — `wb2-design-language`: developing the tool.

**The copies here are canonical.** When one of these skills changes, update the
mirror repository in the same breath, or the installed copies go stale — the
mirror READMEs say the same from their side.
