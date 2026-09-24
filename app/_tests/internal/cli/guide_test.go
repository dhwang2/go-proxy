package cli

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"testing"
)

// CLI.md is the guide an agent is pointed at. A command that no longer exists
// in it is worse than no documentation, because it reads as authoritative. This
// walks every gproxy invocation in the guide and resolves it against the real
// command tree.
func TestGuideOnlyNamesCommandsThatExist(t *testing.T) {
	file, err := os.Open("../../CLI.md")
	if err != nil {
		t.Skipf("guide not readable from the test overlay: %v", err)
	}
	defer file.Close()

	root := New("test", "test", strings.NewReader(""), os.Stdout, os.Stderr).Root()
	invocation := regexp.MustCompile(`^gproxy((?: [a-z][a-z0-9-]*)+)`)
	scanner := bufio.NewScanner(file)
	checked := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		match := invocation.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		words := strings.Fields(match[1])
		// Trim trailing words that are operands rather than subcommands, by
		// resolving the longest prefix cobra recognises.
		for len(words) > 0 {
			if _, _, err := root.Find(words); err == nil {
				found, _, _ := root.Find(words)
				if found != nil && strings.Join(pathOf(found), " ") == strings.Join(words, " ") {
					break
				}
			}
			words = words[:len(words)-1]
		}
		if len(words) == 0 {
			continue
		}
		cmd, _, err := root.Find(words)
		if err != nil || cmd == nil {
			t.Errorf("guide names a command that does not exist: gproxy %s", strings.Join(words, " "))
			continue
		}
		checked++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if checked < 20 {
		t.Fatalf("only %d commands checked; the guide or this matcher stopped working", checked)
	}
}

func pathOf(cmd interface{ CommandPath() string }) []string {
	parts := strings.Fields(cmd.CommandPath())
	if len(parts) > 0 && parts[0] == "gproxy" {
		return parts[1:]
	}
	return parts
}

// Renamed commands must be gone from the runnable examples, not quietly
// aliased. Prose may still name an old command when explaining the migration,
// so only fenced code blocks are checked.
func TestGuideExamplesDoNotUseRenamedCommands(t *testing.T) {
	data, err := os.ReadFile("../../CLI.md")
	if err != nil {
		t.Skipf("guide not readable from the test overlay: %v", err)
	}
	inBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inBlock = !inBlock
			continue
		}
		if !inBlock {
			continue
		}
		for _, stale := range []string{
			"gproxy routing", "protocol install", "gproxy start ", "gproxy stop ",
			"gproxy restart ", "firewall clear", "routing chain", "routing list",
		} {
			if strings.Contains(line, stale) {
				t.Errorf("guide example still uses the removed %q: %s", stale, line)
			}
		}
	}
}

// The redesign's command tree is the other authoritative list, and it drifted:
// the tree survived a rename while the migration tables below it did not. Only
// the tree block is checked, because those tables name dead commands on purpose.
func TestRedesignCommandTreeResolvesAgainstTheRealTree(t *testing.T) {
	data, err := os.ReadFile("../../../docs/plans/v0.3/cli-ux-redesign.md")
	if err != nil {
		t.Skipf("redesign not readable from the test overlay: %v", err)
	}
	root := New("test", "test", strings.NewReader(""), os.Stdout, os.Stderr).Root()
	description := regexp.MustCompile(`\s{3,}`)
	word := regexp.MustCompile(`^[a-z][a-z0-9|-]*$`)

	inTree, seenHeading, checked := false, false, 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "## Command tree") {
			seenHeading = true
			continue
		}
		if !seenHeading {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inTree {
				break
			}
			inTree = true
			continue
		}
		if !inTree {
			continue
		}
		// The description column is separated by three or more spaces; the
		// invocation itself never is.
		invocation := description.Split(strings.TrimSpace(line), 2)[0]
		fields := strings.Fields(invocation)
		if len(fields) == 0 || fields[0] != "gproxy" {
			continue
		}
		// One path per alternation: "user list|add" is two commands, and an
		// operand or a flag ends the path.
		paths := [][]string{{}}
		for _, field := range fields[1:] {
			if !word.MatchString(field) {
				break
			}
			grown := [][]string{}
			for _, alternative := range strings.Split(field, "|") {
				for _, path := range paths {
					grown = append(grown, append(append([]string{}, path...), alternative))
				}
			}
			paths = grown
		}
		for _, path := range paths {
			if len(path) == 0 {
				continue
			}
			cmd, _, findErr := root.Find(path)
			if findErr != nil || cmd == nil || strings.Join(pathOf(cmd), " ") != strings.Join(path, " ") {
				t.Errorf("the command tree names a command that does not exist: gproxy %s", strings.Join(path, " "))
				continue
			}
			checked++
		}
	}
	if checked < 40 {
		t.Fatalf("only %d commands checked; the tree block or this matcher stopped working", checked)
	}
}
