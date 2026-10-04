package activities

import (
	"path/filepath"
	"testing"

	"github.com/ivanizag/izmac/operator"
)

/*
hyperCardScreenshots is HyperCard 1.1 from its Startup diskette: the Home
card, the Address stack gone through, the user level raised to scripting, and
a new stack with a button whose script answers when it is clicked
*/
func hyperCardScreenshots(t *testing.T) {
	const page = "hypercard"
	album := NewAlbum(filepath.Join(activityImages, page))
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testHyperCardDiskette)}
	m := buildTestMac(t, config)
	must(t, operator.New(m).WaitForApplication("HyperCard", 60))
	m.RunFrames(2400)
	must(t, album.Screenshot(m, "home"))

	// The Address stack, a card at a time, recorded
	must(t, operator.New(m).MoveTo(210, 100))
	m.RunFrames(30)
	address := Record(operator.New(m), "")
	address.Capture(100)
	address.Press(true)
	address.Run(6, 2)
	address.Press(false)
	address.Run(1500, 6)
	for i := 0; i < 4; i++ {
		must(t, operator.New(m).Command("3"))
		address.Run(300, 6)
	}
	must(t, album.SaveRecording(address, "address", 200))

	// The last card of Home, with the user level, set to scripting
	must(t, operator.New(m).Command("H"))
	m.RunFrames(900)
	must(t, operator.New(m).Command("4"))
	m.RunFrames(600)
	must(t, operator.New(m).MoveTo(104, 238))
	operator.New(m).Click()
	m.RunFrames(120)
	must(t, album.Screenshot(m, "user-level"))
	must(t, operator.New(m).Command("H"))
	m.RunFrames(600)

	// A new stack, Hello
	must(t, operator.New(m).ChooseFromMenu(53, 27))
	m.RunFrames(600)
	must(t, operator.New(m).TypeString("Hello"))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "new-stack"))
	must(t, operator.New(m).PressKey("Return"))
	m.RunFrames(1200)

	// A new button on its card, named, and its script
	must(t, operator.New(m).ChooseFromMenu(210, 171))
	m.RunFrames(300)
	must(t, album.Screenshot(m, "new-button"))
	must(t, operator.New(m).ChooseFromMenu(210, 27))
	m.RunFrames(600)
	must(t, operator.New(m).TypeString("Say hello"))
	m.RunFrames(30)
	must(t, album.Screenshot(m, "button-info"))
	must(t, operator.New(m).MoveTo(147, 251))
	operator.New(m).Click()
	m.RunFrames(600)
	must(t, operator.New(m).TypeString("answer \"Hello from 1987!\""))
	m.RunFrames(60)
	must(t, album.Screenshot(m, "script"))
	must(t, operator.New(m).MoveTo(370, 308))
	operator.New(m).Click()
	m.RunFrames(300)

	// The browse tool, and the button clicked, recorded
	must(t, operator.New(m).TypeKeys("Command+Tab"))
	m.RunFrames(120)
	must(t, operator.New(m).MoveTo(256, 150))
	m.RunFrames(30)
	hello := Record(operator.New(m), "")
	hello.Capture(150)
	hello.Press(true)
	hello.Run(6, 2)
	hello.Press(false)
	hello.Run(300, 6)
	must(t, album.SaveRecording(hello, "hello", 200))
	must(t, operator.New(m).MoveTo(360, 152))
	operator.New(m).Click()
	m.RunFrames(120)
}
