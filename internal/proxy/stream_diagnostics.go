package proxy

import "sort"

// streamFailureDiagnostics retains bounded protocol metadata only. Neither model
// text, opaque signatures, raw provider errors nor arbitrary finish reasons enter logs.
type streamFailureDiagnostics struct {
	DataEvents         int      `json:"data_events"`
	JSONEvents         int      `json:"json_events"`
	EndedBy            string   `json:"ended_by"`
	SawEOF             bool     `json:"saw_eof"`
	SawDone            bool     `json:"saw_done"`
	SawFinishReason    bool     `json:"saw_finish_reason"`
	FinishReasons      []string `json:"finish_reasons"`
	CandidateCount     int      `json:"candidate_count"`
	FinishedCandidates int      `json:"finished_candidates"`
	UnfinishedIndices  []int    `json:"unfinished_candidate_indices"`
	IndicesTruncated   bool     `json:"unfinished_indices_truncated"`
	NativeError        bool     `json:"native_error"`
	PromptBlocked      bool     `json:"prompt_blocked"`
}

// Known enum labels are useful diagnostics; unknown provider strings may contain
// arbitrary content and collapse to OTHER without changing completion behavior.
func diagnosticFinishReason(reason string) string {
	switch reason {
	case "STOP", "MAX_TOKENS", "SAFETY", "RECITATION", "LANGUAGE", "OTHER", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "MALFORMED_FUNCTION_CALL", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT", "IMAGE_OTHER", "NO_IMAGE", "UNEXPECTED_TOOL_CALL", "TOO_MANY_TOOL_CALLS", "MISSING_THOUGHT_SIGNATURE", "FINISH_REASON_UNSPECIFIED":
		return reason
	default:
		return "OTHER"
	}
}

func (completion *generationStreamCompletion) diagnostics(observation streamFailureDiagnostics) streamFailureDiagnostics {
	observation.CandidateCount = len(completion.candidates)
	observation.NativeError = completion.nativeError
	observation.PromptBlocked = completion.blocked
	observation.FinishReasons = make([]string, 0, len(completion.finishReasons))
	for reason := range completion.finishReasons {
		observation.FinishReasons = append(observation.FinishReasons, reason)
	}
	sort.Strings(observation.FinishReasons)
	observation.SawFinishReason = len(observation.FinishReasons) > 0
	observation.UnfinishedIndices = make([]int, 0)
	// Keep the lowest 16 indices deterministically without allocating an array
	// proportional to an untrusted provider's candidate count.
	for index, finished := range completion.candidates {
		if finished {
			observation.FinishedCandidates++
			continue
		}
		observation.UnfinishedIndices = append(observation.UnfinishedIndices, index)
		sort.Ints(observation.UnfinishedIndices)
		if len(observation.UnfinishedIndices) > 16 {
			observation.UnfinishedIndices = observation.UnfinishedIndices[:16]
			observation.IndicesTruncated = true
		}
	}
	return observation
}
