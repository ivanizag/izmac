package izmac

import (
	"testing"
)

/*
hyperCardScreenshots is HyperCard 1.1 from its Startup diskette: the Home
card, the Address stack gone through, the user level raised to scripting, and
a new stack with a button whose script answers when it is clicked
*/
func hyperCardScreenshots(t *testing.T) {
	const page = "hypercard"
	config := testConfig(t)
	config.Diskettes = []string{testImage(t, testHyperCardDiskette)}
	m := buildTestMac(t, config)
	waitForApplication(t, m, "HyperCard", 60)
	m.RunFrames(2400)
	screenshot(t, m, page, "home")

	// The Address stack, a card at a time, recorded
	moveMouseTo(t, m, 210, 100)
	m.RunFrames(30)
	address := record(m, "")
	address.capture(100)
	address.press(true)
	address.run(6, 2)
	address.press(false)
	address.run(1500, 6)
	for i := 0; i < 4; i++ {
		pressCommand(m, "3")
		address.run(300, 6)
	}
	address.save(t, page, "address", 200)

	// The last card of Home, with the user level, set to scripting
	pressCommand(m, "H")
	m.RunFrames(900)
	pressCommand(m, "4")
	m.RunFrames(600)
	moveMouseTo(t, m, 104, 238)
	clickMouse(m)
	m.RunFrames(120)
	screenshot(t, m, page, "user-level")
	pressCommand(m, "H")
	m.RunFrames(600)

	// A new stack, Hello
	chooseFromMenu(t, m, 53, 27)
	m.RunFrames(600)
	typeString(m, "Hello")
	m.RunFrames(30)
	screenshot(t, m, page, "new-stack")
	pressKey(m, "Return")
	m.RunFrames(1200)

	// A new button on its card, named, and its script
	chooseFromMenu(t, m, 210, 171)
	m.RunFrames(300)
	screenshot(t, m, page, "new-button")
	chooseFromMenu(t, m, 210, 27)
	m.RunFrames(600)
	typeString(m, "Say hello")
	m.RunFrames(30)
	screenshot(t, m, page, "button-info")
	moveMouseTo(t, m, 147, 251)
	clickMouse(m)
	m.RunFrames(600)
	typeString(m, "answer \"Hello from 1987!\"")
	m.RunFrames(60)
	screenshot(t, m, page, "script")
	moveMouseTo(t, m, 370, 308)
	clickMouse(m)
	m.RunFrames(300)

	// The browse tool, and the button clicked, recorded
	typeKeys(m, "Command+Tab")
	m.RunFrames(120)
	moveMouseTo(t, m, 256, 150)
	m.RunFrames(30)
	hello := record(m, "")
	hello.capture(150)
	hello.press(true)
	hello.run(6, 2)
	hello.press(false)
	hello.run(300, 6)
	hello.save(t, page, "hello", 200)
	moveMouseTo(t, m, 360, 152)
	clickMouse(m)
	m.RunFrames(120)
}
