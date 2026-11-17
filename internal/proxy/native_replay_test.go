package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeToolRoundTripRetainsSignatureAndUnknownFields(t *testing.T) {
	fixture, err := os.ReadFile("testdata/native/tool-turn.json")
	if err != nil {
		t.Fatal(err)
	}
	// The client returns the exact content object produced by the model. No
	// signature repair, tool ID synthesis or numeric conversion is allowed.
	var turn struct {
		Candidates []struct {
			Content json.RawMessage `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(fixture, &turn); err != nil {
		t.Fatal(err)
	}
	content := string(turn.Candidates[0].Content)
	first := `{"contents":[{"role":"user","parts":[{"text":"Look up the fixture"}]}],"tools":[{"functionDeclarations":[{"name":"fixture_lookup","parametersJsonSchema":{"type":"object","properties":{"id":{"type":"integer"}}}}]}]}`
	second := `{"contents":[{"role":"user","parts":[{"text":"Look up the fixture"}]},` + content + `,{"role":"user","parts":[{"functionResponse":{"name":"fixture_lookup","response":{"id":9007199254740993123,"result":"fixture"}}}]}],"futureRequest":1.000e+40}`
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope struct {
			Request json.RawMessage `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
		}
		want := first
		if calls.Add(1) == 2 {
			want = second
		}
		if string(envelope.Request) != want {
			t.Error("multi-turn request changed bytes")
		}
		_, _ = io.WriteString(w, `{"response":`+string(fixture)+`}`)
	}))
	defer upstream.Close()
	p := generationTestProxy(upstream.URL)
	for _, body := range []string{first, second} {
		response := httptest.NewRecorder()
		p.ServeHTTP(response, httptest.NewRequest("POST", "/v1beta/models/fixture:generateContent", strings.NewReader(body)))
		if response.Code != 200 || strings.TrimSpace(response.Body.String()) != strings.TrimSpace(string(fixture)) {
			t.Fatal("native response changed")
		}
	}
	if calls.Load() != 2 {
		t.Fatal("missing tool continuation")
	}
}

func TestNativeMediaFixtureAdaptsOnlySchemaKey(t *testing.T) {
	fixture, err := os.ReadFile("testdata/native/media-request.json")
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Replace(string(fixture), `"responseJsonSchema"`, `"responseSchema"`, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var envelope struct {
			Request json.RawMessage `json:"request"`
		}
		if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
			t.Error(err)
		}
		// Comparing decoded Number values checks literal precision as well as shape.
		a, _ := json.Marshal(decodeGenerationJSON(t, envelope.Request))
		b, _ := json.Marshal(decodeGenerationJSON(t, []byte(expected)))
		if string(a) != string(b) {
			t.Error("media, metadata or future fields changed")
		}
		_, _ = io.WriteString(w, `{"response":{"candidates":[{"content":{"parts":[{"text":"{\"count\":1}"}]},"finishReason":"STOP"}]}}`)
	}))
	defer upstream.Close()
	p := generationTestProxy(upstream.URL)
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("POST", "/v1beta/models/fixture:generateContent", strings.NewReader(string(fixture))))
	if w.Code != 200 {
		t.Fatalf("media request: %d", w.Code)
	}
}
