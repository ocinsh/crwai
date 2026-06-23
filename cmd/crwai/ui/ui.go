// Package ui is the presentation layer for the crwai CLI. It is the single place
// that decides how things LOOK in the terminal — colors, borders, and the emoji
// icons that label each kind of symbol — so the command files stay logic-only and
// never embed escape codes or layout. Commands hand it plain data (a
// crwai.Signature, an error, a code string) and get back a ready-to-print string.
//
// All styling goes through lipgloss, which degrades gracefully: on a non-TTY or a
// NO_COLOR terminal the styles render as plain text, so piped output stays clean.
package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ocinsh/crwai"
)

// Icons label what each line is at a glance. Symbol kinds get a distinct glyph so
// a function, a method, an interface, and a struct are never confused.
const (
	IconFunc      = "🔧"  // free/top-level function
	IconMethod    = "⚙️" // method bound to a receiver/class
	IconInterface = "🧩"  // interface / protocol / trait
	IconStruct    = "📦"  // struct / class / record
	IconLang      = "🌐"  // a supported language
	IconServer    = "📡"  // MCP server
	IconWrite     = "✏️" // a write/edit
	IconOK        = "✅"  // success
	IconErr       = "❌"  // failure
	IconDoc       = "💬"  // documentation line
)

// Palette — a few semantic colors reused across the CLI.
var (
	accent  = lipgloss.Color("63")  // violet — headings
	good    = lipgloss.Color("42")  // green — success
	bad     = lipgloss.Color("196") // red — errors
	muted   = lipgloss.Color("245") // grey — docs / secondary
	nameCol = lipgloss.Color("81")  // cyan — symbol names

	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	nameStyle    = lipgloss.NewStyle().Bold(true).Foreground(nameCol)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	goodStyle    = lipgloss.NewStyle().Bold(true).Foreground(good)
	badStyle     = lipgloss.NewStyle().Bold(true).Foreground(bad)
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).Padding(0, 1)
)

// KindIcon returns the emoji that labels a symbol of the given kind.
func KindIcon(k crwai.SymbolKind) string {
	switch k {
	case crwai.KindMethod:
		return IconMethod
	case crwai.KindInterface:
		return IconInterface
	case crwai.KindStruct:
		return IconStruct
	default:
		return IconFunc
	}
}

// Heading renders a bold, icon-prefixed section title.
func Heading(icon, text string) string {
	return headingStyle.Render(icon + "  " + text)
}

// Signature renders one Signature as a labelled line: an icon, the bold name, its
// parameters and return type, and — when present — the first line of its doc shown
// dimmed underneath.
func Signature(icon string, s crwai.Signature) string {
	sig := fmt.Sprintf("%s(%s)", nameStyle.Render(s.Name), strings.Join(s.Params, ", "))
	if s.Returns != "" {
		sig += " " + mutedStyle.Render(s.Returns)
	}
	line := icon + "  " + sig
	if doc := firstLine(s.Doc); doc != "" {
		line += "\n   " + mutedStyle.Render(IconDoc+" "+doc)
	}
	return line
}

// Code wraps a block of source in a rounded, titled box.
func Code(title, body string) string {
	return Heading(IconDoc, title) + "\n" + boxStyle.Render(body)
}

// Lang renders one supported-language row: its icon, name, and extensions.
func Lang(name string, exts []string) string {
	return IconLang + "  " + nameStyle.Render(name) + "  " + mutedStyle.Render(strings.Join(exts, " "))
}

// OK renders a green success line.
func OK(format string, a ...any) string {
	return goodStyle.Render(IconOK + "  " + fmt.Sprintf(format, a...))
}

// Fail renders a red failure line for an error.
func Fail(err error) string {
	return badStyle.Render(IconErr + "  " + err.Error())
}

// firstLine returns the first non-empty, trimmed line of s.
func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}
