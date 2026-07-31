package scrap

import (
	"strings"
	"testing"
)

// The entries of a block, in the order they are in it, with the padding of an
// odd length entry not turning into part of the next one
func TestTheEntriesOfABlockAreWalked(t *testing.T) {
	block := buildBlock(
		"TEXT", "odd",
		"styl", "even",
		"PICT", "odd again")

	entries := strings.Join(Entries(block), " ")
	if entries != "TEXT styl PICT" {
		t.Errorf("the block reads as %q, wanted %q", entries, "TEXT styl PICT")
	}
}

func TestAnEntryIsFoundByItsType(t *testing.T) {
	block := buildBlock(
		"styl", "first",
		"TEXT", "wanted")

	contents, found := Entry(block, TypeText)
	if !found {
		t.Fatal("the text entry was not found")
	}
	if string(contents) != "wanted" {
		t.Errorf("the entry holds %q, wanted %q", contents, "wanted")
	}

	if _, found := Entry(block, "PICT"); found {
		t.Error("an entry that is not in the block was found")
	}
}

/*
The scrap is read while the machine is running and not asked for, so a block
can be caught with an entry only half written. What was whole before it is
still good and is what the walk gives.
*/
func TestATruncatedEntryEndsTheWalk(t *testing.T) {
	block := buildBlock("TEXT", "whole", "styl", "cut short")
	block = block[:len(block)-4]

	entries := strings.Join(Entries(block), " ")
	if entries != "TEXT" {
		t.Errorf("the truncated block reads as %q, wanted %q", entries, "TEXT")
	}

	if _, found := Entry(block, "styl"); found {
		t.Error("an entry that runs off the end of the block was returned")
	}
}

// A length that could not fit in the block ends the walk rather than being
// followed off the end of it
func TestAnImpossibleLengthEndsTheWalk(t *testing.T) {
	block := []byte("TEXT\x7f\xff\xff\xffnot that long")

	if entries := Entries(block); len(entries) != 0 {
		t.Errorf("a block claiming a two gigabyte entry read as %v", entries)
	}
}

func TestABlockTooShortToHoldAnEntryIsEmpty(t *testing.T) {
	if entries := Entries([]byte("TEXT")); len(entries) != 0 {
		t.Errorf("four bytes read as %v entries", len(entries))
	}
	if entries := Entries(nil); len(entries) != 0 {
		t.Errorf("nothing read as %v entries", len(entries))
	}
}
