package afp

/*
The calls on the volume and what is in it, by number, as AFP 2.0 has them. The
ones of AFP 2.1 and later, file IDs, exchanging files and searching the
catalog, are not offered: a server of version 2.0 is not asked for them.
*/
const (
	fpByteRangeLock   = 1
	fpCloseVol        = 2
	fpCloseDir        = 3
	fpCloseFork       = 4
	fpCopyFile        = 5
	fpCreateDir       = 6
	fpCreateFile      = 7
	fpDelete          = 8
	fpEnumerate       = 9
	fpFlush           = 10
	fpFlushFork       = 11
	fpGetForkParms    = 14
	fpGetVolParms     = 17
	fpMapID           = 21
	fpMapName         = 22
	fpMoveAndRename   = 23
	fpOpenVol         = 24
	fpOpenDir         = 25
	fpOpenFork        = 26
	fpRead            = 27
	fpRename          = 28
	fpSetDirParms     = 29
	fpSetFileParms    = 30
	fpSetForkParms    = 31
	fpSetVolParms     = 32
	fpWrite           = 33
	fpGetFileDirParms = 34
	fpSetFileDirParms = 35
	fpOpenDT          = 48
	fpCloseDT         = 49
	fpGetIcon         = 51
	fpGetIconInfo     = 52
	fpAddAPPL         = 53
	fpRemoveAPPL      = 54
	fpGetAPPL         = 55
	fpAddComment      = 56
	fpRemoveComment   = 57
	fpGetComment      = 58
	fpAddIcon         = 192
)

// volumeCommand runs the calls on the volume and what is in it
func (s *Server) volumeCommand(state *sessionState, session int, call uint8, request []uint8) ([]uint8, int32) {
	v := s.volume
	r := &reader{data: request, at: 1}

	switch call {
	case fpOpenVol:
		return v.openVol(r)
	case fpGetVolParms:
		return v.getVolParms(r)
	case fpSetVolParms, fpFlush, fpCloseVol:
		return nil, errNoErr

	case fpGetFileDirParms:
		return v.getFileDirParms(r)
	case fpSetFileDirParms:
		return v.setParms(r, true, true)
	case fpSetFileParms:
		return v.setParms(r, true, false)
	case fpSetDirParms:
		return v.setParms(r, false, true)
	case fpEnumerate:
		return v.enumerate(r)
	case fpOpenDir:
		return v.openDir(r)
	case fpCloseDir:
		return nil, errNoErr

	case fpCreateFile:
		return v.createFile(r)
	case fpCreateDir:
		return v.createDir(r)
	case fpDelete:
		return v.delete(r)
	case fpRename:
		return v.rename(r)
	case fpMoveAndRename:
		return v.moveAndRename(r)

	case fpOpenFork:
		return v.openFork(r, session)
	case fpRead:
		return v.read(r, session)
	case fpCloseFork:
		return v.closeFork(r, session)
	case fpFlushFork:
		return v.flushFork(r, session)
	case fpGetForkParms:
		return v.getForkParms(r, session)
	case fpSetForkParms:
		return v.setForkParms(r, session)
	case fpByteRangeLock:
		return v.byteRangeLock(r, session)

	case fpMapID:
		return mapID(r)
	case fpMapName:
		return mapName(r)

	case fpOpenDT:
		return v.desktop.open(v)
	case fpCloseDT:
		return nil, errNoErr
	case fpGetIcon:
		return v.desktop.getIcon(r)
	case fpGetIconInfo:
		return v.desktop.getIconInfo(r)
	case fpAddAPPL:
		return v.desktop.addAPPL(v, r)
	case fpRemoveAPPL:
		return v.desktop.removeAPPL(v, r)
	case fpGetAPPL:
		return v.desktop.getAPPL(v, r)
	case fpAddComment:
		return v.desktop.addComment(v, r)
	case fpRemoveComment:
		return v.desktop.removeComment(v, r)
	case fpGetComment:
		return v.desktop.getComment(v, r)
	}
	return nil, errCallNotSupported
}

// write runs the calls that come with data: FPWrite, and FPAddIcon with the
// icon
func (s *Server) write(state *sessionState, session int, request []uint8, data []uint8) ([]uint8, int32) {
	r := &reader{data: request, at: 1}
	switch request[0] {
	case fpWrite:
		return s.volume.write(r, session, data)
	case fpAddIcon:
		return s.volume.desktop.addIcon(r, data)
	}
	return nil, errCallNotSupported
}

/*
mapID names a user or a group by its number. There are no users or groups
here, everyone is a guest, and nobody owns anything: number zero is nobody,
and has no name.
*/
func mapID(r *reader) ([]uint8, int32) {
	r.byte()
	id := r.uint32()
	if r.failed {
		return nil, errParamErr
	}
	if id != 0 {
		return nil, errItemNotFound
	}
	return []uint8{0}, errNoErr
}

// mapName is the number of a user or a group by its name, and none has one
func mapName(r *reader) ([]uint8, int32) {
	return nil, errItemNotFound
}
