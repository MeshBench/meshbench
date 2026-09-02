package main

// installed is one canonical skill as a published mirror carries it.
type installed struct {
	// canonical is the directory under .claude/skills, which is the name the
	// skill has everywhere in this repository.
	canonical string

	// as is the directory the mirror installs it under, empty when it keeps
	// its own name. The one rename is deliberate: a mirror is a stranger's
	// front door, and somebody looking for the skill that drives the tool
	// searches for driving it, not for the simulator's old package name.
	as string

	// blurb is the line the mirror's README gives it. Presentation for an
	// audience the canonical tree does not have, which is why it is authored
	// here rather than cut down from the front-matter description.
	blurb string
}

// name is the directory the mirror installs the skill under, and therefore
// also the front-matter name the rendered copy has to announce.
func (i installed) name() string {
	if i.as != "" {
		return i.as
	}
	return i.canonical
}

// Who the published repositories belong to, said once because both manifests
// and the README all have to agree about it.
const (
	org      = "MeshBench"
	owner    = "MeshBench"
	ownerURL = "https://github.com/MeshBench"
	licence  = "GPL-3.0-or-later"
)

// mirror is one published repository.
type mirror struct {
	// repo is the name under github.com/MeshBench, which is both the output
	// directory and what the publisher pushes to.
	repo string

	// plugin is what makes the repository installable rather than only
	// cloneable. See plugin.go for why it sits at the repository root.
	plugin pluginOffer

	// skills is what it carries, in the order its README lists them.
	skills []installed
}

// pluginOffer is the mirror as Claude Code installs it.
type pluginOffer struct {
	// name is the plugin, and therefore the namespace every skill in it is
	// invoked under: a skill s becomes /<name>:s.
	name string

	// version is the release half of the plugin version. The commit is added
	// as build metadata at render time, so this changes only when what the
	// skills promise changes.
	version string

	// description is what both the plugin manifest and the marketplace entry
	// say. One sentence, in one place, because a catalogue whose entry
	// disagrees with the plugin it points at is a catalogue nobody trusts.
	description string

	// catalogue is the marketplace name, the half after the @ in
	// `/plugin install <plugin>@<catalogue>`. It is not the repository name:
	// marketplace names share one namespace in a user's settings alongside
	// every other marketplace they have added, and `agent-skills` there says
	// nothing about whose skills they are.
	catalogue string

	// marketplace is the catalogue's own description, which is about the
	// repository rather than about the plugin inside it.
	marketplace string

	// homepage is where somebody reads about the skills before installing
	// them, which is the docs site rather than the generated repository.
	homepage string
}

// mirrors is the split by audience. It is the whole mapping: mirror_test.go
// reads it against .claude/skills in both directions, so a skill added with no
// row and a row naming a skill that has gone each fail the build.
var mirrors = []mirror{
	{
		repo: "agent-skills",
		plugin: pluginOffer{
			// The plugin name prefixes every skill invocation, so it is the
			// product and nothing longer: /meshbench:meshbench-scripting.
			// That reads twice, which is the price of skill directories that
			// still say what they are when somebody copies one out by hand
			// into ~/.claude/skills, where there is no namespace to lean on.
			name: "meshbench",
			// Below one, because the skill directory names are what a caller
			// types and they are not yet promised to stay put.
			version: "0.1.0",
			description: "Drive and script a running MeshBench workbench: coverage, link budgets, " +
				"why a packet missed, and the clients and control socket that ask.",
			catalogue:   "meshbench-skills",
			marketplace: "MeshBench's own agent skills, published from the repository that defines them.",
			homepage:    "https://meshbench.github.io/docs/agent-skills.html",
		},
		skills: []installed{
			{
				canonical: "meshcoresim",
				as:        "meshbench-driving",
				blurb: "driving the simulator to answer an RF or mesh question: coverage, " +
					"why a packet missed, site selection, a firmware A/B",
			},
			{
				canonical: "meshbench-scripting",
				blurb: "writing or debugging a script that opens a session, brings a mesh up " +
					"and waits for it: an example, a CI check, a soak driver",
			},
		},
	},
}

// unpublished is a canonical skill that is deliberately not mirrored, and why.
//
// Kept as a list rather than as silence, so the test can still insist that a
// new skill is a decision: one that is neither published nor named here fails
// the build, and adding a skill nobody thought about cannot quietly do nothing.
var unpublished = map[string]string{
	"wb2-design-language": "it is for changing MeshBench's own interface, and the " +
		"only place that happens is a checkout of this repository, so a standalone " +
		"copy would be a second thing to keep current for an audience that does not exist",
}
