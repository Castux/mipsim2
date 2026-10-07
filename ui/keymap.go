package ui

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/Castux/mipsim2/editor"
)

type uiAction int

const (
	uiNone uiAction = iota // an editor action
	uiSave
	uiSaveAs
	uiOpen
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
	{key: ebiten.KeyD, action: editor.ActPencil, help: "d draw (alt: line)"},
	{key: ebiten.KeyS, editOnly: true, action: editor.ActSelect, help: "s select"},
	{key: ebiten.KeyN, editOnly: true, action: editor.ActLabel, help: "n label"},
	{key: ebiten.KeyE, action: editor.ActToggleSimulate, help: "e simulate"},
	{key: ebiten.KeyC, editOnly: true, action: editor.ActCopy, help: "c/x/v clipboard"},
	{key: ebiten.KeyX, editOnly: true, action: editor.ActCut},
	{key: ebiten.KeyV, editOnly: true, action: editor.ActPaste},
	{key: ebiten.KeyBackspace, editOnly: true, action: editor.ActDelete, help: "bksp delete"},
	{key: ebiten.KeyDelete, editOnly: true, action: editor.ActDelete},
	{key: ebiten.KeyM, editOnly: true, action: editor.ActMirror, help: "m/r mirror/rotate"},
	{key: ebiten.KeyR, editOnly: true, action: editor.ActRotate},
	{key: ebiten.KeyEscape, action: editor.ActEscape},
	{key: ebiten.KeyT, simOnly: true, action: editor.ActTick, help: "t tick"},
	{key: ebiten.KeyH, simOnly: true, action: editor.ActHalfTick, help: "h half tick"},
	{key: ebiten.KeyR, simOnly: true, action: editor.ActResetSim, help: "r reset"},
	{key: ebiten.KeyZ, ctrl: true, shift: true, action: editor.ActRedo},
	{key: ebiten.KeyZ, ctrl: true, action: editor.ActUndo, help: "ctrl+z/y undo/redo"},
	{key: ebiten.KeyY, ctrl: true, action: editor.ActRedo},
	{key: ebiten.KeyS, ctrl: true, shift: true, ui: uiSaveAs},
	{key: ebiten.KeyS, ctrl: true, ui: uiSave, help: "ctrl+s/o save/open"},
	{key: ebiten.KeyO, ctrl: true, ui: uiOpen},
	{key: ebiten.KeyF, ui: uiFit, help: "f fit"},
	{key: ebiten.KeyB, ui: uiFilter},
	{key: ebiten.KeyF12, ui: uiScreenshot},
}

func lookupKey(k ebiten.Key, m editor.Mods, mode editor.Mode) (binding, bool) {
	for _, b := range keymap {
		if b.key != k || b.ctrl != m.Ctrl || b.shift && !m.Shift {
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
