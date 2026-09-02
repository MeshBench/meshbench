// What `/plugin install` will find, checked against what was rendered, with no
// network.
//
// A manifest fault is invisible on the machine that writes it and loud on
// somebody else's: a marketplace entry naming a directory that was never
// rendered installs a plugin with no skills in it, and a front page telling
// people to type a name the manifests do not carry sends them to a command that
// fails. Both are answerable from the rendered bytes, for the reason
// mirror_test.go gives for staying offline.
package main

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
	"testing"
)

// kebab is the shape a marketplace and a plugin name have to be in.
var kebab = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func TestEveryMirrorInstallsAsAPluginThatCarriesItsSkills(t *testing.T) {
	rendered := renderEverything(t)
	for _, m := range mirrors {
		files := rendered[m.repo]
		market, plug, dir := manifests(t, m.repo, files)

		if market.Name == "" || !kebab.MatchString(market.Name) {
			t.Errorf("%s: marketplace name %q is not kebab-case", m.repo, market.Name)
		}
		if market.Owner.Name == "" {
			t.Errorf("%s: the marketplace names no owner, which it is required to", m.repo)
		}
		if plug.Name == "" || !kebab.MatchString(plug.Name) {
			t.Errorf("%s: plugin name %q is not kebab-case", m.repo, plug.Name)
		}
		if plug.Description == "" || plug.Version == "" {
			t.Errorf("%s: the plugin manifest has no description or no version", m.repo)
		}

		// The skills are the whole point of the plugin, so an entry that
		// resolves somewhere they are not is the fault worth catching.
		for _, s := range m.skills {
			want := path.Join(dir, "skills", s.name(), "SKILL.md")
			if !rendersPath(files, want) {
				t.Errorf("%s: the marketplace offers %s at %q, but %s is not rendered there",
					m.repo, plug.Name, dir, want)
			}
		}
		for _, f := range files {
			if path.Base(f.path) != "SKILL.md" {
				continue
			}
			if !strings.HasPrefix(f.path, path.Join(dir, "skills")+"/") {
				t.Errorf("%s: %s sits outside the plugin at %q, so installing the plugin"+
					" would not install it", m.repo, f.path, dir)
			}
		}
	}
}

// The install commands are the one thing on the front page a reader copies
// character for character, so they are checked against the manifests rather
// than trusted.
func TestTheFrontPageNamesWhatTheManifestsOffer(t *testing.T) {
	rendered := renderEverything(t)
	for _, m := range mirrors {
		files := rendered[m.repo]
		market, plug, _ := manifests(t, m.repo, files)

		readme := string(mustRender(t, m.repo, files, "README.md"))
		for _, want := range []string{
			"/plugin marketplace add " + org + "/" + m.repo,
			"/plugin install " + plug.Name + "@" + market.Name,
		} {
			if !strings.Contains(readme, want) {
				t.Errorf("%s: its README never tells anybody to type %q", m.repo, want)
			}
		}

		// Plugin skills are namespaced, so a reader who was told the bare
		// directory name will type something that does not resolve.
		for _, s := range m.skills {
			want := plug.Name + ":" + s.name()
			if !strings.Contains(readme, want) {
				t.Errorf("%s: its README never says the installed skill is called %q", m.repo, want)
			}
		}
	}
}

// manifests reads both of a mirror's manifests and resolves where its one
// offered plugin lives, which is also the assertion that the JSON parses and
// that the two agree on the plugin's name.
func manifests(t *testing.T, repo string, files []file) (marketplaceManifest, pluginManifest, string) {
	t.Helper()

	var market marketplaceManifest
	unmarshalRendered(t, repo, files, path.Join(manifestDir, "marketplace.json"), &market)
	if len(market.Plugins) != 1 {
		t.Fatalf("%s: its marketplace offers %d plugins; this renders one, at the repository root",
			repo, len(market.Plugins))
	}
	entry := market.Plugins[0]
	if entry.Source == "" || entry.Description == "" {
		t.Errorf("%s: the marketplace entry for %s has no source or no description", repo, entry.Name)
	}
	dir := entryDir(entry.Source)

	var plug pluginManifest
	unmarshalRendered(t, repo, files, path.Join(dir, manifestDir, "plugin.json"), &plug)
	if entry.Name != plug.Name {
		t.Errorf("%s: the marketplace offers %q and the plugin at %q calls itself %q",
			repo, entry.Name, dir, plug.Name)
	}
	return market, plug, dir
}

func unmarshalRendered(t *testing.T, repo string, files []file, want string, into any) {
	t.Helper()
	if err := json.Unmarshal(mustRender(t, repo, files, want), into); err != nil {
		t.Fatalf("%s: %s is not valid JSON: %v", repo, want, err)
	}
}

func mustRender(t *testing.T, repo string, files []file, want string) []byte {
	t.Helper()
	for _, f := range files {
		if f.path == want {
			return f.content
		}
	}
	t.Fatalf("%s renders no %s", repo, want)
	return nil
}

func rendersPath(files []file, want string) bool {
	for _, f := range files {
		if f.path == want {
			return true
		}
	}
	return false
}
