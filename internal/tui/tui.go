// Package tui is the interactive terminal UI: pick the identity for the
// current repository and manage names, emails, keys and identities.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"git-id/internal/repo"
	"git-id/internal/store"
)

type tab int

const (
	tabIdentities tab = iota
	tabNames
	tabEmails
	tabKeys
	numTabs
)

var tabTitles = [numTabs]string{"Identities", "Names", "Emails", "Keys"}

func (t tab) kind() store.Kind {
	switch t {
	case tabNames:
		return store.KindName
	case tabEmails:
		return store.KindEmail
	}
	return store.KindKey
}

func (t tab) noun() string {
	return [numTabs]string{"identity", "name", "email", "key"}[t]
}

type mode int

const (
	modeList mode = iota
	modeForm
	modeConfirm
	modePicker
	modeFilter
)

type item struct{ label, desc string }

type confirmation struct {
	prompt string
	action func(*store.Book) error
	done   string
}

var (
	activeTab = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	idleTab   = lipgloss.NewStyle().Padding(0, 1)
	cursorSty = lipgloss.NewStyle().Bold(true)
	dimSty    = lipgloss.NewStyle().Faint(true)
	keySty    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")) // a key you can press
	errSty    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	okSty     = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnSty   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

// Model is the bubbletea model.
type Model struct {
	path string // address book file
	dir  string // directory whose repository `use` applies to ("" = cwd)
	book *store.Book

	inRepo    bool
	repoLabel string // identity of the current repository, if any

	tab      tab
	cursor   [numTabs]int
	mode     mode
	form     *form
	confirm  *confirmation
	picker   *picker
	filter   string          // narrows the list of the current tab (case-insensitive substring)
	filterIn textinput.Model // the input behind filter while it is being typed
	sshDir   string          // where the key picker looks (default ~/.ssh)

	status    string
	statusErr bool
	warn      string // shown below the status in yellow, e.g. $GIT_SSH_COMMAND overriding the key
	quitting  bool
}

// New loads the address book at path. dir is the directory used to find the
// current repository; empty means the working directory.
func New(path, dir string) (*Model, error) {
	b, err := store.Load(path)
	if err != nil {
		return nil, err
	}
	m := &Model{path: path, dir: dir, book: b}
	if home, err := os.UserHomeDir(); err == nil {
		m.sshDir = filepath.Join(home, ".ssh")
	}
	m.refreshRepo()
	return m, nil
}

// Run starts the TUI on the terminal.
func Run(path string) error {
	m, err := New(path, "")
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m *Model) refreshRepo() {
	label, _, err := repo.Identity(m.dir)
	m.inRepo = err == nil
	m.repoLabel = label
}

func (m *Model) Init() tea.Cmd { return nil }

// items lists the entries of the current tab that match the filter.
func (m *Model) items() []item {
	all := m.allItems()
	if m.filter == "" {
		return all
	}
	needle := strings.ToLower(m.filter)
	var out []item
	for _, it := range all {
		if strings.Contains(strings.ToLower(it.label+" "+it.desc), needle) {
			out = append(out, it)
		}
	}
	return out
}

// allItems lists every entry of the current tab.
func (m *Model) allItems() []item {
	b := m.book
	var out []item
	switch m.tab {
	case tabIdentities:
		for _, l := range b.IdentityLabels() {
			id := b.Identities[l]
			desc := "(broken reference)"
			if r, err := b.Resolve(l); err == nil {
				desc = fmt.Sprintf("%s <%s>  key:%s", r.Name, r.Email, id.Key)
			}
			out = append(out, item{l, desc})
		}
	case tabNames:
		for _, l := range b.Labels(store.KindName) {
			out = append(out, item{l, b.Names[l]})
		}
	case tabEmails:
		for _, l := range b.Labels(store.KindEmail) {
			out = append(out, item{l, b.Emails[l]})
		}
	case tabKeys:
		for _, l := range b.Labels(store.KindKey) {
			k := b.Keys[l]
			desc := k.Private
			if k.Public != "" {
				desc += "  (+pub)"
			}
			out = append(out, item{l, desc})
		}
	}
	return out
}

func (m *Model) current() (item, bool) {
	items := m.items()
	c := m.cursor[m.tab]
	if c < 0 || c >= len(items) {
		return item{}, false
	}
	return items[c], true
}

func (m *Model) clampCursor() {
	n := len(m.items())
	if m.cursor[m.tab] >= n {
		m.cursor[m.tab] = n - 1
	}
	if m.cursor[m.tab] < 0 {
		m.cursor[m.tab] = 0
	}
}

func (m *Model) setStatus(msg string, isErr bool) { m.status, m.statusErr, m.warn = msg, isErr, "" }

// commit reloads the book from disk, applies fn and saves it, so edits made
// by the CLI in the meantime are not overwritten.
func (m *Model) commit(fn func(*store.Book) error) error {
	b, err := store.Load(m.path)
	if err != nil {
		return err
	}
	if err := fn(b); err != nil {
		return err
	}
	if err := store.Save(m.path, b); err != nil {
		return err
	}
	m.book = b
	m.clampCursor()
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	switch m.mode {
	case modeForm:
		m.updateForm(key)
	case modeConfirm:
		m.updateConfirm(key)
	case modePicker:
		m.updatePicker(key)
	case modeFilter:
		m.updateFilter(key)
	default:
		return m.updateList(key)
	}
	return m, nil
}

func (m *Model) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.setStatus("", false)
	switch key.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "left", "h", "shift+tab":
		m.tab = (m.tab + numTabs - 1) % numTabs
		m.filter = ""
	case "right", "l", "tab":
		m.tab = (m.tab + 1) % numTabs
		m.filter = ""
	case "/":
		m.openFilter()
	case "esc":
		m.filter = ""
		m.clampCursor()
	case "up", "k":
		if m.cursor[m.tab] > 0 {
			m.cursor[m.tab]--
		}
	case "down", "j":
		if m.cursor[m.tab] < len(m.items())-1 {
			m.cursor[m.tab]++
		}
	case "a":
		m.openForm(true)
	case "e":
		m.openForm(false)
	case "d":
		m.askDelete()
	case "enter":
		m.useCurrent()
	}
	return m, nil
}

