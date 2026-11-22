package notify

import "testing"

func TestNoticeBody(t *testing.T) {
	tests := []struct {
		notice Notice
		want   string
	}{
		{Notice{Kind: KindLimit, Minutes: 15}, "15 minutes left today."},
		{Notice{Kind: KindLimit, Minutes: 0}, "Time is up for today."},
		{Notice{Kind: KindWindow, Minutes: 10}, "The computer locks in 10 minutes."},
		{Notice{Kind: KindWindow, Minutes: 1}, "The computer locks in 1 minute."},
		{Notice{Kind: KindWindow, Minutes: 0}, "The time window is closed."},
	}
	for _, test := range tests {
		if got := test.notice.Body(); got != test.want {
			t.Errorf("%+v.Body() = %q, want %q", test.notice, got, test.want)
		}
	}
}

func TestNewNotice(t *testing.T) {
	if notice, err := NewNotice("limit", 5); err != nil || notice.Kind != KindLimit || notice.Minutes != 5 {
		t.Errorf("NewNotice(limit, 5) = %+v, %v", notice, err)
	}
	for _, test := range []struct {
		kind    string
		minutes int
	}{
		{"other", 5},
		{"limit", -1},
		{"window", 24*60 + 1},
	} {
		if _, err := NewNotice(test.kind, test.minutes); err == nil {
			t.Errorf("NewNotice(%q, %d) error = nil, want an error", test.kind, test.minutes)
		}
	}
}
