package cmd

import "testing"

func TestGuardBind(t *testing.T) {
	cases := []struct {
		addr     string
		authOn   bool
		override bool
		wantErr  bool
	}{
		{"0.0.0.0", false, false, true},
		{"0.0.0.0", true, false, false},
		{"0.0.0.0", false, true, false},
		{"0.0.0.0:7373", false, false, true},

		{"192.168.1.10", false, false, true},
		{"192.168.1.10", true, false, false},
		{"192.168.1.10", false, true, false},

		{"[::]", false, false, true},
		{"[::]", true, false, false},

		{"127.0.0.1", false, false, false},
		{"127.0.0.1", true, false, false},
		{"127.0.0.1:7373", false, false, false},

		{"localhost", false, false, false},
		{"LOCALHOST", false, false, false},

		{"::1", false, false, false},

		{"unix:///var/run/vessel.sock", false, false, false},
		{"/var/run/vessel.sock", false, false, false},
	}

	for _, tc := range cases {
		err := GuardBind(tc.addr, tc.authOn, tc.override)
		if (err != nil) != tc.wantErr {
			t.Errorf("GuardBind(%q, auth=%v, override=%v) = %v, wantErr=%v",
				tc.addr, tc.authOn, tc.override, err, tc.wantErr)
		}
		if err != nil {
			var ge *GuardBindError
			if !asGuardBindError(err, &ge) {
				t.Errorf("GuardBind(%q) error is not *GuardBindError: %T", tc.addr, err)
			} else if ge.ExitCode() != 3 {
				t.Errorf("GuardBind(%q) exit code = %d, want 3", tc.addr, ge.ExitCode())
			}
		}
	}
}

func asGuardBindError(err error, target **GuardBindError) bool {
	if ge, ok := err.(*GuardBindError); ok {
		*target = ge
		return true
	}
	return false
}
