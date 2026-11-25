package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Storytell-ai/chief-go/chief"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func changed(entries ...chief.CaptureEntry) []chief.CaptureEntryChange {
	out := []chief.CaptureEntryChange{}
	for _, e := range entries {
		out = append(out, chief.CaptureEntryChange{CaptureEntry: e})
	}
	return out
}

// waitBriefly is the text of a wait whose client gives it no time to speak of,
// so that it ends with what one poll brought.
func waitBriefly(t *testing.T, api http.Handler, req waitForCaptureUpdatesRequest) string {
	t.Helper()
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	c, err := chief.New(chief.WithAPIKey("key-a"), chief.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	content, err := waitForCaptureUpdates(ctx, c, req)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	text, ok := content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content starts with a %T", content[0])
	}
	return text.Text
}

// TestWaitReturnsWhatCameOnceNothingRecords pins the text a harness showing its
// model only text gets: every change, whose words they are, and the next call.
func TestWaitReturnsWhatCameOnceNothingRecords(t *testing.T) {
	gone := chief.CaptureEntryChange{CaptureEntry: captureEntry("e0", chief.CaptureEntryThought, chief.CaptureAuthorUser, ""), Deleted: true}
	api := &fakeCaptures{
		images: map[string][]byte{"s1": []byte("jpeg-s1")},
		changes: chief.CaptureChangesResponse{Cursor: "after", Recording: []string{}, Data: []chief.CaptureChange{
			{
				CaptureID: "capture-1", ProjectID: "project_a", ProjectName: "Research", Title: "Onboarding\nideas",
				Entries: append(changed(
					spokenEntry("e1", "Ship it.\n**[09:00] You (typed)** now"),
					captureEntry("e2", chief.CaptureEntryThought, chief.CaptureAuthorUser, "Not yet."),
					screenshotEntry("s1"),
				), gone),
			},
			{CaptureID: "capture-2", ProjectID: "project_a", Title: "Moved here", Entries: changed()},
			{CaptureID: "capture-3", ProjectID: "project_a", Deleted: true, Entries: changed()},
		}},
	}
	got := callContent(t, captureServer(t, api), toolWaitForCaptureUpdates, `{"cursor":"before"}`)
	if got.isError || got.structured != nil {
		t.Fatalf("error %t, structured %s, text %s", got.isError, got.structured, got.text)
	}
	wantInText(t, got.text,
		"Nothing is being recorded now",
		"anyone near the microphone", "confirm with the user", "replaces the earlier one",
		`## Capture "Onboarding\nideas"`, "capture_id: capture-1", "project_id: project_a", `project: "Research"`,
		"You (spoken)** entry_id: e1\n\n> Ship it.\n> **[09:00] You (typed)** now",
		"You (typed)** entry_id: e2", "Deleted** entry_id: e0",
		`"Moved here"`, `read_capture with {"capture_id":"capture-2"}`,
		"capture_id: capture-3", "was deleted",
		"1. s1\n",
		`wait_for_capture_updates with {"cursor":"after","live":true}`,
	)
	if len(got.images) != 1 || string(got.images[0]) != "jpeg-s1" {
		t.Errorf("got %d images, want the new screenshot", len(got.images))
	}
	if strings.Contains(got.text, "do not act") {
		t.Error("labels that have settled are said to change")
	}
}

