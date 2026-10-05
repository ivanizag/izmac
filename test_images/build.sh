#!/bin/sh
#
# Builds the test images from the disks they are taken from, with hfsutils,
# python3, unzip and unar. The images are in the repository, and this is only needed
# to make them again or change them; see README.md for what is on each.
#
# The disks they are taken from are downloaded once, from the Internet Archive
# and, for THINK Pascal, from WinWorld: they are kept in
# IZMAC_TEST_IMAGES_CACHE, ~/.cache/izmac-test-images unless it says
# otherwise, and checked against the SHA-256 they had when the images were
# first built from them. About 385 MB in all, most of it the supplement
# disk of the MacPack, which is the one place the File Sharing of System 7.1.2
# was found ready to copy.
#
#   macplus.rom     the Macintosh Plus ROM v3, from the Macintosh ROM archive,
#                   the same izmac downloads when it is not given one
#   macpaint.dsk    MacPaint 1.5 with System 2.0, the diskette izmac starts
#                   when nothing is named
#   HD20SC.vhd      the hard disk of the MacPack, a pack of software for the
#                   Macintosh Plus core of MiSTer: System 6.0.8 and System
#                   7.1.2 folders, and Apple's partition map and driver
#   Supplement.vhd  the supplement disk of the MacPack, with the System Extras
#                   of each System, File Sharing among them
#   DSK.zip         the diskettes of the MacPack, the 800K set of System 6.0.8
#                   among them: Utilities 1, with AppleShare installed,
#                   Utilities 2, with the Key Layout and the control panels,
#                   and Printing Tools, with the ImageWriter driver and
#                   TeachText. The four of the set are also kept as they are,
#                   for installing System 6.
#
# And the disks and archives of the activities of doc/activities, kept as
# they were downloaded, each the one a reader is sent to:
#
#   macwrite.dsk    MacWrite 4.5 with System 2.0, the Disk Write diskette
#   multiplan.dsk   Microsoft Multiplan 1.11
#   basic.dsk       Microsoft BASIC 2.0, in DiskCopy 4.2
#   hypercard.dsk   HyperCard 1.1, its Startup diskette, out of a RAR of the
#                   whole set of four with the scans of the box
#   resedit.sit     ResEdit 2.1, the diskette of the book ResEdit Complete in
#                   a StuffIt archive, out of a zip with the scans
#   bolo.sit        Bolo 0.99.7, as the Tucows archive kept it
#   loderunner.dsk  Lode Runner, and darkcastle.dsk Dark Castle 1.2
#   thinkpascal-1.dsk, thinkpascal-2.dsk
#                   the first two of the four diskettes of THINK Pascal 4.0,
#                   the application and its interfaces and libraries, out of
#                   the 7-Zip archive WinWorld keeps of the set, which is not
#                   on the Internet Archive
#
# After building, boot each once so the Finder makes its desktop file:
#
#   IZMAC_SETTLE_TEST_IMAGES=1 go test -run TestSettleTestImages ./e2e_tests
#
set -e

ARCHIVE=https://archive.org/download
MACPACK_ZIP=$ARCHIVE/macpack/MacPack-20240308.zip
CACHE=${IZMAC_TEST_IMAGES_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/izmac-test-images}

OUT=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
trap 'humount >/dev/null 2>&1 || true; rm -rf "$WORK"' EXIT
mkdir -p "$CACHE"

