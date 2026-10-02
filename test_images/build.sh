#!/bin/sh
#
# Builds the test images from the disks they are taken from, with hfsutils.
# The images are in the repository, and this is only needed to make them
# again or change them; see README.md for what is on each.
#
# The sources, which are not in the repository, are named by these, with the
# defaults the images were first built from:
#
#   UTILITIES   the Utilities 1 diskette of System 6.0.8, a startup diskette
#               with AppleShare installed
#   MACPACK     a MacPack hard disk image with System 6.0.8 and System 7.1.2
#               folders, TeachText at its root, and the Apple driver
#   SUPPLEMENT  the MacPack supplement disk, with the System Extras folder
#   DRIVER      a blank disk formatted by Apple's HD SC Setup
#   ROM         the Macintosh Plus ROM v3, checksum 4D1F8172
#   MACPAINT    the MacPaint 1.5 diskette, with System 2.0
#
# After building, boot each once so the Finder makes its desktop file:
#
#   IZMAC_SETTLE_TEST_IMAGES=1 go test -run TestSettleTestImages .
#
set -e

UTILITIES=${UTILITIES:-"izmac_sys608 - Utilities 1.dsk"}
MACPACK=${MACPACK:-frontend/macebiten/HD20SC_7.0.vhd}
SUPPLEMENT=${SUPPLEMENT:-frontend/macebiten/Supplement.vhd}
DRIVER=${DRIVER:-izmac_hddriver.rom}
ROM=${ROM:-izmac_default.rom}
MACPAINT=${MACPAINT:-frontend/macebiten/izmac_macpaint.dsk}

OUT=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
trap 'humount >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT

# The MacPack disk has its HFS volume after the partition map and the driver,
# 96 blocks in; the first two blocks of a volume are its boot blocks
MACPACK_VOLUME=96

# get <image> <path> <file>: a file taken out with both forks, as MacBinary
get() {
	hmount "$1" >/dev/null
	hcopy -m "$2" "$WORK/$3"
	humount >/dev/null
}

# put <path> <file>: a file put in with both forks, on the mounted volume
put() {
	hcopy -m "$WORK/$2" "$1"
}

# text <path> <words>: a TeachText document, with a carriage return at the end
text() {
	printf '%s\r' "$2" >"$WORK/text.txt"
	hcopy -t "$WORK/text.txt" "$1"
	hattrib -t TEXT -c ttxt "$1"
}

# blank <image> <kilobytes> <name>: an empty HFS volume
blank() {
	dd if=/dev/zero of="$1" bs=1024 count="$2" 2>/dev/null
	hformat -l "$3" "$1" >/dev/null
}

echo "Taking the files out of the disks they are on"
get "$UTILITIES" ":System Folder:System" u1-system
get "$UTILITIES" ":System Folder:Finder" u1-finder
get "$UTILITIES" ":System Folder:AppleShare" u1-appleshare
get "$UTILITIES" ":System Folder:DA Handler" u1-dahandler
get "$UTILITIES" ":System Folder:Multifinder" u1-multifinder
get "$MACPACK" ":TeachText" teachtext
for f in System Finder MultiFinder General "Startup Device" "Scrapbook File" Backgrounder; do
	get "$MACPACK" ":System 6.0.8:$f" "s6-$f"
done
S7=":System 7.1.2"
for f in System Finder "Scrapbook File"; do
	get "$MACPACK" "${S7}:$f" "s7-$f"
done
for f in Calculator Chooser "Key Caps" "Note Pad" Scrapbook; do
	get "$MACPACK" "${S7}:Apple Menu Items:$f" "s7-ami-$f"
done
for f in "General Controls" Keyboard Mouse "Startup Disk" Views; do
	get "$MACPACK" "${S7}:Control Panels:$f" "s7-cp-$f"
done
for f in "Sound Manager" "System Update"; do
	get "$MACPACK" "${S7}:Extensions:$f" "s7-ext-$f"
done
for f in Chicago Geneva Monaco; do
	get "$MACPACK" "${S7}:Fonts:$f" "s7-font-$f"
done
X=":System Extras:7.1.2 Extras"
for f in AppleShare "File Sharing Extension" "Network Extension"; do
	get "$SUPPLEMENT" "${X}:Extensions:$f" "s7-ext-$f"
done
for f in "Sharing Setup" "Users & Groups"; do
	get "$SUPPLEMENT" "${X}:Control Panels:$f" "s7-cp-$f"
done

