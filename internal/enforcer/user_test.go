package enforcer

import "testing"

func TestParseUser(t *testing.T) {
	tests := []struct {
		value  string
		local  string
		server string
		ok     bool
	}{
		{value: "dani", local: "dani", server: "dani", ok: true},
		{value: "dani=dani", local: "dani", server: "dani", ok: true},
		{value: "kissd=dani", local: "kissd", server: "dani", ok: true},
		{value: " kissd = dani ", local: "kissd", server: "dani", ok: true},
		{value: "", ok: false},
		{value: "=dani", ok: false},
		{value: "kissd=", ok: false},
		{value: "a=b=c", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			user, err := ParseUser(tt.value)
			if (err == nil) != tt.ok {
				t.Fatalf("ParseUser() error = %v, want ok=%v", err, tt.ok)
			}
			if !tt.ok {
				return
			}
			if user.Local != tt.local || user.Server != tt.server {
				t.Errorf("ParseUser() = %+v, want %s=%s", user, tt.local, tt.server)
			}
		})
	}
}

func TestUserString(t *testing.T) {
	if got := (User{Local: "dani", Server: "dani"}).String(); got != "dani" {
		t.Errorf("String() = %q, want dani", got)
	}
	if got := (User{Local: "kissd", Server: "dani"}).String(); got != "kissd=dani" {
		t.Errorf("String() = %q, want kissd=dani", got)
	}
}
