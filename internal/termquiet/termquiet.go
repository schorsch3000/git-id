// Package termquiet keeps git-id from querying the terminal at startup.
//
// bubbletea (v1) calls lipgloss.HasDarkBackground() in an init function. That
// makes every process that merely links it, "git id --version" and the shell
// hook's "git id current -q" included, write an OSC 11 background-colour query
// to the terminal and then wait for the answer; terminals that do not answer
// (some multiplexers, ssh chains, dumb terminals) stall each command until a
// timeout, and answers can leak into the shell's input.
//
// git-id uses fixed colours and never needs the answer, so this package states
// the background explicitly, which makes lipgloss skip the query. It must be
// initialised before bubbletea: Go initialises packages in import-path order
// once their dependencies are ready, and "git-id/..." sorts before
// "github.com/...", so main imports it for its side effect. e2e/startup_test.go
// guards that this keeps working.
package termquiet

import "github.com/charmbracelet/lipgloss"

func init() { lipgloss.SetHasDarkBackground(true) }
