package layout

import "fmt"

// Header returns a simple "title  version" line.
func Header(title, version string) string {
	return fmt.Sprintf("  %s  %s", title, version)
}