func (m *Model) useCurrent() {
	if m.tab != tabIdentities {
		return
	}
	it, ok := m.current()
	if !ok {
		return
	}
	if !m.inRepo {
		m.setStatus("not inside a git repository", true)
		return
	}
	top, err := repo.Use(m.path, m.book, it.label, m.dir, false)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.refreshRepo()
	m.setStatus(fmt.Sprintf("using identity %s in %s", it.label, top), false)
	if repo.SSHOverridden() {
		m.warn = repo.SSHOverrideWarning
	}
}

func (m *Model) askDelete() {
	it, ok := m.current()
	if !ok {
		return
	}
	noun := m.tab.noun()
	if m.tab == tabIdentities {
		m.confirm = &confirmation{
			prompt: fmt.Sprintf("Delete identity %q? Names, emails and keys stay. [y/N]", it.label),
			action: func(b *store.Book) error { return b.RemoveIdentity(it.label) },
			done:   fmt.Sprintf("deleted identity %s", it.label),
		}
	} else {
		kind := m.tab.kind()
		users := m.book.UsedBy(kind, it.label)
		c := &confirmation{
			prompt: fmt.Sprintf("Delete %s %q? [y/N]", noun, it.label),
			action: func(b *store.Book) error { _, err := b.Remove(kind, it.label, len(users) > 0); return err },
			done:   fmt.Sprintf("deleted %s %s", noun, it.label),
		}
		if len(users) > 0 {
			c.prompt = fmt.Sprintf("%s %q is used by identities %s. Delete it and those identities? [y/N]",
				strings.ToUpper(noun[:1])+noun[1:], it.label, strings.Join(users, ", "))
			c.done = fmt.Sprintf("deleted %s %s and identities %s", noun, it.label, strings.Join(users, ", "))
		}
		m.confirm = c
	}
	m.mode = modeConfirm
}

func (m *Model) updateConfirm(key tea.KeyMsg) {
	c := m.confirm
	m.mode, m.confirm = modeList, nil
	if key.String() != "y" && key.String() != "Y" {
		m.setStatus("cancelled", false)
		return
	}
	if err := m.commit(c.action); err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.setStatus(c.done, false)
}

