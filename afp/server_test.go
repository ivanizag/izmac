package afp

import (
	"encoding/binary"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *Server {
	s := NewServer("Ivan’s Mac", "Shared: stuff", t.TempDir(), nil)
	s.OpenSession(1)
	return s
}

func loginRequest(version, uam string) []uint8 {
	request := []uint8{fpLogin}
	request = appendPascal(request, []uint8(version))
	return appendPascal(request, []uint8(uam))
}

// pascalAt is the Pascal string at an offset of a block
func pascalAt(t *testing.T, block []uint8, at int) string {
	t.Helper()
	if at >= len(block) || at+1+int(block[at]) > len(block) {
		t.Fatalf("no string at %v of a block of %v bytes", at, len(block))
	}
	return string(block[at+1 : at+1+int(block[at])])
}

func TestTheStatusNamesTheServerAndHowToLogIn(t *testing.T) {
	s := newTestServer(t)
	status := s.Status()

	// The name comes right after the offsets and the flags, in Mac OS
	// Roman: the curly apostrophe is $d5
	if name := pascalAt(t, status, 10); name != "Ivan\xd5s Mac" {
		t.Errorf("the server is called %q", name)
	}

	machine := int(binary.BigEndian.Uint16(status[0:]))
	if machine%2 != 0 || pascalAt(t, status, machine) != machineType {
		t.Errorf("the machine type at %v is wrong", machine)
	}

	versions := int(binary.BigEndian.Uint16(status[2:]))
	if status[versions] != 2 || pascalAt(t, status, versions+1) != version11 ||
		pascalAt(t, status, versions+2+len(version11)) != version20 {
		t.Errorf("the versions offered are wrong")
	}

	uams := int(binary.BigEndian.Uint16(status[4:]))
	if status[uams] != 1 || pascalAt(t, status, uams+1) != uamGuest {
		t.Errorf("the ways to log in are wrong")
	}
}

func TestNothingRunsBeforeLoggingIn(t *testing.T) {
	s := newTestServer(t)
	if _, result := s.Command(1, []uint8{fpGetSrvrParms}); result != errAccessDenied {
		t.Errorf("FPGetSrvrParms before logging in gave %v", result)
	}
	if _, result := s.Command(1, []uint8{fpGetSrvrInfo}); result != errNoErr {
		t.Errorf("FPGetSrvrInfo before logging in gave %v", result)
	}
}

func TestAGuestLogsIn(t *testing.T) {
	cases := []struct {
		version, uam string
		result       int32
	}{
		{version20, uamGuest, errNoErr},
		{version11, "no user authent", errNoErr},
		{"AFPVersion 2.1", uamGuest, errBadVersNum},
		{version20, "Cleartxt Passwrd", errBadUAM},
	}
	for _, c := range cases {
		s := newTestServer(t)
		if _, result := s.Command(1, loginRequest(c.version, c.uam)); result != c.result {
			t.Errorf("logging in with %q and %q gave %v, wanted %v", c.version, c.uam, result, c.result)
		}
	}
}

func TestTheServerHasOneVolume(t *testing.T) {
	s := newTestServer(t)
	s.Command(1, loginRequest(version20, uamGuest))

	reply, result := s.Command(1, []uint8{fpGetSrvrParms})
	if result != errNoErr {
		t.Fatalf("FPGetSrvrParms gave %v", result)
	}
	when := fromAFPTime(binary.BigEndian.Uint32(reply))
	if d := time.Since(when); d < -time.Second || d > 5*time.Second {
		t.Errorf("the server's time is %v", when)
	}
	// A colon cannot be in a name, it separates folders
	if reply[4] != 1 || reply[5] != 0 || pascalAt(t, reply, 6) != "Shared- stuff" {
		t.Errorf("the volumes are %x", reply[4:])
	}
}

func TestLoggingOutEndsWhatCanBeDone(t *testing.T) {
	s := newTestServer(t)
	s.Command(1, loginRequest(version20, uamGuest))
	s.Command(1, []uint8{fpLogout})
	if _, result := s.Command(1, []uint8{fpGetSrvrParms}); result != errAccessDenied {
		t.Errorf("FPGetSrvrParms after logging out gave %v", result)
	}
}
