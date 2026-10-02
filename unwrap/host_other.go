//go:build !darwin

package unwrap

// readHostMetadata has nothing to read on a host that keeps no more of a file
// than its data, where AppleDouble files are all there is
func readHostMetadata(name string, f *File) {}