func (m *Model) openForm(add bool) {
	b := m.book
	t := m.tab
	var cur item
	if !add {
		it, ok := m.current()
		if !ok {
			return
		}
		cur = it
	}
	title := fmt.Sprintf("Add %s", t.noun())
	target := ""
	if !add {
		title = fmt.Sprintf("Edit %s %s", t.noun(), cur.label)
		target = cur.label
	}
	// The label comes first; when editing it can be changed to rename, and the
	// focus starts on the first field after it.
	fields := []field{textField("label", cur.label)}
	switch t {
	case tabIdentities:
		names, emails, keys := b.Labels(store.KindName), b.Labels(store.KindEmail), b.Labels(store.KindKey)
		if len(names) == 0 || len(emails) == 0 || len(keys) == 0 {
			m.setStatus("add at least one name, email and key first", true)
			return
		}
		id := b.Identities[target]
		fields = append(fields,
			selectField("name", names, id.Name),
			selectField("email", emails, id.Email),
			selectField("key", keys, id.Key))
	case tabNames:
		fields = append(fields, textField("value", b.Names[target]))
	case tabEmails:
		fields = append(fields, textField("value", b.Emails[target]))
	case tabKeys:
		k := b.Keys[target]
		fields = append(fields, textField("private", k.Private), textField("public", k.Public))
	}
	m.form = newForm(title, target, fields...)
	if !add {
		m.form.setFocus(1)
	}
	m.mode = modeForm
}

func (m *Model) updateForm(key tea.KeyMsg) {
	if key.String() == "ctrl+o" && m.tab == tabKeys {
		m.openPicker()
		return
	}
	if key.String() == "esc" {
		m.mode, m.form = modeList, nil
		m.setStatus("cancelled", false)
		return
	}
	if !m.form.key(key) {
		return
	}
	old := m.form.target
	label, err := m.submit()
	if err != nil {
		m.form.err = err.Error()
		return
	}
	verb, renamed := "added", old != "" && label != old
	if old != "" {
		verb = "saved"
	}
	m.mode, m.form = modeList, nil
	for i, it := range m.items() {
		if it.label == label {
			m.cursor[m.tab] = i
		}
	}
	msg := fmt.Sprintf("%s %s %s", verb, m.tab.noun(), label)
	if renamed {
		msg += " (renamed from " + old + ")"
	}
	// Repositories that use a renamed identity point at the old label.
	if renamed && m.tab == tabIdentities {
		if _, err := repo.Relabel(m.path, old, label); err != nil {
			m.setStatus(msg+"; updating repositories failed: "+err.Error(), true)
			return
		}
		m.refreshRepo()
	}
	m.setStatus(msg, false)
}

// submit applies the open form to the book and returns the affected label. An
// edit with a changed label renames first, so identities follow.
func (m *Model) submit() (string, error) {
	f := m.form
	adding := f.target == ""
	label := f.get("label")
	return label, m.commit(func(b *store.Book) error {
		if !adding && label != f.target {
			var err error
			if m.tab == tabIdentities {
				err = b.RenameIdentity(f.target, label)
			} else {
				err = b.RenameDatapoint(m.tab.kind(), f.target, label)
			}
			if err != nil {
				return err
			}
		}
		switch m.tab {
		case tabIdentities:
			id := store.Identity{Name: f.get("name"), Email: f.get("email"), Key: f.get("key")}
			if adding {
				return b.AddIdentity(label, id)
			}
			return b.SetIdentity(label, id)
		case tabNames:
			if adding {
				return b.AddName(label, f.get("value"))
			}
			return b.SetName(label, f.get("value"))
		case tabEmails:
			if adding {
				return b.AddEmail(label, f.get("value"))
			}
			return b.SetEmail(label, f.get("value"))
		}
		priv, err := store.ResolveKeyPath(f.get("private"))
		if err != nil {
			return err
		}
		k := store.Key{Private: priv}
		if pub := f.get("public"); pub != "" {
			if k.Public, err = store.ResolveKeyPath(pub); err != nil {
				return err
			}
		}
		if adding {
			return b.AddKey(label, k)
		}
		return b.SetKey(label, k)
	})
}

