package amazongames

import "testing"

func TestParseAuthCode(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"landing address", "https://www.amazon.com/?openid.assoc_handle=amzn_sonic_games_launcher&openid.oa2.authorization_code=ANcodeXYZ&openid.mode=id_res", "ANcodeXYZ", false},
		{"landing address with spaces", "  https://www.amazon.com/?openid.oa2.authorization_code=AN123  \n", "AN123", false},
		{"bare code", "ANbareCODE", "ANbareCODE", false},
		{"address without a code", "https://www.amazon.com/?openid.mode=cancel", "", true},
		{"empty", "   ", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAuthCode(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("code = %q, want %q", got, tc.want)
			}
		})
	}
}
