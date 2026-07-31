package izmac

import "fmt"

/*
Every Toolbox and operating system call of the Macintosh is an opcode of the
line 1010, unimplemented on the 68000, that vectors through the exception 10.
The handler reads the opcode back to know which call was made, which is why
iz68000 stacking the address of the trap itself and not of the next
instruction is load bearing rather than a detail.

The word is decoded as:

	1010 f nnnnnnnnnnn

	bit 11  0 for an operating system trap, 1 for a Toolbox one
	bit 10  auto pop for the Toolbox traps, a flag for the others
	rest    the trap number, 8 bits for the OS and 10 for the Toolbox

The tables below are the whole set, taken from the trap macros of the ROM
disassembly at ../macdocs/mac_rom, include/traps.inc.

The operating system traps carry flags on their bits 8 to 10 as well as the
number on the low eight: whether the call is asynchronous, whether it takes
memory from the system heap, whether the result comes back in A0. The name is
of the plain form, and the raw word is printed beside it so that the flags of
a variant are still there to read.
*/

const (
	trapMask    uint16 = 0xf000
	trapLineA   uint16 = 0xa000
	trapToolbox uint16 = 1 << 11

	trapOSNumberMask      uint16 = 0x00ff
	trapToolboxNumberMask uint16 = 0x03ff
)

// isLineA tells if a word is a line 1010 opcode
func isLineA(word uint16) bool {
	return word&trapMask == trapLineA
}

// osTrapNames returns the operating system traps, $A000 to $A0ff
func osTrapNames() map[uint16]string {
	return map[uint16]string{
		0x00: "Open", 0x01: "Close", 0x02: "Read",
		0x03: "Write", 0x04: "Control", 0x05: "Status",
		0x07: "HGetVInfo", 0x08: "Create", 0x0a: "OpenRF",
		0x0c: "GetFileInfo", 0x0d: "SetFileInfo", 0x0f: "MountVol",
		0x11: "GetEOF", 0x12: "SetEOF", 0x13: "FlushVol",
		0x14: "GetVol", 0x15: "SetVol", 0x17: "Eject",
		0x19: "InitZone", 0x1e: "NewPtr", 0x1f: "DisposePtr",
		0x20: "SetPtrSize", 0x21: "GetPtrSize", 0x22: "NewHandle",
		0x23: "DisposeHandle", 0x24: "SetHandleSize", 0x25: "GetHandleSize",
		0x26: "HandleZone", 0x27: "ReallocHandle", 0x28: "RecoverHandle",
		0x29: "HLock", 0x2a: "HUnlock", 0x2b: "EmptyHandle",
		0x2c: "InitApplZone", 0x2d: "SetApplLimit", 0x2e: "BlockMove",
		0x2f: "PostEvent", 0x30: "OSEventAvail", 0x31: "GetOSEvent",
		0x32: "FlushEvents", 0x33: "VInstall", 0x34: "VRemove",
		0x35: "OffLine", 0x38: "WriteParam", 0x3b: "Delay",
		0x3c: "CmpString", 0x3d: "DrvrInstall", 0x3f: "InitUtil",
		0x40: "ResrvMem", 0x46: "GetTrapAddress", 0x4b: "SetGrowZone",
		0x4c: "CompactMem", 0x4d: "PurgeMemSys", 0x4e: "AddDrive",
		0x4f: "RDrvrInstall", 0x50: "CompareString", 0x51: "ReadXPRam",
		0x52: "WriteXPRam", 0x54: "UprStringMarks", 0x57: "SetApplBase",
		0x58: "InsTime", 0x60: "HFSDispatch", 0x61: "MaxBlock",
		0x64: "MoveHHi", 0x66: "NewEmptyHandle", 0x6c: "InitFS",
		0x6d: "InitEvents",
	}
}

