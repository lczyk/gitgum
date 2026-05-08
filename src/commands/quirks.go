package commands

import (
	"fmt"
	"os"
	"strings"
)

var knownQuirks = map[string]bool{
	"normal-branches": true,
}

func parseQuirks() (map[string]bool, error) {
	raw := os.Getenv("GG_QUIRKS")
	if raw == "" {
		return nil, nil
	}
	active := make(map[string]bool)
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		neg := strings.HasPrefix(tok, "-")
		name := strings.TrimPrefix(tok, "-")
		if !knownQuirks[name] {
			known := make([]string, 0, len(knownQuirks))
			for k := range knownQuirks {
				known = append(known, k)
			}
			return nil, fmt.Errorf("unknown quirk %q (known: %s)", name, strings.Join(known, ", "))
		}
		if neg {
			delete(active, name)
		} else {
			active[name] = true
		}
	}
	return active, nil
}

func quirkEnabled(name string) bool {
	q, err := parseQuirks()
	if err != nil {
		return false
	}
	return q[name]
}
