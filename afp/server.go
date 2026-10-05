// Package afp is an AppleShare file server, AFP 2.0, serving a folder of the
// host over the sessions of the appletalk package, knowing nothing of the
// emulator.
package afp

import (
	"encoding/binary"
	"strings"
	"time"
)

/*
The AppleTalk Filing Protocol, version 2.0, the one AppleShare 2.0 speaks on
System 6 and the System 7 client speaks to a server that offers no more. A
call is a block whose first byte is its number; the answer is a result code
and a reply block. This is the server's side of it, as Inside AppleTalk,
second edition, and the AFP 2.x reference describe it.

What is served is one folder of the host, as one volume. Who connects logs in
as a guest: there are no users and no passwords, and the volume is open to
everyone who can see the server.
*/

// The calls, by number
const (
	fpGetSrvrInfo  = 15
	fpGetSrvrParms = 16
	fpLogin        = 18
	fpLogout       = 20
)

// The result codes, from -5000 down as AFP numbers them
const (
	errNoErr            int32 = 0
	errAccessDenied     int32 = -5000
	errBadUAM           int32 = -5002
	errBadVersNum       int32 = -5003
	errBitmapErr        int32 = -5004
	errCantMove         int32 = -5005
	errDenyConflict     int32 = -5006
	errDirNotEmpty      int32 = -5007
	errDiskFull         int32 = -5008
	errEOFErr           int32 = -5009
	errFileBusy         int32 = -5010
	errItemNotFound     int32 = -5012
	errMiscErr          int32 = -5014
	errObjectExists     int32 = -5017
	errObjectNotFound   int32 = -5018
	errParamErr         int32 = -5019
	errCallNotSupported int32 = -5024
	errObjectTypeErr    int32 = -5025
	errCantRename       int32 = -5028
	errDirNotFound      int32 = -5029
	errVolLocked        int32 = -5031
	errObjectLocked     int32 = -5032
)

const (
	// The versions offered, and the one way to log in
	version11 = "AFPVersion 1.1"
	version20 = "AFPVersion 2.0"
	uamGuest  = "No User Authent"

	// machineType is what the server says it runs on
	machineType = "izmac"

	// The longest names: a server's is an NBP object, a volume's that of
	// an HFS volume
	longestServerName = 31
	longestVolumeName = 27
)

// Server is the file server, the Server of an appletalk Listener
type Server struct {
	name   []uint8
	volume *volume

	// sessions holds what each session has logged in and opened
	sessions map[int]*sessionState
}

// sessionState is what a session has done
type sessionState struct {
	loggedIn bool
}

/*
NewServer serves a folder of the host as a volume, under the names given,
which the machine gets in Mac OS Roman. Its time, the one it answers with
and the one it dates what the machine changes with, is what now says, or the
host's when it is nil; now is called only while the server handles a call.
*/
func NewServer(name string, volumeName string, folder string, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	return &Server{
		name:     macName(name, longestServerName),
		volume:   newVolume(macName(volumeName, longestVolumeName), folder, now),
		sessions: make(map[int]*sessionState),
	}
}

// Name is the server's name, in Mac OS Roman
func (s *Server) Name() []uint8 {
	return s.name
}

/*
Status is the server information, what FPGetSrvrInfo answers and ASP's
GetStatus carries: four offsets into the block and the flags, then the
server's name, and at those offsets the machine type, the versions offered and
the ways to log in. No icon: the Chooser draws the AppleShare one.
*/
func (s *Server) Status() []uint8 {
	block := make([]uint8, 10)
	block = appendPascal(block, s.name)
	block = alignEven(block)

	binary.BigEndian.PutUint16(block[0:], uint16(len(block)))
	block = appendPascal(block, []uint8(machineType))

	binary.BigEndian.PutUint16(block[2:], uint16(len(block)))
	block = append(block, 2)
	block = appendPascal(block, []uint8(version11))
	block = appendPascal(block, []uint8(version20))

	binary.BigEndian.PutUint16(block[4:], uint16(len(block)))
	block = append(block, 1)
	block = appendPascal(block, []uint8(uamGuest))

	// No volume icon, and no flags: no FPCopyFile, no changing passwords
	return block
}

// OpenSession and CloseSession keep track of the sessions
func (s *Server) OpenSession(session int) {
	s.sessions[session] = &sessionState{}
}

// CloseSession closes the forks a session left open, which writes back what
// was written to them
func (s *Server) CloseSession(session int) {
	s.volume.closeForks(session)
	delete(s.sessions, session)
}

// Command runs a call
func (s *Server) Command(session int, request []uint8) ([]uint8, int32) {
	state, ok := s.sessions[session]
	if !ok || len(request) < 1 {
		return nil, errParamErr
	}

	call := request[0]
	if call == fpLogin {
		return s.login(state, request)
	}
	if call == fpGetSrvrInfo {
		return s.Status(), errNoErr
	}
	if !state.loggedIn {
		return nil, errAccessDenied
	}

	switch call {
	case fpLogout:
		state.loggedIn = false
		return nil, errNoErr
	case fpGetSrvrParms:
		return s.serverParms(), errNoErr
	}
	return s.volumeCommand(state, session, call, request)
}

// Write runs a call that came with data
func (s *Server) Write(session int, request []uint8, data []uint8) ([]uint8, int32) {
	state, ok := s.sessions[session]
	if !ok || len(request) < 1 {
		return nil, errParamErr
	}
	if !state.loggedIn {
		return nil, errAccessDenied
	}
	return s.write(state, session, request, data)
}

/*
login takes a guest in: the version, which has to be one offered, and the way
to log in, which has to be no authentication
*/
func (s *Server) login(state *sessionState, request []uint8) ([]uint8, int32) {
	r := reader{data: request[1:]}
	version := string(r.pascal())
	uam := string(r.pascal())
	if r.failed {
		return nil, errParamErr
	}
	if version != version11 && version != version20 {
		return nil, errBadVersNum
	}
	if !strings.EqualFold(uam, uamGuest) {
		return nil, errBadUAM
	}
	state.loggedIn = true
	return nil, errNoErr
}

/*
serverParms is what FPGetSrvrParms answers: the time on the server and the
volumes, each with its flags, no password and no Apple II configuration, and
its name
*/
func (s *Server) serverParms() []uint8 {
	reply := binary.BigEndian.AppendUint32(nil, afpTime(s.volume.now()))
	reply = append(reply, 1, 0)
	return appendPascal(reply, s.volume.name)
}
