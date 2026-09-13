package storage

import "io"

type File interface {
	io.ReadSeekCloser

	// Name returns the name of the file including the extension.
	Name() string
	// SetName sets the name of the file. name should not include extension, otherwise it will sanitize the ext and use
	// original ext through Ext()
	SetName(name string) error
	// Ext returns the extension of the file.
	Ext() string
	// ContentType returns the content-type of the file
	ContentType() string
	// Return true if no file/content
	NoFile() bool
}
