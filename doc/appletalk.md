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

`local` is a LocalTalk network with nothing on it but this machine. It is what
lets AppleTalk be turned on at all, and the base the rest is built on.

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
