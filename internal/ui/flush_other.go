//go:build !linux

package ui

import "os"

func flushInput(*os.File) {}
