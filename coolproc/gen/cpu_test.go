package main

import "testing"

// TestProcessorLaysOut lays the processor out and checks the drawing is
// the design, transistor by transistor.
func TestProcessorLaysOut(t *testing.T) {
	p := BuildProcessor()
	layout, err := p.LayOut()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d gates, %d signals, %d tracks, %v", len(p.gates), len(p.signals), layout.Tracks, layout.Bounds)
	document := layout.Document()
	if _, err := p.Verify(document, layout); err != nil {
		t.Fatal(err)
	}
}