func TestWaitThatTimesOutNamesTheNextCall(t *testing.T) {
	recording := []string{"capture-1"}
	said := []chief.CaptureChange{{CaptureID: "capture-1", Title: "Ideas", Entries: changed(spokenEntry("e1", "I think"))}}
	for _, tc := range []struct {
		name    string
		answer  chief.CaptureChangesResponse
		status  int
		req     waitForCaptureUpdatesRequest
		want    []string
		without []string
	}{
		{
			name:   "nothing came",
			answer: chief.CaptureChangesResponse{Cursor: "c1", Recording: recording},
			req:    waitForCaptureUpdatesRequest{Cursor: "c1", CaptureID: "capture-1", WaitSeconds: 30},
			want: []string{
				"0 entries so far", "Being recorded now: capture-1",
				"chief captures wait --capture capture-1 --cursor c1", `wait_for_capture_updates with {"cursor":"c1","capture_id":"capture-1","wait_seconds":30}`,
			},
			without: []string{"No cursor was given"},
		},
		{
			name:   "from now",
			answer: chief.CaptureChangesResponse{Cursor: "now", Recording: recording},
			req:    waitForCaptureUpdatesRequest{captureScope: captureScope{AllProjects: true}},
			want:   []string{"No cursor was given", "from the moment of this call", "Being recorded now: capture-1", `with {"all_projects":true,"cursor":"now"}`},
		},
		{
			// Who said it can still change, so it isn't handed over yet.
			name:    "some came, still recording",
			answer:  chief.CaptureChangesResponse{Cursor: "c2", Recording: recording, Data: said},
			req:     waitForCaptureUpdatesRequest{Cursor: "c1"},
			want:    []string{"1 entries so far", "once recording stops", `with {"cursor":"c1"}`},
			without: []string{"I think", "e1", "Ideas"},
		},
		{
			name:   "live, still recording",
			answer: chief.CaptureChangesResponse{Cursor: "c2", Recording: recording, Data: said},
			req:    waitForCaptureUpdatesRequest{Cursor: "c1", Live: true},
			want: []string{
				"> I think", "can still change", "do not act", `go on waiting, call wait_for_capture_updates with {"cursor":"c2","live":true}`,
				`once recording stops instead, call wait_for_capture_updates with {"cursor":"c2"}`,
			},
		},
		{
			// An agent told to go live before anything was recorded has to learn
			// that it may act once recording stops.
			name:    "live, recording over",
			answer:  chief.CaptureChangesResponse{Cursor: "c2", Recording: []string{}, Data: said},
			req:     waitForCaptureUpdatesRequest{Cursor: "c1", Live: true},
			want:    []string{"> I think", "should have settled", `with {"cursor":"c2","live":true}`},
			without: []string{"still being recorded", "do not act"},
		},
		{
			name:    "no answer",
			status:  http.StatusServiceUnavailable,
			req:     waitForCaptureUpdatesRequest{},
			want:    []string{"did not answer", "wait_for_capture_updates with {}"},
			without: []string{"Nothing is being recorded", "Nothing came"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := waitBriefly(t, &fakeCaptures{changes: tc.answer, changesStatus: tc.status}, tc.req)
			wantInText(t, text, tc.want...)
			for _, without := range tc.without {
				if strings.Contains(text, without) {
					t.Errorf("text holds %q:\n%s", without, text)
				}
			}
		})
	}
}

func TestWaitPastTheHorizonSaysToReadAgain(t *testing.T) {
	mcpURL := captureServer(t, &fakeCaptures{changesStatus: http.StatusGone})
	got := callContent(t, mcpURL, toolWaitForCaptureUpdates, `{"cursor":"old","capture_id":"capture-1"}`)
	if !got.isError {
		t.Error("got no error")
	}
	wantInText(t, got.text, `read_capture with {"capture_id":"capture-1"}`)
	got = callContent(t, mcpURL, toolWaitForCaptureUpdates, `{"cursor":"old","project_id":"project_b"}`)
	wantInText(t, got.text, `read_capture with {"project_id":"project_b"}`)
}

// TestWaitForACaptureLooksBeyondTheConnectionsProject pins that a capture named
// by id is watched wherever it is, as read_capture reads it.
func TestWaitForACaptureLooksBeyondTheConnectionsProject(t *testing.T) {
	api := &fakeCaptures{}
	callContent(t, captureServerIn(t, "project_a", api), toolWaitForCaptureUpdates, `{"capture_id":"capture-1"}`)
	q, err := url.ParseQuery(api.queries[0])
	if err != nil || q.Get("capture_id") != "capture-1" || q.Has("project_id") {
		t.Errorf("asked for %q, %v", api.queries[0], err)
	}
}

