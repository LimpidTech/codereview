package review

type Verdict string

const (
	VerdictApprove        Verdict = "APPROVE"
	VerdictRequestChanges Verdict = "REQUEST_CHANGES"
	VerdictComment        Verdict = "COMMENT"
)

type Label string

const (
	LabelNit        Label = "nit"
	LabelSuggestion Label = "suggestion"
	LabelIssue      Label = "issue"
	LabelQuestion   Label = "question"
	LabelThought    Label = "thought"
	LabelChore      Label = "chore"
	LabelPraise     Label = "praise"
)

var validLabels = map[Label]bool{
	LabelNit:        true,
	LabelSuggestion: true,
	LabelIssue:      true,
	LabelQuestion:   true,
	LabelThought:    true,
	LabelChore:      true,
	LabelPraise:     true,
}

func IsValidLabel(l Label) bool {
	return validLabels[l]
}

type Comment struct {
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Label Label  `json:"label"`
	Body  string `json:"body"`
}

type Result struct {
	Verdict  Verdict   `json:"verdict"`
	Summary  string    `json:"summary"`
	Comments []Comment `json:"comments"`
}
