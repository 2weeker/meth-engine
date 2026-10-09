package themes

type Theme struct{ Name, Label string }

var All = []Theme{
	{"angelic", "Angelic"}, {"blame", "BLAME!"}, {"coffee", "Coffee"}, {"cyb", "Cyb"},
	{"macos", "MacOS"}, {"meth", "Meth"}, {"ratwires", "Ratwires"},
}

const Fallback = "coffee"

var renamed = map[string]string{"bm": "meth", "main": "ratwires"}

func Lookup(name string) (string, bool) {
	if n, ok := renamed[name]; ok {
		name = n
	}
	for _, t := range All {
		if t.Name == name {
			return name, true
		}
	}
	return "", false
}

func Names() []string {
	out := make([]string, len(All))
	for i, t := range All {
		out[i] = t.Name
	}
	return out
}

func Resolve(value, fallback string) string {
	if name, ok := Lookup(value); ok {
		return name
	}
	if name, ok := Lookup(fallback); ok {
		return name
	}
	return Fallback
}