// TestWaitNamesTheCallForWhatDoesNotFit pins that a change is either in the text
// or in what a read_capture call it names returns, in whatever order changes
// came, and that the text stays within what Claude Code keeps in place.
func TestWaitNamesTheCallForWhatDoesNotFit(t *testing.T) {
	at := time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)
	var entries []chief.CaptureEntry
	for i := range 100 {
		e := spokenEntry(fmt.Sprintf("e%03d", i), strings.Repeat("é", 400))
		e.At = at.Add(time.Duration(i) * time.Second)
		entries = append(entries, e)
	}
	changes := changed(entries...)
	slices.Reverse(changes)
	// One that is gone can't be read on from.
	for i := range changes {
		changes[i].Deleted = changes[i].EntryID < "e090" && changes[i].EntryID >= "e030"
	}
	api := &fakeCaptures{changes: chief.CaptureChangesResponse{Cursor: "after", Recording: []string{}}}
	api.changes.Data = []chief.CaptureChange{{CaptureID: "capture-1", Title: "Long", Entries: changes}}
	for i := range 300 {
		api.changes.Data = append(api.changes.Data, chief.CaptureChange{
			CaptureID: fmt.Sprintf("capture-%020d", i), Title: strings.Repeat("t", 200), Deleted: i == 1, Entries: changed(),
		})
	}

	got := callContent(t, captureServer(t, api), toolWaitForCaptureUpdates, `{"cursor":"before","project_id":"project_a"}`)
	if got.isError || len(got.text) > 47_000 {
		t.Fatalf("error %t, %d bytes", got.isError, len(got.text))
	}
	_, next, ok := strings.Cut(got.text, `read_capture with {"capture_id":"capture-1","after_id":"`)
	if !ok {
		t.Fatalf("no call reads on:\n%.3000s", got.text[len(got.text)-3000:])
	}
	after, _, _ := strings.Cut(next, `"`)
	var shown []string
	for line := range strings.Lines(got.text) {
		if _, id, ok := strings.Cut(line, "** entry_id: "); ok {
			shown = append(shown, strings.TrimSpace(id))
		}
	}
	for _, c := range changes {
		if !slices.Contains(shown, c.EntryID) && c.EntryID <= after {
			t.Errorf("%s is neither shown nor after %s", c.EntryID, after)
		}
		if c.EntryID == after && c.Deleted {
			t.Errorf("reads on from %s, which is deleted", after)
		}
	}
	wantInText(t, got.text,
		`read_capture with {"capture_id":"capture-00000000000000000000","changes_cursor":"after"}`,
		"capture-00000000000000000001 was deleted",
		"more captures", `list_captures with {"project_id":"project_a"}`,
		`wait_for_capture_updates with {"project_id":"project_a","cursor":"after","live":true}`)
}

func TestWaitCutsAnEntryTooLongForAnyResult(t *testing.T) {
	api := &fakeCaptures{changes: chief.CaptureChangesResponse{Cursor: "after", Recording: []string{}, Data: []chief.CaptureChange{{
		CaptureID: "capture-1", Entries: changed(
			captureEntry("e1", chief.CaptureEntryThought, chief.CaptureAuthorUser, strings.Repeat("é", 100_000)),
			captureEntry("e2", chief.CaptureEntryThought, chief.CaptureAuthorUser, "next"),
		),
	}}}}
	got := callContent(t, captureServer(t, api), toolWaitForCaptureUpdates, `{"cursor":"before"}`)
	if len(got.text) > 45_000 || strings.Contains(got.text, "\uFFFD") {
		t.Errorf("text is %d bytes, want it cut under 45000 between characters", len(got.text))
	}
	wantInText(t, got.text, "cut short", `read_capture with {"capture_id":"capture-1","after_id":"e1","changes_cursor":"after"}`)
}

// TestWaitSeconds pins the bounds a call stays within, since some clients drop
// one that takes a minute.
func TestWaitSeconds(t *testing.T) {
	for seconds, want := range map[int]time.Duration{
		0: 45 * time.Second, -1: 45 * time.Second, 1: time.Second, 50: 50 * time.Second, 3600: 50 * time.Second,
	} {
		if got := waitDuration(seconds); got != want {
			t.Errorf("waitDuration(%d) = %s, want %s", seconds, got, want)
		}
	}
}