func (m *Model) View() string {
	if m.quitting {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(cursorSty.Render("git-id") + "  ")
	switch {
	case !m.inRepo:
		sb.WriteString(dimSty.Render("not in a git repository"))
	case m.repoLabel == "":
		sb.WriteString(dimSty.Render("this repository has no identity"))
	default:
		sb.WriteString(dimSty.Render("this repository uses ") + m.repoLabel)
	}
	sb.WriteString("\n\n")
	for t := tab(0); t < numTabs; t++ {
		if t == m.tab {
			sb.WriteString(activeTab.Render(tabTitles[t]))
		} else {
			sb.WriteString(idleTab.Render(tabTitles[t]))
		}
	}
	sb.WriteString("\n\n")
	switch {
	case m.mode == modeFilter:
		sb.WriteString(keySty.Render("/") + " " + m.filterIn.View() + "\n\n")
	case m.filter != "" && m.mode == modeList:
		sb.WriteString(dimSty.Render("filter: ") + m.filter + "\n\n")
	}

	switch m.mode {
	case modeForm:
		sb.WriteString(m.viewForm())
	case modeConfirm:
		sb.WriteString(strings.Replace(m.confirm.prompt, "[y/N]", "["+keySty.Render("y")+"/N]", 1) + "\n")
	case modePicker:
		sb.WriteString(m.viewPicker())
	default:
		sb.WriteString(m.viewList())
	}

	sb.WriteString("\n")
	if m.status != "" {
		sty := okSty
		if m.statusErr {
			sty = errSty
		}
		sb.WriteString(sty.Render(m.status) + "\n")
	}
	if m.warn != "" {
		sb.WriteString(warnSty.Render(m.warn) + "\n")
	}
	sb.WriteString(renderHints(m.hints()) + "\n")
	return sb.String()
}

func (m *Model) viewList() string {
	items := m.items()
	if len(items) == 0 && m.filter != "" {
		return dimSty.Render(fmt.Sprintf("(no %s match %q)", m.tab.plural(), m.filter)) + "\n"
	}
	if len(items) == 0 {
		return dimSty.Render(fmt.Sprintf("(no %s, press ", m.tab.plural())) + keySty.Render("a") + dimSty.Render(" to add one)") + "\n"
	}
	width := 0
	for _, it := range items {
		if len(it.label) > width {
			width = len(it.label)
		}
	}
	var sb strings.Builder
	for i, it := range items {
		mark := " "
		if m.tab == tabIdentities && it.label == m.repoLabel {
			mark = "*"
		}
		line := fmt.Sprintf("%s %-*s  %s", mark, width, it.label, it.desc)
		if i == m.cursor[m.tab] {
			sb.WriteString(cursorSty.Render("> "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	return sb.String()
}

func (m *Model) viewForm() string {
	f := m.form
	var sb strings.Builder
	sb.WriteString(cursorSty.Render(f.title) + "\n\n")
	for i := range f.fields {
		fl := &f.fields[i]
		marker := "  "
		if i == f.focus {
			marker = "> "
		}
		var val string
		if fl.kind == fieldSelect {
			val = "< " + fl.value() + " >"
		} else {
			val = fl.input.View()
		}
		sb.WriteString(fmt.Sprintf("%s%-8s %s\n", marker, fl.name+":", val))
	}
	if f.err != "" {
		sb.WriteString("\n" + errSty.Render(f.err) + "\n")
	}
	return sb.String()
}

func (m *Model) hints() []hint {
	switch m.mode {
	case modeForm:
		choose := hint{"←/→", "choose"}
		if m.tab == tabKeys {
			choose = hint{"ctrl+o", "pick a key from ~/.ssh"}
		}
		return []hint{{"tab/shift+tab", "field"}, choose, {"enter", "next/save"}, {"ctrl+s", "save"}, {"esc", "cancel"}}
	case modePicker:
		return []hint{{"↑/↓", "move"}, {"enter", "choose"}, {"esc", "back to the form"}}
	case modeFilter:
		return []hint{{"enter", "keep filter"}, {"esc", "clear filter"}}
	case modeConfirm:
		return []hint{{"y", "confirm"}, {"any other key", "cancels"}}
	}
	h := []hint{{"←/→", "tab"}, {"↑/↓", "move"}, {"/", "filter"}}
	if m.filter != "" {
		h = append(h, hint{"esc", "clear filter"})
	}
	if m.tab == tabIdentities {
		h = append(h, hint{"enter", "use here"})
	}
	return append(h, hint{"a", "add"}, hint{"e", "edit"}, hint{"d", "delete"}, hint{"q", "quit"})
}

func (m *Model) openPicker() {
	p := &picker{dir: m.sshDir}
	keys, err := discoverKeys(m.sshDir)
	if err != nil {
		p.err = err.Error()
	}
	p.keys = keys
	m.picker = p
	m.mode = modePicker
}

func (m *Model) updatePicker(key tea.KeyMsg) {
	p := m.picker
	switch key.String() {
	case "esc":
		m.mode, m.picker = modeForm, nil
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
	case "down", "j":
		if p.cursor < len(p.keys)-1 {
			p.cursor++
		}
	case "enter":
		if len(p.keys) == 0 {
			return
		}
		k := p.keys[p.cursor]
		m.form.set("private", k.Path)
		m.form.set("public", k.Public)
		if m.form.target == "" && m.form.get("label") == "" {
			if l := suggestLabel(k.Name); l != "" && !m.book.HasKey(l) {
				m.form.set("label", l)
			}
		}
		m.mode, m.picker = modeForm, nil
	}
}

// keyLabelsFor returns the labels under which the address book already holds
// the private key at path.
func (m *Model) keyLabelsFor(path string) []string {
	var labels []string
	for _, l := range m.book.Labels(store.KindKey) {
		if m.book.Keys[l].Private == path {
			labels = append(labels, l)
		}
	}
	return labels
}

func (m *Model) viewPicker() string {
	p := m.picker
	var sb strings.Builder
	sb.WriteString(cursorSty.Render("Choose a private key from "+p.dir) + "\n\n")
	if p.err != "" {
		sb.WriteString(errSty.Render(p.err) + "\n")
	} else if len(p.keys) == 0 {
		sb.WriteString(dimSty.Render("no private keys found there") + "\n")
	}
	width := 0
	for _, k := range p.keys {
		if len(k.Name) > width {
			width = len(k.Name)
		}
	}
	for i, k := range p.keys {
		line := fmt.Sprintf("%-*s  %s", width, k.Name, k.Fingerprint)
		if k.Public == "" {
			line = fmt.Sprintf("%-*s  (no .pub file)", width, k.Name)
		}
		if k.Encrypted {
			line += "  (passphrase)"
		}
		if labels := m.keyLabelsFor(k.Path); len(labels) > 0 {
			line += "  [in address book: " + strings.Join(labels, ", ") + "]"
		}
		if i == p.cursor {
			sb.WriteString(cursorSty.Render("> "+line) + "\n")
		} else {
			sb.WriteString("  " + line + "\n")
		}
	}
	return sb.String()
}

// hint is one key the user can press and what it does.
type hint struct{ key, desc string }

// renderHints draws "key description" pairs with the key highlighted.
func renderHints(hs []hint) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		parts[i] = keySty.Render(h.key) + " " + dimSty.Render(h.desc)
	}
	return strings.Join(parts, "  ")
}

func (t tab) plural() string {
	if t == tabIdentities {
		return "identities"
	}
	return t.noun() + "s"
}

func (m *Model) openFilter() {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 80
	in.Width = 40
	in.SetValue(m.filter)
	in.CursorEnd()
	in.Focus()
	m.filterIn = in
	m.mode = modeFilter
}

// updateFilter narrows the list while typing; enter keeps the filter, esc drops it.
func (m *Model) updateFilter(key tea.KeyMsg) {
	switch key.String() {
	case "esc":
		m.filter = ""
		m.mode = modeList
	case "enter":
		m.mode = modeList
	default:
		m.filterIn, _ = m.filterIn.Update(key)
		m.filter = m.filterIn.Value()
		m.cursor[m.tab] = 0
	}
	m.clampCursor()
}
