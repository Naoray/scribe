package adopt

// AdoptCandidateFoundMsg is emitted for each candidate discovered by FindCandidates.
type AdoptCandidateFoundMsg struct{ Candidate Candidate }

// AdoptStartedMsg is emitted when Apply begins processing a candidate.
type AdoptStartedMsg struct{ Name string }

// AdoptedMsg is emitted when a candidate is successfully adopted.
type AdoptedMsg struct {
	Name  string
	Tools []string
}

// AdoptErrorMsg is emitted when adoption fails for a single candidate.
type AdoptErrorMsg struct {
	Name string
	Err  error
}

// AdoptCompleteMsg is emitted after all candidates in Apply have been processed.
type AdoptCompleteMsg struct {
	Adopted int
	Skipped int
	Failed  int
}
