package singbox

import (
	"testing"
)

func TestSocks_Parse_Credentials(t *testing.T) {
	tests := []struct {
		name     string
		link     string
		username string
		password string
	}{
		{
			name:     "base64 user:pass",
			link:     "socks://dXNlcjpwYXNzd29yZA==@example.com:1080#Remark",
			username: "user",
			password: "password",
		},
		{
			name:     "plain user:pass",
			link:     "socks://user:pass@example.com:1080",
			username: "user",
			password: "pass",
		},
		{
			name:     "base64 without colon",
			link:     "socks://dXNlcm5hbWU=@example.com:1080",
			username: "dXNlcm5hbWU=",
			password: "",
		},
		{
			name:     "no credentials",
			link:     "socks://example.com:1080",
			username: "",
			password: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Socks{OrigLink: tt.link}
			if err := s.Parse(); err != nil {
				t.Fatalf("Parse() failed: %v", err)
			}
			if s.Username != tt.username {
				t.Errorf("Username = %q, want %q", s.Username, tt.username)
			}
			if s.Password != tt.password {
				t.Errorf("Password = %q, want %q", s.Password, tt.password)
			}
		})
	}
}
