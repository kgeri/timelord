package process

import "testing"

func TestWindowsScope(t *testing.T) {
	systemPaths := []string{
		`c:\windows\`,
		`c:\programdata\microsoft\windows defender\`,
		`c:\program files\windowsapps\microsoftwindows.client.`,
		`c:\program files\windowsapps\microsoft.widgetsplatformruntime`,
	}

	tests := []struct {
		name        string
		sessionID   uint32
		executable  string
		systemOwner bool
		want        Scope
	}{
		{"session 0 service", 0, `C:\Program Files\Vendor\agent.exe`, false, ScopeSystem},
		{"interactive app", 1, `C:\Program Files\Google\Chrome\chrome.exe`, false, ScopeApp},
		{"app in user profile", 5, `C:\Users\kid\AppData\Local\Programs\app.exe`, false, ScopeApp},
		{"system account in a session", 5, `C:\Program Files\Vendor\agent.exe`, true, ScopeSystem},
		{"windows component", 5, `C:\Windows\System32\svchost.exe`, false, ScopeSystem},
		{"windows component is case-insensitive", 5, `c:\WINDOWS\System32\svchost.exe`, false, ScopeSystem},
		{"windows shell app", 5, `C:\Windows\SystemApps\Microsoft.Windows.StartMenuExperienceHost_cw5n1h2txyewy\StartMenuExperienceHost.exe`, false, ScopeSystem},
		{"windows defender helper", 5, `C:\ProgramData\Microsoft\Windows Defender\Platform\4.18.26080.4-0\DefenderSessionHelper.exe`, false, ScopeSystem},
		{"windows inbox package", 5, `C:\Program Files\WindowsApps\MicrosoftWindows.Client.WebExperience_526.21100.40.0_x64__cw5n1h2txyewy\WidgetBoard.exe`, false, ScopeSystem},
		{"windows inbox package prefix", 5, `C:\Program Files\WindowsApps\Microsoft.WidgetsPlatformRuntime_1.6.19.0_x64__8wekyb3d8bbwe\WidgetService\WidgetService.exe`, false, ScopeSystem},
		{"store app is not an inbox package", 5, `C:\Program Files\WindowsApps\Microsoft.MinecraftUWP_1.21.0.0_x64__8wekyb3d8bbwe\Minecraft.Windows.exe`, false, ScopeApp},
		{"lookalike directory is not windows", 5, `C:\WindowsApps\Microsoft.Minecraft\minecraft.exe`, false, ScopeApp},
		{"empty executable is not a system path", 5, "", false, ScopeApp},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := windowsScope(tt.sessionID, tt.executable, tt.systemOwner, systemPaths)
			if got != tt.want {
				t.Errorf("windowsScope(%d, %q, %v) = %q, want %q",
					tt.sessionID, tt.executable, tt.systemOwner, got, tt.want)
			}
		})
	}
}

func TestHasAnyPrefix(t *testing.T) {
	prefixes := []string{`C:\Windows\`, "", `C:\Program Files\WindowsApps\MicrosoftWindows.Client.`}

	tests := []struct {
		path string
		want bool
	}{
		{`C:\Windows\System32\cmd.exe`, true},
		{`c:\windows\system32\cmd.exe`, true},
		{`C:\Windows`, false},
		{`C:\WindowsApps\app.exe`, false},
		{`C:\Program Files\WindowsApps\MicrosoftWindows.Client.WebExperience_1.0\Widgets.exe`, true},
		{`C:\Program Files\WindowsApps\Microsoft.MinecraftUWP_1.0\Minecraft.exe`, false},
		{`C:\Users\kid\app.exe`, false},
		{``, false},
	}

	for _, tt := range tests {
		if got := hasAnyPrefix(tt.path, prefixes); got != tt.want {
			t.Errorf("hasAnyPrefix(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
