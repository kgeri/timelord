package session

import "testing"

func TestWTSSessionFromData(t *testing.T) {
	tests := []struct {
		name       string
		data       wtsSessionData
		wantActive bool
		wantLocked bool
	}{
		{
			name: "active locked session",
			data: wtsSessionData{
				ID: 7, User: "dani", Seat: "Console", State: wtsStateActive,
				Flags: wtsSessionStateLock, FlagsKnown: true,
			},
			wantActive: true, wantLocked: true,
		},
		{
			name: "active unlocked session",
			data: wtsSessionData{
				ID: 7, User: "dani", Seat: "Console", State: wtsStateActive,
				Flags: wtsSessionStateUnlock, FlagsKnown: true,
			},
			wantActive: true, wantLocked: false,
		},
		{
			name: "inactive session ignores lock flag",
			data: wtsSessionData{
				ID: 7, User: "dani", Seat: "Console", State: 4,
				Flags: wtsSessionStateLock, FlagsKnown: true,
			},
			wantActive: false, wantLocked: false,
		},
		{
			name: "unknown lock state is not locked",
			data: wtsSessionData{
				ID: 7, User: "dani", Seat: "Console", State: wtsStateActive,
				Flags: wtsSessionStateLock, FlagsKnown: false,
			},
			wantActive: true, wantLocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wtsSessionFromData(tt.data)
			if got.ID != "7" || got.User != tt.data.User || got.Seat != tt.data.Seat {
				t.Errorf("wtsSessionFromData() = %+v", got)
			}
			if got.Active != tt.wantActive || got.Locked != tt.wantLocked {
				t.Errorf("active = %v locked = %v, want %v %v", got.Active, got.Locked, tt.wantActive, tt.wantLocked)
			}
			if got.Remote {
				t.Errorf("remote = true, want false for Windows sessions")
			}
		})
	}
}
