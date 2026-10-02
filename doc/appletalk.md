# AppleTalk

[Back to the manual](manual.md)

A Macintosh Plus speaks AppleTalk out of its printer port, over LocalTalk. All
of it is in the ROM: the drivers that take a node address on the network, find
names on it and move packets across it. What izmac adds is the side of the
serial chip LocalTalk runs on, the synchronous one, and a network on the other
end of the wire.

AppleTalk is off unless you ask for it:

```bash
izmac -appletalk local System.img
```

`local` is a LocalTalk network with nothing on it but this machine. `host`
puts it on a LocalTalk with the other izmacs on this computer, and `udp` on
the LocalTalk of your local network:

```bash
izmac -appletalk host System.img
izmac -appletalk udp System.img
```

## Other machines

With `udp` the machine is on LocalTalk over UDP, the way Mini vMac 37 and later
do it: every frame goes to the multicast group `239.192.76.84`, port `1954`,
and everything listening there is on the same LocalTalk. That is other izmacs,
on this computer or another one on the same network, and Mini vMacs. Through a
bridge such as [MultiTalk](https://github.com/sfiera/multitalk) or TashRouter
it is also EtherTalk, a netatalk file server, and real Macintoshes on real
LocalTalk.

Two izmacs on one computer need a disk image each: two machines writing to the
same file corrupt it.

The first time, macOS asks whether izmac may accept incoming network
connections. Say yes: the frames of the other machines come in that way, and
without them each machine is alone on its network. Some networks do not carry
multicast between computers at all, a guest Wi-Fi among them, and a firewall
managed by someone else may keep it from leaving the computer. When frames do
not get out, izmac says so once, and the machine goes on alone.

`host` is the same LocalTalk over UDP kept inside the computer, on its
loopback interface. It reaches every izmac started with `host` on the same
computer and nothing else, and works where `udp` cannot: no firewall stands
between a computer and itself.

The LocalTalk handshake, the RTS and CTS before every frame to one node, never
crosses the network. It has to be answered within 200µs, which no network
does, so each machine answers it for itself, as Mini vMac does.

## Sharing a folder

`-share` puts a folder of your computer on the network, served by an AppleShare
file server inside izmac:

```bash
izmac -share ~/Documents/ForTheMac System.img
```

It turns AppleTalk on, `local` unless `-appletalk` says otherwise. The server
is a node of its own on the network, named after your computer, and the folder
is its one volume, under the folder's name. In the Chooser, pick AppleShare,
then the server, and log in as a guest: there are no users and no passwords,
and anyone who sees the server can use the folder. With `-appletalk host`, that
is every izmac on this computer, and with `udp` every machine on the LocalTalk
of your network.

The Macintosh needs the AppleShare client, the `AppleShare` file in the System
Folder that puts the AppleShare icon in the Chooser. System 6.0.8 has it on
the *Utilities 1* diskette, and System 7 in the Extensions folder.

Once logged in, choose the volume and it appears on the desktop like a disk,
to read and write. A file the machine makes, a folder, a rename, a move, are
the same on your computer at once.

What makes a Macintosh file more than its data, its resource fork and its type
and creator, is kept where the host keeps it. On macOS that is with the file
itself, where the Finder of the Mac copies it along, so applications can be
put in the folder and run from there. On Linux and Windows it is in a second
file next to each, named with `._` in front, the AppleDouble files netatalk
and macOS write on disks of other kinds. A file from the host that has none
is a plain document, or a text file for TeachText when its name ends in
`.txt`.

Names the machine cannot have as they are, longer than 31 characters or with
characters Mac OS Roman does not have, are shortened with a `#` and a number
in them. Files and folders whose names start with a dot are not shown.

The icons and the applications that open each kind of document, which the
Finder keeps in the Desktop file of a disk, are found by looking in the
applications on the folder the first time the volume opens, and kept until
izmac stops; so are the comments of Get Info.

## What changes

With AppleTalk on, the printer port is AppleTalk's, as it was on a Macintosh
on a network. The parameter RAM says so as the machine starts, whatever the
Chooser left in it last time: the option decides, in both directions.

The ImageWriter moves to the **modem port**, which is where a Macintosh on a
network had its printer cable. Choose the ImageWriter in the Chooser and pick
the modem port icon beside it; the printer port icon is greyed out while
AppleTalk holds it. Asking for the printer on the printer port with AppleTalk
on is refused at the start, with `-printer none` the one way round it.

## On System 6

System 6 opens AppleTalk when something needs it: the Chooser, a LaserWriter
driver, AppleShare. The first time it opens, it picks a node address from 1 to
127 and asks the network whether anyone has it, a few hundred times over,
before it takes it. On a network with nobody else on it, that is a second or
so of the Chooser looking busy.

## On System 7

System 7 opens AppleTalk as it starts, and keeps which connection to use in
the extended parameter RAM. izmac has the 256 bytes of it the Plus has, saved
with the rest in `izmac_pram.bin`.
