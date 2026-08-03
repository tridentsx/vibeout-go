package controller

// Editor is the behavior behind the WipEout-style mapping screen: vertical
// row selection, confirm to listen, then one face/shoulder press to assign.
type Editor struct {
	Mapping Mapping
	Cursor  int
	Waiting bool
}

func NewEditor(mapping Mapping) Editor { return Editor{Mapping: mapping} }

func (e *Editor) Move(delta int) {
	if e.Waiting {
		return
	}
	rows := RemappableActions()
	e.Cursor = (e.Cursor + delta%len(rows) + len(rows)) % len(rows)
}

func (e *Editor) Selected() Action { return RemappableActions()[e.Cursor] }
func (e *Editor) BeginAssign()     { e.Waiting = true }
func (e *Editor) Cancel()          { e.Waiting = false }

// Capture ignores invalid controls while listening, just like the original
// UI. It returns true only when an assignment was accepted.
func (e *Editor) Capture(button Button) bool {
	if !e.Waiting || e.Mapping.Assign(e.Selected(), button) != nil {
		return false
	}
	e.Waiting = false
	return true
}

type Row struct {
	Action   Action
	Label    string
	Button   Button
	Selected bool
	Waiting  bool
}

// Rows is renderer-agnostic view data for reproducing the controls screen.
func (e Editor) Rows() []Row {
	actions := RemappableActions()
	rows := make([]Row, len(actions))
	for i, action := range actions {
		info, _ := Info(action)
		rows[i] = Row{action, info.Label, e.Mapping.Button(action), i == e.Cursor, i == e.Cursor && e.Waiting}
	}
	return rows
}
