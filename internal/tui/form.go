package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldSelect
)

type field struct {
	name    string
	kind    fieldKind
	input   textinput.Model
	options []string // fieldSelect: the labels to choose from
	sel     int
}

func textField(name, value string) field {
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(value)
	in.CharLimit = 512
	in.Width = 50
	return field{name: name, kind: fieldText, input: in}
}

func selectField(name string, options []string, current string) field {
	f := field{name: name, kind: fieldSelect, options: options}
	for i, o := range options {
		if o == current {
			f.sel = i
		}
	}
	return f
}

func (f *field) value() string {
	if f.kind == fieldSelect {
		if len(f.options) == 0 {
			return ""
		}
		return f.options[f.sel]
	}
	return strings.TrimSpace(f.input.Value())
}

// form is the add/edit dialog. target is the label being edited (empty when adding).
type form struct {
	title  string
	fields []field
	focus  int
	target string
	err    string
}

func newForm(title, target string, fields ...field) *form {
	f := &form{title: title, target: target, fields: fields}
	f.setFocus(0)
	return f
}

func (f *form) setFocus(i int) {
	f.focus = i
	for j := range f.fields {
		if f.fields[j].kind != fieldText {
			continue
		}
		if j == i {
			f.fields[j].input.Focus()
		} else {
			f.fields[j].input.Blur()
		}
	}
}

func (f *form) get(name string) string {
	for i := range f.fields {
		if f.fields[i].name == name {
			return f.fields[i].value()
		}
	}
	return ""
}

func (f *form) last() bool { return f.focus == len(f.fields)-1 }

// key handles navigation and editing keys. It reports whether the form should be submitted.
func (f *form) key(msg tea.KeyMsg) (submit bool) {
	cur := &f.fields[f.focus]
	switch msg.String() {
	case "tab", "down":
		f.setFocus((f.focus + 1) % len(f.fields))
	case "shift+tab", "up":
		f.setFocus((f.focus + len(f.fields) - 1) % len(f.fields))
	case "ctrl+s":
		return true
	case "enter":
		if f.last() {
			return true
		}
		f.setFocus(f.focus + 1)
	case "left", "right":
		if cur.kind == fieldSelect && len(cur.options) > 0 {
			d := 1
			if msg.String() == "left" {
				d = len(cur.options) - 1
			}
			cur.sel = (cur.sel + d) % len(cur.options)
		} else if cur.kind == fieldText {
			cur.input, _ = cur.input.Update(msg)
		}
	default:
		if cur.kind == fieldText {
			cur.input, _ = cur.input.Update(msg)
		}
	}
	return false
}

// set replaces the value of a text field.
func (f *form) set(name, value string) {
	for i := range f.fields {
		if f.fields[i].name == name && f.fields[i].kind == fieldText {
			f.fields[i].input.SetValue(value)
			f.fields[i].input.CursorEnd()
		}
	}
}
