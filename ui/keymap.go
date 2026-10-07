package ui

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Castux/mipsim2/editor"
)

type uiAction int

const (
	uiNone uiAction = iota // an editor action
	uiSave
	uiFit
	uiFilter
	uiScreenshot
)

// binding maps a key with modifiers, in some modes, to an action. All keys
// live in this one table.
type binding struct {
	key      ebiten.Key
	ctrl     bool
	shift    bool
	editOnly bool
	simOnly  bool
	action   editor.Action
	ui       uiAction
	help     string
}

var keymap = []binding{
	{key: ebiten.KeyD, action: editor.ActPencil, help: "d pencil (alt: straight line)"},
	{key: ebiten.KeyE, action: editor.ActToggleSimulate, help: "e simulate"},
	{key: ebiten.KeyT, simOnly: true, action: editor.ActTick, help: "t tick"},
	{key: ebiten.KeyH, simOnly: true, action: editor.ActHalfTick, help: "h half tick"},
	{key: ebiten.KeyR, simOnly: true, action: editor.ActResetSim, help: "r reset"},
	{key: ebiten.KeyZ, ctrl: true, shift: true, action: editor.ActRedo, help: ""},
	{key: ebiten.KeyZ, ctrl: true, action: editor.ActUndo, help: "ctrl+z undo"},
	{key: ebiten.KeyY, ctrl: true, action: editor.ActRedo, help: "ctrl+shift+z redo"},
	{key: ebiten.KeyS, ctrl: true, ui: uiSave, help: "ctrl+s save"},
	{key: ebiten.KeyF, ui: uiFit, help: "f fit"},
	{key: ebiten.KeyB, ui: uiFilter, help: "b zoom-out filter"},
	{key: ebiten.KeyF12, ui: uiScreenshot, help: "f12 screenshot"},
}

func lookupKey(k ebiten.Key, m editor.Mods, mode editor.Mode) (binding, bool) {
	for _, b := range keymap {
		if b.key != k || b.ctrl != m.Ctrl || (b.shift && !m.Shift) {
			continue
		}
		if !b.shift && m.Shift && b.ctrl {
			continue
		}
		if b.editOnly && mode != editor.EditMode || b.simOnly && mode != editor.SimulateMode {
			continue
		}
		return b, true
	}
	return binding{}, false
}

func helpLine(mode editor.Mode) string {
	s := "space+drag pan, wheel zoom"
	if mode == editor.SimulateMode {
		s = "left pin high, right pin low, middle release, " + s
	}
	for _, b := range keymap {
		if b.help == "" || b.editOnly && mode != editor.EditMode || b.simOnly && mode != editor.SimulateMode {
			continue
		}
		s += ", " + b.help
	}
	return s
}
