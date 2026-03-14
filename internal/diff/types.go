package diff

type LineKind int

const (
	KindContext LineKind = iota
	KindAdded
	KindRemoved
)

type Line struct {
	Number  int
	Kind    LineKind
	Content string
}

type Hunk struct {
	OldStartLine int
	OldLineCount int
	NewStartLine int
	NewLineCount int
	Lines        []Line
}

type File struct {
	Path  string
	Hunks []Hunk
}