sha256() {
	if command -v sha256sum >/dev/null; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# fetch <name> <url> <sha256>: a source, downloaded into the cache the first
# time and checked every time, and a copy of it in the work directory, since
# hfsutils writes to a disk it mounts
fetch() {
	if [ ! -f "$CACHE/$1" ]; then
		echo "Downloading $1 from $2"
		curl -fL --retry 5 --retry-all-errors --retry-delay 5 -o "$CACHE/$1.part" "$2"
		mv "$CACHE/$1.part" "$CACHE/$1"
	fi
	if [ "$(sha256 "$CACHE/$1")" != "$3" ]; then
		echo "$CACHE/$1 is not the file the images were built from; delete it to download it again" >&2
		exit 1
	fi
	cp "$CACHE/$1" "$WORK/$1"
}

fetch macplus.rom "$ARCHIVE/mac_rom_archive_-_as_of_8-19-2011/mac_rom_archive_-_as_of_8-19-2011.zip/4D1F8172%20-%20MacPlus%20v3.ROM" \
	dd908e2b65772a6b1f0c859c24e9a0d3dcde17b1c6a24f4abd8955846d7895e7
fetch macpaint.dsk "$ARCHIVE/mac_Paint_2/Paint_2.dsk" \
	735b0f5c557937f7177ef64b94d85b702074ef59d71f67c6a0c95c099d99e483
fetch HD20SC.vhd "$MACPACK_ZIP/HD20SC.vhd" \
	d4b3d697bf36d1fdb5cc1b529ae9bb67ffd34bd8dbd175bac8cdabc6fa2b71ad
fetch Supplement.vhd "$MACPACK_ZIP/Supplement.vhd" \
	b42502933e869d68f4a28ec30d2fef1571025f1523bac6d13715b9b5fe4ff2e5
fetch DSK.zip "$MACPACK_ZIP/DSK.zip" \
	ff5eca49dd9c3f63bf5e72ba35a897c560cc86e0bb4b93944bb0f5f42f6b2152
fetch macwrite.dsk "$ARCHIVE/mac_Disk_Write_2/Disk_Write_2.dsk" \
	94c4bddf64c5d474342326c0b999ac1cbac8efcf10d7432062707e4a97fa195d
fetch multiplan.dsk "$ARCHIVE/mac_MSMultiplan_1.11/MSMultiplan_1.11.dsk" \
	58b3088be10e2c051103be4feeac99ec00c39f0bc1ef8da5fc88ae8b40ddb064
fetch basic.dsk "$ARCHIVE/mac_MSBASIC_2/MSBASIC_2.dsk" \
	71ff985e69d11cf75a92320a9c02f566b8d0f5f5e5ba3c86c51341e8974e7183
fetch HyperCard11.rar "$ARCHIVE/apple-hyper-card-1.1-1987-english-3.5-800-kb/Apple%20HyperCard%201.1%20(1987)%20%5BEnglish%5D%20(3.5''-800KB).rar" \
	75e3a303f5046ea497563e8756c0202d5dc203a34e285854d1a0afc5172878e4
fetch ResEdit21.zip "$ARCHIVE/apple-res-edit-2.1-for-mac-addison-wesley-edition-2.1-1990-12-english-3.5-800-k/Apple%20ResEdit%202.1%20for%20Mac%20Addison-Wesley%20Edition%20(2.1)%20(1990-12)%20%5BEnglish%5D%20(3.5''-800K).zip" \
	3fb12c2deef58a69d74dc15f81534b2d03acb818c8c034a178a70ab236ac0742
fetch bolo.sit "$ARCHIVE/tucows_205988_Bolo/bolojolopak.sit" \
	70836d2474a0c66ead15148e31252b9601bb40463b09197bd65c0e56ae693d4c
fetch loderunner.dsk "$ARCHIVE/mac_Lode_Runner/Lode_Runner.dsk" \
	2d9a85fc60c4bdc62b4e2f4aa9d81ad0a0a7de2c19a71ce0698c89b7517230f1
fetch darkcastle.dsk "$ARCHIVE/mac_DarkCastle_1_2/DarkCastle_1_2.dsk" \
	102e644bb7aa85b28efe362d9fe94f58fa721988ed9939762b3f51ec83f0a0b1
fetch ThinkPascal40.7z "https://winworldpc.com/download/3295d3eb-1e78-11ec-ad33-0200008a0da4/from/c39ac2af-c381-c2bf-1b25-11c3a4e284a2" \
	51faeb5c80e1b949d373039686e48a2e8d3eb716a5d4385f5a67ae96c1ba9584

MACPACK="$WORK/HD20SC.vhd"
SUPPLEMENT="$WORK/Supplement.vhd"
UTILITIES="$WORK/utilities1.dsk"
UTILITIES2="$WORK/utilities2.dsk"
PRINTING="$WORK/printingtools.dsk"
TOOLS="$WORK/systemtools.dsk"
unzip -p "$WORK/DSK.zip" "800K/System608/Utilities 1.dsk" >"$UTILITIES"
unzip -p "$WORK/DSK.zip" "800K/System608/Utilities 2.dsk" >"$UTILITIES2"
unzip -p "$WORK/DSK.zip" "800K/System608/Printing Tools.dsk" >"$PRINTING"
unzip -p "$WORK/DSK.zip" "800K/System608/System Tools.dsk" >"$TOOLS"

# The archives of the activities: HyperCard's Startup diskette out of its RAR,
# and ResEdit's StuffIt archive out of its zip
unar -q -o "$WORK/hypercard" "$WORK/HyperCard11.rar" "*/IMG/HyperCard Startup.img" >/dev/null
find "$WORK/hypercard" -name "HyperCard Startup.img" -exec cp {} "$WORK/hypercard.dsk" \;
unzip -p "$WORK/ResEdit21.zip" "*/IMG/ResEdit 2.1.sit" >"$WORK/resedit.sit"

# And the first two diskettes of THINK Pascal out of its 7-Zip archive
unar -q -o "$WORK/thinkpascal" "$WORK/ThinkPascal40.7z" "*/disk01.img" "*/disk02.img" >/dev/null
find "$WORK/thinkpascal" -name disk01.img -exec cp {} "$WORK/thinkpascal-1.dsk" \;
find "$WORK/thinkpascal" -name disk02.img -exec cp {} "$WORK/thinkpascal-2.dsk" \;

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
get "$PRINTING" ":Apple Color:TeachText" teachtext
get "$PRINTING" ":ImageWriter" imagewriter
for f in "Key Layout" Keyboard Mouse Sound; do
	get "$UTILITIES2" ":System Folder Additions:$f" "u2-$f"
done
for f in System Finder General "Startup Device" "Scrapbook File" "Clipboard File"; do
	get "$MACPACK" ":System 4.1:$f" "s41-$f"
done
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
put ":System Folder:ImageWriter" imagewriter
for f in "Key Layout" Keyboard Mouse Sound; do
	put ":System Folder:$f" "u2-$f"
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

# The partition map and the driver of the MacPack disk, before the volume, with
# the sizes in the map made the volume's
python3 - "$MACPACK" "$WORK/system7.hfs" "$WORK/system7.img" <<'PYTHON'
import struct, sys
macpack = open(sys.argv[1], 'rb').read()
volume = open(sys.argv[2], 'rb').read()
start, blocks = 96, len(volume) // 512
disk = bytearray(macpack[:start * 512])
struct.pack_into('>I', disk, 4, start + blocks)
for i in range(1, 8):
    entry = i * 512
    if disk[entry:entry + 2] != b'PM':
        break
    if bytes(disk[entry + 48:entry + 80]).split(b'\0')[0] == b'Apple_HFS':
        assert struct.unpack('>I', disk[entry + 8:entry + 12])[0] == start
        struct.pack_into('>I', disk, entry + 12, blocks)
        struct.pack_into('>I', disk, entry + 84, blocks)
open(sys.argv[3], 'wb').write(disk + volume)
PYTHON

echo "system41.dsk: System 4.1 and the Finder 5.5 on an 800K diskette"
blank "$WORK/system41.dsk" 800 "System 4.1"
dd if="$MACPACK" of="$WORK/system41.dsk" bs=512 skip=$MACPACK_VOLUME count=2 conv=notrunc 2>/dev/null
hmount "$WORK/system41.dsk" >/dev/null
hmkdir ":System Folder"
for f in System Finder General "Startup Device" "Scrapbook File" "Clipboard File"; do
	put ":System Folder:$f" "s41-$f"
done
hattrib -b ":System Folder"
humount >/dev/null

echo "The rest as they are"
cp "$TOOLS" "$WORK/system-tools.dsk"
cp "$UTILITIES" "$WORK/utilities-1.dsk"
cp "$UTILITIES2" "$WORK/utilities-2.dsk"
cp "$PRINTING" "$WORK/printing-tools.dsk"
cp "$WORK/teachtext" "$WORK/teachtext.bin"

# The partition map and the driver of the MacPack disk, cut short, is what a
# bare volume borrows its SCSI driver from
head -c 65536 "$CACHE/HD20SC.vhd" >"$WORK/hddriver.img"

for f in system6.dsk system6.img system7.img macplus.rom hddriver.img macpaint.dsk teachtext.bin \
	system41.dsk system-tools.dsk utilities-1.dsk utilities-2.dsk printing-tools.dsk \
	macwrite.dsk multiplan.dsk basic.dsk hypercard.dsk resedit.sit bolo.sit \
	loderunner.dsk darkcastle.dsk thinkpascal-1.dsk thinkpascal-2.dsk; do
	cp "$WORK/$f" "$OUT/$f"
	chmod 644 "$OUT/$f"
done
echo "Done, now settle them: IZMAC_SETTLE_TEST_IMAGES=1 go test -run TestSettleTestImages ./e2e_tests"
