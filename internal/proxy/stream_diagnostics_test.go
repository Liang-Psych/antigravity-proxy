package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
)

func TestStreamFailureDiagnosticsExplainCompletionWithoutLoggingContent(t *testing.T) {
	const partial = "data: {\"response\":{\"candidates\":[{\"index\":1,\"content\":{\"parts\":[{\"text\":\"body-secret\",\"thoughtSignature\":\"signature-secret\"}]}}]}}\n\n"
	const finished = "data: {\"response\":{\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}]}}\n\n"
	const unknown = "data: {\"response\":{\"candidates\":[{\"index\":0,\"finishReason\":\"private-finish-secret\"}]}}\n\n"
	for _, tc := range []struct {
		name                                     string
		reader                                   io.Reader
		category, end                            string
		events, jsonEvents, candidates, finished int
		eof, done                                bool
		reasons                                  []string
	}{
		{"empty EOF", strings.NewReader(""), "upstream_incomplete", "eof", 0, 0, 0, 0, true, false, []string{}},
		{"partial EOF", strings.NewReader(partial), "upstream_incomplete", "eof", 1, 1, 1, 0, true, false, []string{}},
		{"partial DONE", strings.NewReader(partial + "data: [DONE]\n\n"), "upstream_incomplete", "done", 2, 1, 1, 0, false, true, []string{}},
		{"one candidate unfinished", strings.NewReader(finished + partial), "upstream_incomplete", "eof", 2, 2, 2, 1, true, false, []string{"STOP"}},
		{"unknown finish is redacted", strings.NewReader(unknown + partial), "upstream_incomplete", "eof", 2, 2, 2, 1, true, false, []string{"OTHER"}},
		{"malformed provider response", strings.NewReader("data: raw-error-secret\n\n"), "upstream_invalid_response", "upstream_invalid_response", 1, 0, 0, 0, false, false, []string{}},
		{"read error after data", io.MultiReader(strings.NewReader(partial), iotest.ErrReader(errors.New("private-read-secret"))), "upstream_read_failed", "upstream_read_failed", 1, 1, 1, 0, false, false, []string{}},
		{"read timeout after data", io.MultiReader(strings.NewReader(partial), iotest.ErrReader(context.DeadlineExceeded)), "upstream_timeout", "upstream_timeout", 1, 1, 1, 0, false, false, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), accessStateKey{}, &accessRequestState{})
			response := httptest.NewRecorder()
			func() {
				defer func() {
					if recovered := recover(); recovered != nil && recovered != http.ErrAbortHandler {
						t.Errorf("unexpected panic: %v", recovered)
					}
				}()
				forwardGenerationStream(ctx, response, tc.reader)
			}()
			d := requestStreamDiagnostic(ctx)
			if d == nil {
				t.Fatal("missing failure metadata")
			}
			if requestStreamFailure(ctx) != tc.category || d.EndedBy != tc.end || d.DataEvents != tc.events || d.JSONEvents != tc.jsonEvents || d.CandidateCount != tc.candidates || d.FinishedCandidates != tc.finished || d.SawEOF != tc.eof || d.SawDone != tc.done || !reflect.DeepEqual(d.FinishReasons, tc.reasons) {
				t.Fatalf("incorrect metadata: %+v", d)
			}
			if tc.candidates > tc.finished && !reflect.DeepEqual(d.UnfinishedIndices, []int{1}) {
				t.Fatalf("unfinished candidates: %v", d.UnfinishedIndices)
			}
			encoded, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "secret") {
				t.Fatalf("provider content leaked: %s", encoded)
			}
			if strings.Contains(response.Body.String(), "stream_diagnostic") || strings.Contains(response.Body.String(), "upstream_incomplete") {
				t.Fatal("diagnostics changed response body")
			}
		})
	}
}

func TestStreamDiagnosticIndicesAreBoundedAndDeterministic(t *testing.T) {
	completion := generationStreamCompletion{candidates: make(map[int]bool), finishReasons: map[string]bool{"STOP": true}}
	for i := 100; i >= 0; i-- {
		completion.candidates[i] = i == 100
	}
	d := completion.diagnostics(streamFailureDiagnostics{DataEvents: 101, JSONEvents: 101})
	if d.CandidateCount != 101 || d.FinishedCandidates != 1 || !d.IndicesTruncated || len(d.UnfinishedIndices) != 16 || !d.SawFinishReason {
		t.Fatalf("unbounded metadata: %+v", d)
	}
	for i, index := range d.UnfinishedIndices {
		if index != i {
			t.Fatal("indices should keep the lowest sixteen deterministically")
		}
	}
}

func TestSuccessfulStreamDoesNotPublishFailureDiagnostic(t *testing.T) {
	ctx := context.WithValue(context.Background(), accessStateKey{}, &accessRequestState{})
	const body = "data: {\"response\":{\"candidates\":[{\"finishReason\":\"STOP\"}]}}\n\ndata: [DONE]\n\n"
	response := httptest.NewRecorder()
	forwardGenerationStream(ctx, response, strings.NewReader(body))
	if requestStreamDiagnostic(ctx) != nil || requestStreamFailure(ctx) != "" {
		t.Fatal("success should not publish a failure diagnostic")
	}
	if response.Body.String() != "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n" {
		t.Fatalf("native SSE changed: %q", response.Body.String())
	}
}
