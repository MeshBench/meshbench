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

// mirror is one published repository.
type mirror struct {
	// repo is the name under github.com/MeshBench, which is both the output
	// directory and what the publisher pushes to.
	repo string

	// skills is what it carries, in the order its README lists them.
	skills []installed
}

// mirrors is the split by audience. It is the whole mapping: mirror_test.go
// reads it against .claude/skills in both directions, so a skill added with no
// row and a row naming a skill that has gone each fail the build.
var mirrors = []mirror{
	{
		repo: "meshbench-agent-skills",
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
