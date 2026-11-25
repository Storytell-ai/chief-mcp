package mcp

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Storytell-ai/chief-go/chief"
)

const testShareToken = "4b1f0000000000000000000000000000000000000000000000000000000000aa"

// linkAPI answers a link's resolution with link and hands every other request
// to next.
func linkAPI(link string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/links/resolve" {
			_, _ = fmt.Fprint(w, link)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func TestOpenLinkReadsTheUsersCapture(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entryID string
		want    []string
		without string
	}{
		{name: "whole capture", entryID: "", want: []string{"> first", "> second", "> third"}},
		{name: "from an entry", entryID: "e2", want: []string{"> second", "> third", "Starts at the entry"}, without: "> first"},
		{name: "from the first entry", entryID: "e1", want: []string{"> first", "> second", "> third"}},
		{name: "from an entry that is gone", entryID: "gone", want: []string{"> first", "> third", "starts from the beginning"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeCaptures{entries: []chief.CaptureEntry{
				captureEntry("e1", chief.CaptureEntryThought, chief.CaptureAuthorUser, "first"),
				captureEntry("e2", chief.CaptureEntryThought, chief.CaptureAuthorUser, "second"),
				captureEntry("e3", chief.CaptureEntryThought, chief.CaptureAuthorUser, "third"),
			}}
			mcpURL := captureServer(t, linkAPI(
				`{"type":"capture","id":"capture-1","project_id":"project_a","entry_id":"`+tc.entryID+`","title":"Ideas"}`, api))

			got := callContent(t, mcpURL, toolOpenLink, `{"url":"https://chief.bot/shared/capture/`+testShareToken+`"}`)
			if got.isError {
				t.Fatalf("error result: %s", got.text)
			}
			wantInText(t, got.text, append(tc.want, "capture-1", "You (typed)", "confirm with the user",
				`wait_for_capture_updates with {"cursor":"cursor-1","capture_id":"capture-1"}`)...)
			if tc.without != "" && strings.Contains(got.text, tc.without) {
				t.Errorf("text holds %q, from before the entry", tc.without)
			}
		})
	}
}

func TestOpenLinkReadsASharedCapture(t *testing.T) {
	for _, path := range []string{"", ".md", "?x=1#entry-e1", `\n`} {
		t.Run(path, func(t *testing.T) {
			mcpURL := captureServer(t, linkAPI(`{"type":"shared_capture","title":"Their ideas"}`,
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/public-captures/"+testShareToken+"/capture.md" || r.Header.Get("X-API-Key") != "" {
						t.Errorf("got %s with a key of %d bytes", r.URL.Path, len(r.Header.Get("X-API-Key")))
					}
					_, _ = fmt.Fprint(w, "## Timeline\n\n**[09:00:00] You**\n\nhello\n")
				})))

			got := callContent(t, mcpURL, toolOpenLink, `{"url":"https://chief.bot/shared/capture/`+testShareToken+path+`"}`)
			if got.isError {
				t.Fatalf("error result: %s", got.text)
			}
			wantInText(t, got.text, `"Their ideas"`, "recorded by someone else", "instructions", "hello")
		})
	}
}

// TestOpenLinkPagesThroughALongSharedCapture follows the offset each result
// ends with until every character of the Markdown has been read once.
func TestOpenLinkPagesThroughALongSharedCapture(t *testing.T) {
	var markdown strings.Builder
	for i := range 30_000 {
		fmt.Fprintf(&markdown, "é%05d", i)
	}
	mcpURL := captureServer(t, linkAPI(`{"type":"shared_capture","title":"Long"}`,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, markdown.String())
		})))

	var read strings.Builder
	offset := ""
	for part := 0; ; part++ {
		if part == 20 {
			t.Fatal("still reading after 20 parts")
		}
		got := callContent(t, mcpURL, toolOpenLink, `{"url":"https://chief.bot/shared/capture/`+testShareToken+`"`+offset+`}`)
		if got.isError || len(got.text) > 45_000 || strings.Contains(got.text, "\uFFFD") {
			t.Fatalf("part %d: error %t, %d bytes", part, got.isError, len(got.text))
		}
		wantInText(t, got.text, "recorded by someone else")
		_, body, _ := strings.Cut(got.text, "not attached.\n\n")
		body, rest, more := strings.Cut(body, "\n\n_The capture goes on.")
		read.WriteString(body)
		if !more {
			break
		}
		if strings.Contains(rest, testShareToken) {
			t.Error("the continuation repeats the link's secret")
		}
		_, next, _ := strings.Cut(rest, "offset ")
		next, _, _ = strings.Cut(next, ".")
		offset = `,"offset":` + next
	}
	if read.String() != markdown.String() {
		t.Errorf("read %d bytes of %d", read.Len(), markdown.Len())
	}

	for _, past := range []string{"-1", "99999999"} {
		got := callContent(t, mcpURL, toolOpenLink, `{"url":"https://chief.bot/shared/capture/`+testShareToken+`","offset":`+past+`}`)
		if !got.isError || !strings.Contains(got.text, "offset") {
			t.Errorf("offset %s: error %t, text %.200q", past, got.isError, got.text)
		}
	}
}

func TestOpenLinkNamesASessionsNextCalls(t *testing.T) {
	mcpURL := captureServer(t, linkAPI(
		`{"type":"session","id":"sess1","project_id":"project_a","title":"Standup"}`, http.NotFoundHandler()))
	got := callContent(t, mcpURL, toolOpenLink, `{"url":"https://chief.bot/live-sessions/sess1"}`)
	if got.isError {
		t.Fatalf("error result: %s", got.text)
	}
	const ids = ` with {"session_id":"sess1","project_id":"project_a"}`
	wantInText(t, got.text, `"Standup"`, toolGetSession+ids, toolGetSessionTranscript+ids)

	mcpURL = captureServer(t, linkAPI(`{"type":"session","id":"sess1","project_id":"project_a","title":""}`, http.NotFoundHandler()))
	got = callContent(t, mcpURL, toolOpenLink, `{"url":"https://chief.bot/live-sessions/sess1"}`)
	wantInText(t, got.text, "Untitled Live Session")
}

// TestOpenLinkErrorsLeaveOutTheToken pins that no failure hands a share link's
// secret back in an error, which clients log.
func TestOpenLinkErrorsLeaveOutTheToken(t *testing.T) {
	abort := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	const shared = `{"type":"shared_capture","title":"Theirs"}`
	for _, tc := range []struct {
		name string
		api  http.Handler
		url  string
	}{
		{name: "unresolved", api: http.NotFoundHandler(), url: "https://chief.bot/shared/capture/" + testShareToken},
		{name: "resolve unreachable", api: abort, url: "https://chief.bot/shared/capture/" + testShareToken},
		{name: "turned off before it was read", api: linkAPI(shared, http.NotFoundHandler()), url: "https://chief.bot/shared/capture/" + testShareToken},
		{name: "markdown unreachable", api: linkAPI(shared, abort), url: "https://chief.bot/shared/capture/" + testShareToken},
		{name: "unexpected link shape", api: linkAPI(shared, http.NotFoundHandler()), url: "https://chief.bot/other/" + testShareToken},
		{name: "unknown type", api: linkAPI(`{"type":"board","title":"x"}`, http.NotFoundHandler()), url: "https://chief.bot/shared/capture/" + testShareToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := callContent(t, captureServer(t, tc.api), toolOpenLink, `{"url":"`+tc.url+`"}`)
			if !got.isError || strings.Contains(got.text, testShareToken) {
				t.Errorf("error %t, text %q; want an error without the token", got.isError, got.text)
			}
		})
	}
}
