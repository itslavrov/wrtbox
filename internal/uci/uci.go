// Package uci models OpenWrt UCI configuration files and renders them in
// the canonical `/sbin/uci export` text format (tab indent, single-quoted
// values, options before list entries).
package uci

import (
	"fmt"
	"io"
	"strings"
)

// Package is the top-level object: a single uci-export file is always one
// package with zero or more sections.
type Package struct {
	Name     string
	Sections []Section
}

// Section is one `config <type> [name]` block.
type Section struct {
	Type  string
	Name  string // optional anonymous
	Items []Item
}

// Item is either an Option (scalar) or a List (repeated values). Options
// render before lists within a section to match `uci export` output.
type Item struct {
	Kind   ItemKind
	Key    string
	Value  string
	Values []string
}

// ItemKind discriminates Option vs List.
type ItemKind int

const (
	// Option is a scalar key=value line.
	Option ItemKind = iota
	// List is a repeated-value key.
	List
)

// Opt constructs a scalar option item.
func Opt(key, value string) Item { return Item{Kind: Option, Key: key, Value: value} }

// Lst constructs a list item with one or more values.
func Lst(key string, values ...string) Item { return Item{Kind: List, Key: key, Values: values} }

// Render writes p to w in canonical UCI export format.
func Render(w io.Writer, p Package) error {
	var b strings.Builder
	b.WriteString("package ")
	b.WriteString(p.Name)
	b.WriteString("\n")
	for _, s := range p.Sections {
		b.WriteString("\n")
		b.WriteString("config ")
		b.WriteString(s.Type)
		if s.Name != "" {
			b.WriteString(" '")
			b.WriteString(s.Name)
			b.WriteString("'")
		}
		b.WriteString("\n")
		for _, it := range s.Items {
			switch it.Kind {
			case Option:
				fmt.Fprintf(&b, "\toption %s '%s'\n", it.Key, escape(it.Value))
			case List:
				for _, v := range it.Values {
					fmt.Fprintf(&b, "\tlist %s '%s'\n", it.Key, escape(v))
				}
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// escape replaces single quotes in UCI values using the standard
// close-open-backslash trick: foo'bar  →  foo'\”bar.
func escape(s string) string {
	if !strings.ContainsRune(s, '\'') {
		return s
	}
	return strings.ReplaceAll(s, "'", `'\''`)
}
