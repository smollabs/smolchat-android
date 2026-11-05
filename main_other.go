//go:build !android

package main

import "gioui.org/io/event"

func platformEvent(event.Event) {}
