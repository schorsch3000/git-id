// Package store holds the git-id address book: named datapoints (names,
// emails, ssh keys) and identities that reference them by label.
package store

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
)

// Kind selects one of the datapoint collections.
type Kind string

const (
	KindName  Kind = "name"
	KindEmail Kind = "email"
	KindKey   Kind = "key"
)

// Key is an ssh key datapoint.
type Key struct {
	Private string `toml:"private"`
	Public  string `toml:"public,omitempty"`
}

// Identity references one name, one email and one key by label.
type Identity struct {
	Name  string `toml:"name"`
	Email string `toml:"email"`
	Key   string `toml:"key"`
}

// Book is the whole address book.
type Book struct {
	Names      map[string]string   `toml:"names"`
	Emails     map[string]string   `toml:"emails"`
	Keys       map[string]Key      `toml:"keys"`
	Identities map[string]Identity `toml:"identities"`
}

// Resolved is an identity with all references replaced by their values.
type Resolved struct {
	Label string
	Name  string
	Email string
	Key   Key
}

var (
	ErrExists   = errors.New("already exists")
	ErrNotFound = errors.New("not found")
	ErrInUse    = errors.New("still used by identities")

	labelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// New returns an empty book.
func New() *Book {
	b := &Book{}
	b.init()
	return b
}

// init makes all maps non-nil (TOML decoding leaves absent tables nil).
func (b *Book) init() {
	if b.Names == nil {
		b.Names = map[string]string{}
	}
	if b.Emails == nil {
		b.Emails = map[string]string{}
	}
	if b.Keys == nil {
		b.Keys = map[string]Key{}
	}
	if b.Identities == nil {
		b.Identities = map[string]Identity{}
	}
}

// ValidateLabel reports whether s is usable as a label.
func ValidateLabel(s string) error {
	if !labelRe.MatchString(s) {
		return fmt.Errorf("invalid label %q: use letters, digits, '.', '_', '-', starting with a letter or digit", s)
	}
	return nil
}

func (b *Book) has(k Kind, label string) bool {
	switch k {
	case KindName:
		_, ok := b.Names[label]
		return ok
	case KindEmail:
		_, ok := b.Emails[label]
		return ok
	case KindKey:
		_, ok := b.Keys[label]
		return ok
	}
	return false
}

func (b *Book) ref(id Identity, k Kind) string {
	switch k {
	case KindName:
		return id.Name
	case KindEmail:
		return id.Email
	}
	return id.Key
}

// AddName adds a name datapoint.
func (b *Book) AddName(label, value string) error {
	if err := b.checkNew(KindName, label, value); err != nil {
		return err
	}
	b.Names[label] = value
	return nil
}

// AddEmail adds an email datapoint.
func (b *Book) AddEmail(label, value string) error {
	if err := b.checkNew(KindEmail, label, value); err != nil {
		return err
	}
	b.Emails[label] = value
	return nil
}

// AddKey adds an ssh key datapoint.
func (b *Book) AddKey(label string, key Key) error {
	if err := b.checkNew(KindKey, label, key.Private); err != nil {
		return err
	}
	b.Keys[label] = key
	return nil
}

func (b *Book) checkNew(k Kind, label, value string) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("%s %q: empty value", k, label)
	}
	if b.has(k, label) {
		return fmt.Errorf("%s %q: %w", k, label, ErrExists)
	}
	return nil
}

// SetName, SetEmail and SetKey change an existing datapoint. Identities
// reference by label, so every identity sees the change.
func (b *Book) SetName(label, value string) error {
	if !b.has(KindName, label) {
		return fmt.Errorf("name %q: %w", label, ErrNotFound)
	}
	if value == "" {
		return fmt.Errorf("name %q: empty value", label)
	}
	b.Names[label] = value
	return nil
}

func (b *Book) SetEmail(label, value string) error {
	if !b.has(KindEmail, label) {
		return fmt.Errorf("email %q: %w", label, ErrNotFound)
	}
	if value == "" {
		return fmt.Errorf("email %q: empty value", label)
	}
	b.Emails[label] = value
	return nil
}

func (b *Book) SetKey(label string, key Key) error {
	if !b.has(KindKey, label) {
		return fmt.Errorf("key %q: %w", label, ErrNotFound)
	}
	if key.Private == "" {
		return fmt.Errorf("key %q: empty private path", label)
	}
	b.Keys[label] = key
	return nil
}

// UsedBy returns the sorted labels of identities referencing the datapoint.
func (b *Book) UsedBy(k Kind, label string) []string {
	var out []string
	for il, id := range b.Identities {
		if b.ref(id, k) == label {
			out = append(out, il)
		}
	}
	sort.Strings(out)
	return out
}

// Remove deletes a datapoint. If identities still use it, Remove fails with
// ErrInUse unless force is set, in which case those identities are removed
// too. It returns the labels of identities removed.
func (b *Book) Remove(k Kind, label string, force bool) ([]string, error) {
	if !b.has(k, label) {
		return nil, fmt.Errorf("%s %q: %w", k, label, ErrNotFound)
	}
	users := b.UsedBy(k, label)
	if len(users) > 0 && !force {
		return nil, fmt.Errorf("%s %q: %w: %v", k, label, ErrInUse, users)
	}
	for _, u := range users {
		delete(b.Identities, u)
	}
	switch k {
	case KindName:
		delete(b.Names, label)
	case KindEmail:
		delete(b.Emails, label)
	case KindKey:
		delete(b.Keys, label)
	}
	return users, nil
}

