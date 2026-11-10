package policy

import "testing"

func TestParseClock(t *testing.T) {
	tests := map[string]struct {
		want Clock
		ok   bool
	}{
		"00:00": {want: 0, ok: true},
		"08:05": {want: 8*60 + 5, ok: true},
		"23:59": {want: 23*60 + 59, ok: true},
		"24:00": {ok: false},
		"7:00":  {ok: false},
		"08:60": {ok: false},
		"+8:00": {ok: false},
	}

	for input, test := range tests {
		t.Run(input, func(t *testing.T) {
			got, err := ParseClock(input)
			if test.ok && err != nil {
				t.Fatalf("ParseClock() error = %v", err)
			}
			if !test.ok {
				if err == nil {
					t.Fatal("ParseClock() error = nil, want error")
				}
				return
			}
			if got != test.want {
				t.Errorf("ParseClock() = %v, want %v", got, test.want)
			}
		})
	}
}
