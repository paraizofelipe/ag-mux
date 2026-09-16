package ui

import "testing"

func TestStateRoundTrip(t *testing.T) {
	in := persisted{selected: "%31", pinned: map[string]bool{"%30": true, "%46": true}}
	got := decodeState(encodeState(in))
	if got.selected != in.selected {
		t.Errorf("selected = %q, want %q", got.selected, in.selected)
	}
	for id := range in.pinned {
		if !got.pinned[id] {
			t.Errorf("pin %q was lost", id)
		}
	}
	if len(got.pinned) != len(in.pinned) {
		t.Errorf("pinned = %v, want %v", got.pinned, in.pinned)
	}
}

func TestStateDropsUnpinned(t *testing.T) {
	// A pin toggled off must not come back on the next start.
	s := encodeState(persisted{selected: "%1", pinned: map[string]bool{"%2": false}})
	if got := decodeState(s); len(got.pinned) != 0 {
		t.Errorf("pinned = %v, want empty", got.pinned)
	}
}

func TestStateHandlesGarbage(t *testing.T) {
	// The option is user-visible, so it can be anything.
	for _, in := range []string{"", "|", "%1", "||,,", "  %1  |  %2 , %3 "} {
		got := decodeState(in)
		if got.pinned == nil {
			t.Errorf("decodeState(%q) left a nil map", in)
		}
	}
	if got := decodeState("  %1  |  %2 , %3 "); got.selected != "%1" || !got.pinned["%2"] || !got.pinned["%3"] {
		t.Errorf("decodeState trimmed badly: %+v", got)
	}
}