// AddIdentity adds an identity; all referenced datapoints must exist.
func (b *Book) AddIdentity(label string, id Identity) error {
	if err := ValidateLabel(label); err != nil {
		return err
	}
	if _, ok := b.Identities[label]; ok {
		return fmt.Errorf("identity %q: %w", label, ErrExists)
	}
	if err := b.checkRefs(id); err != nil {
		return fmt.Errorf("identity %q: %w", label, err)
	}
	b.Identities[label] = id
	return nil
}

// SetIdentity replaces an existing identity.
func (b *Book) SetIdentity(label string, id Identity) error {
	if _, ok := b.Identities[label]; !ok {
		return fmt.Errorf("identity %q: %w", label, ErrNotFound)
	}
	if err := b.checkRefs(id); err != nil {
		return fmt.Errorf("identity %q: %w", label, err)
	}
	b.Identities[label] = id
	return nil
}

// RemoveIdentity deletes an identity. Datapoints stay.
func (b *Book) RemoveIdentity(label string) error {
	if _, ok := b.Identities[label]; !ok {
		return fmt.Errorf("identity %q: %w", label, ErrNotFound)
	}
	delete(b.Identities, label)
	return nil
}

func (b *Book) checkRefs(id Identity) error {
	for _, r := range []struct {
		k     Kind
		label string
	}{{KindName, id.Name}, {KindEmail, id.Email}, {KindKey, id.Key}} {
		if !b.has(r.k, r.label) {
			return fmt.Errorf("%s %q: %w", r.k, r.label, ErrNotFound)
		}
	}
	return nil
}

// Resolve returns the identity with references replaced by values.
func (b *Book) Resolve(label string) (Resolved, error) {
	id, ok := b.Identities[label]
	if !ok {
		return Resolved{}, fmt.Errorf("identity %q: %w", label, ErrNotFound)
	}
	if err := b.checkRefs(id); err != nil {
		return Resolved{}, fmt.Errorf("identity %q: %w", label, err)
	}
	return Resolved{Label: label, Name: b.Names[id.Name], Email: b.Emails[id.Email], Key: b.Keys[id.Key]}, nil
}

// Validate checks the whole book: labels and identity references.
func (b *Book) Validate() error {
	var errs []error
	for l := range b.Names {
		errs = append(errs, ValidateLabel(l))
	}
	for l := range b.Emails {
		errs = append(errs, ValidateLabel(l))
	}
	for l := range b.Keys {
		errs = append(errs, ValidateLabel(l))
	}
	for l, id := range b.Identities {
		errs = append(errs, ValidateLabel(l))
		if err := b.checkRefs(id); err != nil {
			errs = append(errs, fmt.Errorf("identity %q: %w", l, err))
		}
	}
	return errors.Join(errs...)
}

// Labels returns the sorted labels of a datapoint kind.
func (b *Book) Labels(k Kind) []string {
	var out []string
	switch k {
	case KindName:
		for l := range b.Names {
			out = append(out, l)
		}
	case KindEmail:
		for l := range b.Emails {
			out = append(out, l)
		}
	case KindKey:
		for l := range b.Keys {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

// IdentityLabels returns the sorted identity labels.
func (b *Book) IdentityLabels() []string {
	out := make([]string, 0, len(b.Identities))
	for l := range b.Identities {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// RenameDatapoint gives a datapoint a new label; identities that reference it
// follow. The new label must be valid and unused within the kind.
func (b *Book) RenameDatapoint(k Kind, old, new string) error {
	if err := ValidateLabel(new); err != nil {
		return err
	}
	if !b.has(k, old) {
		return fmt.Errorf("%s %q: %w", k, old, ErrNotFound)
	}
	if old == new {
		return nil
	}
	if b.has(k, new) {
		return fmt.Errorf("%s %q: %w", k, new, ErrExists)
	}
	switch k {
	case KindName:
		b.Names[new] = b.Names[old]
		delete(b.Names, old)
	case KindEmail:
		b.Emails[new] = b.Emails[old]
		delete(b.Emails, old)
	case KindKey:
		b.Keys[new] = b.Keys[old]
		delete(b.Keys, old)
	}
	for l, id := range b.Identities {
		switch k {
		case KindName:
			if id.Name == old {
				id.Name = new
			}
		case KindEmail:
			if id.Email == old {
				id.Email = new
			}
		case KindKey:
			if id.Key == old {
				id.Key = new
			}
		}
		b.Identities[l] = id
	}
	return nil
}

// RenameIdentity gives an identity a new label.
func (b *Book) RenameIdentity(old, new string) error {
	if err := ValidateLabel(new); err != nil {
		return err
	}
	id, ok := b.Identities[old]
	if !ok {
		return fmt.Errorf("identity %q: %w", old, ErrNotFound)
	}
	if old == new {
		return nil
	}
	if _, taken := b.Identities[new]; taken {
		return fmt.Errorf("identity %q: %w", new, ErrExists)
	}
	b.Identities[new] = id
	delete(b.Identities, old)
	return nil
}
