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
	uiSetWatch // prompt: set a watched bus
	uiSetWord  // prompt: set a memory word (target "device:index")
)

// place says where a binding's button goes.
type place int

const (
	noButton   place = iota
	topBar           // file and history, right of the top bar
	toolCol          // edit mode, left column: tools
	editCol          // edit mode, left column: selection actions
	simCol           // simulate mode, left column
	compCol          // edit mode, left column: components
	modeToggle       // the edit/simulate switch
)

// binding maps a key with modifiers, in some modes, to an action, and
// describes its button. Every key and every button comes from this one
// table, so they cannot drift apart.
type binding struct {
	key       ebiten.Key
	keyName   string // as shown on the button
	ctrl      bool
	shift     bool
	editOnly  bool
	simOnly   bool
	action    editor.Action
	ui        uiAction
	onRelease bool // handled by the ui on key release (space)

	place place
	label string
}

var keymap = []binding{
	{key: ebiten.KeyE, keyName: "e", action: editor.ActToggleSimulate, place: modeToggle},

	{key: ebiten.KeyD, keyName: "d", editOnly: true, action: editor.ActPencil, place: toolCol, label: "Draw"},
	{key: ebiten.KeyS, keyName: "s", editOnly: true, action: editor.ActSelect, place: toolCol, label: "Select"},
	{key: ebiten.KeyN, keyName: "n", editOnly: true, action: editor.ActLabel, place: toolCol, label: "Label"},

	{key: ebiten.KeyC, keyName: "c", editOnly: true, action: editor.ActCopy, place: editCol, label: "Copy"},
	{key: ebiten.KeyX, keyName: "x", editOnly: true, action: editor.ActCut, place: editCol, label: "Cut"},
	{key: ebiten.KeyV, keyName: "v", editOnly: true, action: editor.ActPaste, place: editCol, label: "Paste"},
	{key: ebiten.KeyBackspace, keyName: "bksp", editOnly: true, action: editor.ActDelete, place: editCol, label: "Delete"},
	{key: ebiten.KeyDelete, editOnly: true, action: editor.ActDelete},
	{key: ebiten.KeyM, keyName: "m", editOnly: true, action: editor.ActMirror, place: editCol, label: "Mirror"},
	{key: ebiten.KeyR, keyName: "r", editOnly: true, action: editor.ActRotate, place: editCol, label: "Rotate"},

	{key: ebiten.KeyK, keyName: "K", shift: true, editOnly: true, action: editor.ActExplode, place: compCol, label: "Explode"},
	{key: ebiten.KeyK, keyName: "k", editOnly: true, action: editor.ActMakeComponent, place: compCol, label: "Component"},
	{key: ebiten.KeyF2, keyName: "F2", editOnly: true, action: editor.ActRename, place: compCol, label: "Rename"},

	{key: ebiten.KeySpace, keyName: "space", simOnly: true, action: editor.ActRunPause, onRelease: true, place: simCol, label: "Run"},
	{key: ebiten.KeyT, keyName: "t", simOnly: true, action: editor.ActTick, place: simCol, label: "Tick"},
	{key: ebiten.KeyH, keyName: "h", simOnly: true, action: editor.ActHalfTick, place: simCol, label: "Half tick"},
	{key: ebiten.KeyPeriod, keyName: ".", simOnly: true, action: editor.ActStep, place: simCol, label: "Step"},
	{key: ebiten.KeyR, keyName: "r", simOnly: true, action: editor.ActResetSim, place: simCol, label: "Reset"},
	{key: ebiten.KeyBracketLeft, keyName: "[", simOnly: true, action: editor.ActSlower, place: simCol, label: "Slower"},
	{key: ebiten.KeyBracketRight, keyName: "]", simOnly: true, action: editor.ActFaster, place: simCol, label: "Faster"},

	{key: ebiten.KeyEscape, action: editor.ActEscape},

	{key: ebiten.KeyZ, ctrl: true, shift: true, action: editor.ActRedo},
	{key: ebiten.KeyZ, keyName: "^Z", ctrl: true, action: editor.ActUndo, place: topBar, label: "Undo"},
	{key: ebiten.KeyY, keyName: "^Y", ctrl: true, action: editor.ActRedo, place: topBar, label: "Redo"},
	{key: ebiten.KeyO, keyName: "^O", ctrl: true, ui: uiOpen, place: topBar, label: "Open"},
	{key: ebiten.KeyS, ctrl: true, shift: true, ui: uiSaveAs},
	{key: ebiten.KeyS, keyName: "^S", ctrl: true, ui: uiSave, place: topBar, label: "Save"},

	{key: ebiten.KeyF, ui: uiFit},
	{key: ebiten.KeyB, ui: uiFilter},
	{key: ebiten.KeyF12, ui: uiScreenshot},
}

func (b binding) applies(mode editor.Mode) bool {
	return !(b.editOnly && mode != editor.EditMode || b.simOnly && mode != editor.SimulateMode)
}

func lookupKey(k ebiten.Key, m editor.Mods, mode editor.Mode) (binding, bool) {
	for _, b := range keymap {
		if b.key != k || b.ctrl != m.Ctrl || b.shift && !m.Shift || b.onRelease || !b.applies(mode) {
			continue
		}
		return b, true
	}
	return binding{}, false
}

// buttons returns the bindings with a button in the given place, in table order.
func buttonsAt(p place, mode editor.Mode) []binding {
	var bs []binding
	for _, b := range keymap {
		if b.place == p && b.applies(mode) {
			bs = append(bs, b)
		}
	}
	return bs
}