echo "system6.dsk: System 6.0.8 on an 800K diskette"
blank "$WORK/system6.dsk" 800 "System 6"
dd if="$UTILITIES" of="$WORK/system6.dsk" bs=512 count=2 conv=notrunc 2>/dev/null
hmount "$WORK/system6.dsk" >/dev/null
hmkdir ":System Folder"
put ":System Folder:System" u1-system
put ":System Folder:Finder" u1-finder
put ":System Folder:AppleShare" u1-appleshare
put ":System Folder:DA Handler" u1-dahandler
put ":System Folder:MultiFinder" u1-multifinder
put ":TeachText" teachtext
text ":Read Me" "This is a text file on the System 6 test diskette."
hattrib -b ":System Folder"
humount >/dev/null

echo "system6.img: System 6.0.8 on a bare 2 MB volume"
blank "$WORK/system6.img" 2048 "System 6 HD"
dd if="$MACPACK" of="$WORK/system6.img" bs=512 skip=$MACPACK_VOLUME count=2 conv=notrunc 2>/dev/null
hmount "$WORK/system6.img" >/dev/null
hmkdir ":System Folder"
for f in System Finder MultiFinder General "Startup Device" "Scrapbook File" Backgrounder; do
	put ":System Folder:$f" "s6-$f"
done
put ":TeachText" teachtext
text ":Read Me" "This is a text file on the System 6 test disk."
hmkdir ":Empty Folder"
hattrib -b ":System Folder"
humount >/dev/null

echo "system7.img: System 7.1.2 on a partitioned 4 MB disk"
blank "$WORK/system7.hfs" 4096 "System 7 HD"
dd if="$MACPACK" of="$WORK/system7.hfs" bs=512 skip=$MACPACK_VOLUME count=2 conv=notrunc 2>/dev/null
hmount "$WORK/system7.hfs" >/dev/null
F=":System Folder"
hmkdir "$F"
for d in "Apple Menu Items" "Control Panels" Extensions Fonts Preferences "Startup Items"; do
	hmkdir "${F}:$d"
done
for f in System Finder "Scrapbook File"; do
	put "${F}:$f" "s7-$f"
done
for f in Calculator Chooser "Key Caps" "Note Pad" Scrapbook; do
	put "${F}:Apple Menu Items:$f" "s7-ami-$f"
done
for f in "General Controls" Keyboard Mouse "Startup Disk" Views "Sharing Setup" "Users & Groups"; do
	put "${F}:Control Panels:$f" "s7-cp-$f"
done
for f in "Sound Manager" "System Update" AppleShare "File Sharing Extension" "Network Extension"; do
	put "${F}:Extensions:$f" "s7-ext-$f"
done
for f in Chicago Geneva Monaco; do
	put "${F}:Fonts:$f" "s7-font-$f"
done
put ":TeachText" teachtext
text ":Read Me" "This is a text file on the System 7 test disk."
hmkdir ":Shared"
hattrib -b "$F"
humount >/dev/null

# The partition map and the driver of the blank disk, before the volume, with
# the sizes in the map made the volume's, and its last 32 blocks free
python3 - "$DRIVER" "$WORK/system7.hfs" "$WORK/system7.img" <<'EOF'
import struct, sys
driver = open(sys.argv[1], 'rb').read()
volume = open(sys.argv[2], 'rb').read()
start, blocks, free = 96, len(volume) // 512, 32
disk = bytearray(driver[:start * 512])
struct.pack_into('>I', disk, 4, start + blocks + free)
for i in range(1, 8):
    entry = i * 512
    if disk[entry:entry + 2] != b'PM':
        break
    kind = bytes(disk[entry + 48:entry + 80]).split(b'\0')[0]
    if kind == b'Apple_HFS':
        assert struct.unpack('>I', disk[entry + 8:entry + 12])[0] == start
        struct.pack_into('>I', disk, entry + 12, blocks)
        struct.pack_into('>I', disk, entry + 84, blocks)
    elif kind == b'Apple_Free':
        struct.pack_into('>I', disk, entry + 8, start + blocks)
open(sys.argv[3], 'wb').write(disk + volume + bytes(free * 512))
EOF

echo "The rest as they are"
cp "$ROM" "$WORK/macplus.rom"
cp "$DRIVER" "$WORK/hddriver.img"
cp "$MACPAINT" "$WORK/macpaint.dsk"

for f in system6.dsk system6.img system7.img macplus.rom hddriver.img macpaint.dsk; do
	cp "$WORK/$f" "$OUT/$f"
	chmod 644 "$OUT/$f"
done
echo "Done, now settle them: IZMAC_SETTLE_TEST_IMAGES=1 go test -run TestSettleTestImages ."
