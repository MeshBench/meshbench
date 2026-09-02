package main

import (
	"encoding/json"
	"fmt"
	"path"
)

// The manifests that make a mirror installable the way Claude Code installs
// everything else, rather than by cloning it and copying directories out.
//
// There is no install command for a bare skills repository. The supported route
// is a plugin offered by a marketplace: a marketplace is a repository carrying
// .claude-plugin/marketplace.json, a plugin is a directory carrying
// .claude-plugin/plugin.json with its skills in skills/<name>/SKILL.md.
//
// The mirror is both. The plugin sits at the repository root and the
// marketplace's one entry names "./", so the skills/ tree the manual copy
// route already uses is the same tree the plugin installs, and no SKILL.md is
// written twice. Putting the plugin in a plugins/<name>/ subdirectory would
// mean either a second copy of every skill or a repository whose top level is
// no longer something a person can copy out of. The layout is unusual enough to
// be worth checking rather than assuming: `claude plugin validate` passes on
// it, and adding the rendered tree as a local marketplace installs the plugin
// with both skills found.

// pluginManifest is .claude-plugin/plugin.json.
//
// Its Name is the namespace every skill in the plugin is invoked under, so it
// is the product's name and nothing longer.
type pluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      party  `json:"author"`
	Homepage    string `json:"homepage"`
	Repository  string `json:"repository"`
	License     string `json:"license"`
}

// marketplaceManifest is .claude-plugin/marketplace.json, the catalogue
// `/plugin marketplace add` reads.
type marketplaceManifest struct {
	Name        string             `json:"name"`
	Owner       party              `json:"owner"`
	Description string             `json:"description"`
	Plugins     []marketplaceEntry `json:"plugins"`
}

// marketplaceEntry is one offered plugin. Source is relative to the
// marketplace root and not to .claude-plugin/, which is what lets the entry
// point at the repository it is already inside.
type marketplaceEntry struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

// party is whoever is named as author or owner.
type party struct {
	Name string `json:"name"`
	URL  string `json:"url,omitempty"`
}

// rootSource is the marketplace entry pointing at its own repository root.
const rootSource = "./"

// manifestDir is where both manifests live, for a plugin at the root.
const manifestDir = ".claude-plugin"

// renderPlugin is the two manifests, filled in from the same mapping the skill
// tree and the front page come from so that what a user installs cannot come to
// disagree with what was published.
func renderPlugin(sha string, m mirror) ([]file, error) {
	p := m.plugin
	if p.name == "" || p.catalogue == "" || p.description == "" {
		return nil, fmt.Errorf("%s: a mirror missing a plugin name, a marketplace name or a"+
			" description is not something anybody can install", m.repo)
	}

	pluginJSON, err := marshal(pluginManifest{
		Name:        p.name,
		Description: p.description,
		Version:     stampedVersion(p.version, sha),
		Author:      party{Name: owner, URL: ownerURL},
		Homepage:    p.homepage,
		Repository:  "https://github.com/" + org + "/" + m.repo,
		License:     licence,
	})
	if err != nil {
		return nil, err
	}

	marketJSON, err := marshal(marketplaceManifest{
		Name:        p.catalogue,
		Owner:       party{Name: owner, URL: ownerURL},
		Description: p.marketplace,
		Plugins: []marketplaceEntry{{
			Name:        p.name,
			Source:      rootSource,
			Description: p.description,
		}},
	})
	if err != nil {
		return nil, err
	}

	return []file{
		{path: path.Join(manifestDir, "plugin.json"), content: pluginJSON},
		{path: path.Join(manifestDir, "marketplace.json"), content: marketJSON},
	}, nil
}

// stampedVersion carries the commit as semantic-version build metadata, so
// `claude plugin list` says which MeshBench an install came from without
// anybody opening a file. The release half is bumped by hand when what the
// skills promise changes; build metadata is ignored when versions are compared,
// so it cannot make an install look newer than it is.
func stampedVersion(release, sha string) string {
	short := sha
	if len(short) > 7 {
		short = short[:7]
	}
	return release + "+g" + short
}

// marshal writes a manifest the way a person would have: indented, and ending
// in a newline, because these are files somebody will open and read.
func marshal(v any) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// entryDir is where a marketplace entry's source lands inside the rendered
// tree. Cleaning turns the root entry into ".", which path.Join then folds
// away, so the same arithmetic reads a subdirectory entry if one is ever added.
func entryDir(source string) string {
	return path.Clean(source)
}
