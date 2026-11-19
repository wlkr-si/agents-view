package document

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const Version = "<!-- agents-view v0 -->"
const Budget = 1000

// Markdown and the document format use JavaScript's definition of whitespace.
const space = `\t\n\v\f\r\x20\x{A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}`
const lineText = `[^\r\n\x{2028}\x{2029}]`

var (
	headingName         = regexp.MustCompile(`^ {0,3}##[ \t]+(` + lineText + `+?)[` + space + `]*$`)
	headingTail         = regexp.MustCompile(`[` + space + `]+#+$`)
	leading             = regexp.MustCompile(`^(?:[ \t]*\r?\n)*`)
	trailing            = regexp.MustCompile(`(?:\r?\n[ \t]*)+$`)
	pageLine            = regexp.MustCompile(`^ {0,3}[-*+][` + space + `]+\[([^\]]+)\]\((<[^>]+>|(?:[^()` + space + `]|\([^()]*\))+)\)(?:[` + space + `]+—[` + space + `]+(` + lineText + `+))?$`)
	overrideLine        = regexp.MustCompile(`^- Global: "((?:[^"\\]|\\` + lineText + `)+)"[` + space + `]+→ here:[` + space + `]+(` + lineText + `+)$`)
	literalOverrideLine = regexp.MustCompile(`^- Global: "(` + lineText + `+?)"[` + space + `]+→ here:[` + space + `]+(` + lineText + `+)$`)
)

func TrimSpace(text string) string {
	return strings.Trim(text, "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff")
}

func Tokens(text string) int {
	return (characterCount(text) + 3) / 4
}

func characterCount(text string) int {
	length := 0
	for _, char := range text {
		length++
		if char > 0xffff {
			length++ // JavaScript counts UTF-16 code units.
		}
	}
	return length
}

type SectionType string

const (
	Text      SectionType = "text"
	Rules     SectionType = "rules"
	Pages     SectionType = "pages"
	Diagram   SectionType = "diagram"
	Overrides SectionType = "overrides"
)

type Section struct {
	Name               string
	Type               SectionType
	Body               string
	Heading            string
	Start, End         int
	BodyStart, BodyEnd int
	Tokens             int
}

func (section Section) DisplayName() string {
	if section.Heading == "" {
		return "Intro"
	}
	return section.Name
}

type Document struct {
	HasVersion bool
	Sections   []Section
	Tokens     int
}

func Parse(text string) Document {
	lines := strings.SplitAfter(text, "\n")
	offsets := make([]int, len(lines)+1)
	for index, line := range lines {
		offsets[index+1] = offsets[index] + len(line)
	}

	document := Document{Tokens: Tokens(text)}
	start := 0
	if lines[0] == Version || lines[0] == Version+"\n" || lines[0] == Version+"\r\n" {
		document.HasVersion = true
		start = offsets[1]
	}
	name, heading := "", ""
	parseMarkdown(text, func(item markdownItem) {
		if item.heading != 2 || item.depth != 0 || item.start >= len(lines) ||
			item.end > len(lines) || offsets[item.start] < start {
			return
		}
		match := headingName.FindStringSubmatch(lines[item.start])
		if match == nil {
			return
		}
		end := offsets[item.start]
		document.Sections = append(document.Sections, newSection(name, heading, text, start, end))
		name = headingTail.ReplaceAllString(match[1], "")
		start = offsets[item.end]
		heading = text[end:start]
	})
	document.Sections = append(document.Sections, newSection(name, heading, text, start, len(text)))
	return document
}

func bodyBounds(raw string) (int, int) {
	start := leading.FindStringIndex(raw)[1]
	end := len(raw)
	if ending := trailing.FindStringIndex(raw[start:]); ending != nil {
		end = start + ending[0]
	}
	return start, end
}

func newSection(name, heading, text string, start, end int) Section {
	raw := text[start:end]
	first, last := bodyBounds(raw)
	body := raw[first:last]
	return Section{
		Name: name, Heading: heading, Body: body, Type: inferSectionType(name, body),
		Start: start, End: end, BodyStart: start + first, BodyEnd: start + last,
		Tokens: (characterCount(heading) + characterCount(raw) + 3) / 4,
	}
}

func inferSectionType(name, body string) SectionType {
	if name == "Overrides" {
		lines := overrideEntries(body)
		if len(lines) > 0 {
			valid := true
			for _, line := range lines {
				valid = valid && line.matched
			}
			if valid {
				return Overrides
			}
		}
	}
	diagram, rules := false, false
	parseMarkdown(body, func(item markdownItem) {
		diagram = diagram || item.diagram
		rules = rules || item.bullet
	})
	switch {
	case diagram:
		return Diagram
	case PageEntries(body) != nil:
		return Pages
	case rules:
		return Rules
	default:
		return Text
	}
}

func (document Document) FindSection(name string) (*Section, error) {
	var match *Section
	for index := range document.Sections {
		section := &document.Sections[index]
		if section.DisplayName() != name {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf("Ambiguous section: %s", name)
		}
		match = section
	}
	return match, nil
}

type Page struct {
	Name, Href, Note string
	Line             int // Zero-based line within the section body.
}

func PageEntries(body string) []Page {
	var pages []Page
	for index, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if TrimSpace(line) == "" {
			continue
		}
		match := pageLine.FindStringSubmatch(line)
		if match == nil {
			return nil
		}
		pages = append(pages, Page{
			Name: match[1],
			Href: strings.TrimSuffix(strings.TrimPrefix(match[2], "<"), ">"),
			Note: match[3],
			Line: index,
		})
	}
	return pages
}

type Override struct {
	Line, Quote, Replacement string
	Index                    int // Zero-based line within the section body.
	matched                  bool
}

func (document Document) OverrideEntries() ([]Override, error) {
	section, err := document.FindSection("Overrides")
	if err != nil || section == nil {
		return nil, err
	}
	return overrideEntries(section.Body), nil
}

func overrideEntries(body string) []Override {
	var entries []Override
	for index, line := range strings.Split(body, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if TrimSpace(line) == "" {
			continue
		}
		entry := Override{Line: line, Index: index}
		match := overrideLine.FindStringSubmatch(line)
		if match == nil {
			match = literalOverrideLine.FindStringSubmatch(line)
		}
		if match != nil {
			entry.matched = true
			entry.Quote, entry.Replacement = match[1], match[2]
			var decoded string
			if json.Unmarshal([]byte(`"`+entry.Quote+`"`), &decoded) == nil {
				entry.Quote = decoded
			}
		}
		entries = append(entries, entry)
	}
	return entries
}