// toolboxTrapNames returns the Toolbox traps, $A800 to $Abff
func toolboxTrapNames() map[uint16]string {
	return map[uint16]string{
		0x013: "TEAutoView", 0x015: "SCSIDispatch", 0x033: "ScrnBitMap",
		0x051: "SetCursor", 0x052: "HideCursor", 0x053: "ShowCursor",
		0x056: "ObscureCursor", 0x060: "WaitNextEvent", 0x061: "Random",
		0x067: "LongMul",
		0x068: "FixMul", 0x069: "FixRatio", 0x06d: "InitPort",
		0x06e: "InitGraf", 0x06f: "OpenPort", 0x071: "GlobalToLocal",
		0x073: "SetPort", 0x074: "GetPort", 0x076: "PortSize",
		0x077: "MovePortTo", 0x079: "SetClip", 0x07a: "GetClip",
		0x07b: "ClipRect", 0x07d: "ClosePort", 0x07e: "AddPt",
		0x07f: "SubPt", 0x083: "DrawChar", 0x084: "DrawString",
		0x085: "DrawText", 0x086: "TextWidth", 0x087: "TextFont",
		0x088: "TextFace", 0x08b: "GetFontInfo", 0x08c: "StringWidth",
		0x08d: "CharWidth", 0x091: "LineTo", 0x093: "MoveTo",
		0x094: "Move", 0x096: "HidePen", 0x097: "ShowPen",
		0x098: "GetPenState", 0x099: "SetPenState", 0x09a: "GetPen",
		0x09b: "PenSize", 0x09c: "PenMode", 0x09d: "PenPat",
		0x09e: "PenNormal", 0x0a1: "FrameRect", 0x0a2: "PaintRect",
		0x0a3: "EraseRect", 0x0a4: "InvertRect", 0x0a5: "FillRect",
		0x0a8: "OffsetRect", 0x0a9: "InsetRect", 0x0aa: "SectRect",
		0x0ad: "PtInRect", 0x0ae: "EmptyRect", 0x0b0: "FrameRoundRect",
		0x0b2: "EraseRoundRect", 0x0b3: "InvertRoundRect", 0x0b4: "FillRoundRect",
		0x0d3: "PaintRgn", 0x0d4: "EraseRgn", 0x0d8: "NewRgn",
		0x0d9: "DisposeRgn", 0x0da: "OpenRgn", 0x0db: "CloseRgn",
		0x0dc: "CopyRgn", 0x0dd: "SetEmptyRgn", 0x0de: "SetRectRgn",
		0x0df: "RectRgn", 0x0e0: "OffsetRgn", 0x0e1: "InsetRgn",
		0x0e2: "EmptyRgn", 0x0e3: "EqualRgn", 0x0e4: "SectRgn",
		0x0e5: "UnionRgn", 0x0e6: "DiffRgn", 0x0e7: "XOrRgn",
		0x0e8: "PtInRgn", 0x0e9: "RectInRgn", 0x0ec: "CopyBits",
		0x0ef: "ScrollRect", 0x0f5: "KillPicture", 0x0f6: "DrawPicture",
		0x0fe: "InitFonts", 0x105: "DragGrayRgn", 0x106: "NewString",
		0x107: "SetString", 0x108: "ShowHide", 0x109: "CalcVis",
		0x10a: "CalcVBehind", 0x10b: "ClipAbove", 0x10c: "PaintOne",
		0x10d: "PaintBehind", 0x10e: "SaveOld", 0x10f: "DrawNew",
		0x111: "CheckUpDate", 0x113: "NewWindow", 0x115: "ShowWindow",
		0x11b: "MoveWindow", 0x11c: "HiliteWindow", 0x11e: "TrackGoAway",
		0x11f: "SelectWindow", 0x122: "BeginUpDate", 0x123: "EndUpDate",
		0x124: "FrontWindow", 0x125: "DragWindow", 0x126: "DragTheRgn",
		0x127: "InvalRgn", 0x128: "InvalRect", 0x12a: "ValidRect",
		0x12c: "FindWindow", 0x12d: "CloseWindow", 0x134: "ClearMenuBar",
		0x135: "InsertMenu", 0x137: "DrawMenuBar", 0x138: "HiliteMenu",
		0x13b: "GetMenuBar", 0x13c: "SetMenuBar", 0x148: "CalcMenuSize",
		0x14b: "PlotIcon", 0x14c: "FlashMenuBar", 0x14e: "PinRect",
		0x150: "CountMItems", 0x153: "UpdtControl", 0x154: "NewControl",
		0x155: "DisposeControl", 0x156: "KillControls", 0x157: "ShowControl",
		0x158: "HideControl", 0x159: "MoveControl", 0x15d: "HiliteControl",
		0x15f: "SetControlTitle", 0x166: "TestControl", 0x168: "TrackControl",
		0x169: "DrawControls", 0x16c: "FindControl", 0x16e: "Dequeue",
		0x16f: "Enqueue", 0x170: "GetNextEvent", 0x171: "EventAvail",
		0x172: "GetMouse", 0x173: "StillDown", 0x174: "Button",
		0x175: "TickCount", 0x176: "GetKeys", 0x177: "WaitMouseUp",
		0x17d: "NewDialog", 0x17f: "IsDialogEvent", 0x180: "DialogSelect",
		0x181: "DrawDialog", 0x182: "CloseDialog", 0x183: "DisposeDialog",
		0x18d: "GetDialogItem", 0x191: "ModalDialog", 0x192: "DetachResource",
		0x195: "InitResources", 0x196: "RsrcZoneInit", 0x197: "OpenResFile",
		0x199: "UpdateResFile", 0x19a: "CloseResFile", 0x19b: "SetResLoad",
		0x19c: "CountResources", 0x19d: "GetIndResource", 0x1a0: "GetResource",
		0x1a1: "GetNamedResource", 0x1a2: "LoadResource", 0x1a3: "ReleaseResource",
		0x1a6: "GetResAttrs", 0x1a8: "GetResInfo", 0x1b0: "WriteResource",
		0x1b2: "SystemEvent", 0x1b4: "SystemTask", 0x1b5: "SystemMenu",
		0x1b7: "CloseDeskAcc", 0x1b8: "GetPattern", 0x1ba: "GetString",
		0x1bb: "GetIcon", 0x1be: "GetNewControl", 0x1c5: "RsrcMapEntry",
		0x1c8: "SysBeep", 0x1c9: "SysError", 0x1cd: "TEDispose",
		0x1ce: "TETextBox", 0x1cf: "TESetText", 0x1d0: "TECalText",
		0x1d2: "TENew", 0x1d3: "TEUpdate", 0x1d4: "TEClick",
		0x1d5: "TECopy", 0x1d7: "TEDelete", 0x1d8: "TEActivate",
		0x1d9: "TEDeactivate", 0x1da: "TEIdle", 0x1dc: "TEKey",
		0x1dd: "TEScroll", 0x1e0: "Munger", 0x1e1: "HandToHand",
		0x1e2: "PtrToXHand", 0x1e3: "PtrToHand", 0x1e6: "InitAllPacks",
		0x1eb: "FP68K", 0x1ef: "PtrAndHand", 0x1f2: "Launch",
		0x1f9: "InfoScrap", 0x1fa: "UnloadScrap", 0x1fb: "LoadScrap",
		0x1fc: "ZeroScrap", 0x1fd: "GetScrap", 0x1fe: "PutScrap",
		0x1ff: "Debugger",
	}
}

// trapName describes a line 1010 opcode
func trapName(word uint16) string {
	if word&trapToolbox != 0 {
		number := word & trapToolboxNumberMask
		if name, known := toolboxTrapNames()[number]; known {
			return name
		}
		return fmt.Sprintf("Toolbox trap %v", number)
	}

	number := word & trapOSNumberMask
	if name, known := osTrapNames()[number]; known {
		return name
	}
	return fmt.Sprintf("OS trap %v", number)
}

// traceToolboxAt prints the trap about to run, if the instruction at the
// program counter is one
func (m *Mac) traceToolboxAt(pc uint32) {
	word := uint16(m.mm.Peek(pc))<<8 | uint16(m.mm.Peek(pc+1))
	if !isLineA(word) {
		return
	}

	fmt.Printf("%09d  $%06x  $%04x  %v\n", m.cycles, pc, word, trapName(word))
}
