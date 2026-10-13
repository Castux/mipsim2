package main

import (
	"image"
	"testing"
)

func TestCPULays(t *testing.T) {
	c := BuildCPU()
	l, err := c.Lay()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d gates, %d signals, %d tracks, %v", len(c.gates), len(c.sigs), l.Tracks, l.Bounds)
	dc := l.Document(image.Point{})
	if _, err := c.Verify(dc, l, image.Point{}); err != nil {
		t.Fatal(err)
	}
}
