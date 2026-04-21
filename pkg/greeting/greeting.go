package greeting

import "strings"

func Message(name string, from string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "World"
	}

	from = strings.TrimSpace(from)
	if from == "" {
		from = "bu1ld"
	}

	return "Hello, " + name + " from " + from + "!"
}
