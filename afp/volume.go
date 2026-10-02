package afp

/*
The volume: the folder of the host the server shares. Opening it and what is
in it are the volume and file calls, which come in their own change; until
then every one of them is answered as not supported, which an AppleShare
client takes for a server it can log in to and not use.
*/

// fpWrite is the call that sends data, the one that comes through ASP's write
const fpWrite = 33

// volume is the shared folder
type volume struct {
	name   []uint8
	folder string
}

func newVolume(name []uint8, folder string) *volume {
	return &volume{name: name, folder: folder}
}

// openFork is a fork a session has open
type openFork struct{}

func (f *openFork) close() {}

// volumeCommand runs the calls on the volume and what is in it
func (s *Server) volumeCommand(state *sessionState, call uint8, request []uint8) ([]uint8, int32) {
	return nil, errCallNotSupported
}

// write runs FPWrite
func (s *Server) write(state *sessionState, request []uint8, data []uint8) ([]uint8, int32) {
	return nil, errCallNotSupported
}
