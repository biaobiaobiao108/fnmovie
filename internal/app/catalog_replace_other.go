//go:build !windows

package app

import "os"

func replaceCatalogFile(source, destination string) error {
	return os.Rename(source, destination)
}
