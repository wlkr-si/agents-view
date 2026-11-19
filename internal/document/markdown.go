package document

import (
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Only the Markdown items used by the document and Global discovery are
// emitted. Line numbers refer to the original document, including in lists.
type markdownItem struct {
	start, end, depth, heading int
	diagram, bullet            bool
	inline, reference          string
}

type markdownLine struct {
	text              string
	indent, tabOffset int
}

func makeMarkdownLine(text string, column int) markdownLine {
	end, next := indentation(text, column)
	return markdownLine{text[end:], next - column, column}
}

func indentation(text string, column int) (int, int) {
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case ' ':
			column++
		case '\t':
			column += 4 - column%4
		default:
			return index, column
		}
	}
	return len(text), column
}

func asciiTrim(text string) string { return strings.Trim(text, " \t\n\r") }

type markdownParser struct {
	lines []markdownLine
	emit  func(markdownItem)
}

var markdownNormalization = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\x00", "\uFFFD")

func parseMarkdown(text string, emit func(markdownItem)) {
	raw := strings.Split(markdownNormalization.Replace(text), "\n")
	lines := make([]markdownLine, len(raw))
	for index, line := range raw {
		lines[index] = makeMarkdownLine(line, 0)
	}
	markdownParser{lines, emit}.parse(0, len(lines), 0, 0)
}

func (parser markdownParser) parse(start, end, indent, depth int) int {
	lines := parser.lines
	for index := start; index < end; {
		line := lines[index]
		if line.text == "" {
			index++
			continue
		}
		if line.indent < indent {
			return index
		}
		// The v0 parser stops at 20 open Markdown containers.
		if depth >= 20 {
			return end
		}
		item := markdownItem{start: index, depth: depth}
		if columns := parser.tableHeader(index, end, indent); len(columns) != 0 {
			for _, cell := range columns {
				parser.emit(markdownItem{inline: TrimSpace(cell)})
			}
			missing := 0
			index += 2
			for ; index < end; index++ {
				row := lines[index]
				if row.indent < indent || row.indent-indent >= 4 || TrimSpace(row.text) == "" ||
					parser.interrupts(index, end, indent, false) {
					break
				}
				cells := tableCells(TrimSpace(row.text))
				missing += len(columns) - len(cells)
				if missing > 0x10000 {
					break
				}
				for _, cell := range cells[:min(len(cells), len(columns))] {
					parser.emit(markdownItem{inline: TrimSpace(cell)})
				}
			}
			continue
		}
		if line.indent-indent >= 4 {
			index++
			for index < end && (lines[index].text == "" || lines[index].indent-indent >= 4) {
				index++
			}
			continue
		}
		if marker, count, info := fence(line.text); count != 0 {
			item.diagram = TrimSpace(info) == "mermaid"
			index++
			for index < end {
				next := lines[index]
				if next.text != "" && next.indent < indent {
					break
				}
				index++
				if next.indent-indent < 4 && repeated(next.text, marker) >= count &&
					asciiTrim(strings.TrimLeft(next.text, string(marker))) == "" {
					break
				}
			}
			parser.emit(item)
			continue
		}
		if line.text[0] == '>' {
			index = parser.quote(index, end, indent, depth)
			continue
		}
		if thematicBreak(line.text) {
			index++
			continue
		}
		if marker, _ := listMarker(line.text); marker != 0 {
			item.bullet = strings.ContainsRune("-+*", rune(line.text[marker-1]))
			parser.emit(item)
			next := parser.listItem(index, end, depth, marker)
			index = max(index+1, next)
			continue
		}
		if reference, label := parser.referenceEnd(index, end, indent); reference > index {
			parser.emit(markdownItem{reference: label})
			index = reference
			continue
		}
		if closing, _ := htmlSection(line.text); closing != nil {
			index++
			if closing == blankHTML {
				for index < end && lines[index].text != "" && lines[index].indent >= indent {
					index++
				}
			} else if !closing.MatchString(line.text) {
				for index < end && (lines[index].text == "" || lines[index].indent >= indent) {
					index++
					if closing.MatchString(lines[index-1].text) {
						break
					}
				}
			}
			continue
		}
		if level := headingLevel(line.text); level > 0 {
			item.heading = level
			item.end = index + 1
			content := line.text[level:]
			trimmed := strings.TrimRight(content, " \t")
			tail := strings.TrimRight(trimmed, "#")
			if len(tail) < len(trimmed) && len(tail) > 0 && strings.ContainsRune(" \t", rune(tail[len(tail)-1])) {
				content = tail
			}
			item.inline = asciiTrim(content)
			parser.emit(item)
			index++
			continue
		}

		var inline strings.Builder
		inline.WriteString(line.text)
		index++
		for index < end && lines[index].text != "" {
			next := lines[index]
			if next.indent >= indent && next.indent-indent < 4 && setext(next.text) != 0 {
				item.heading = setext(next.text)
				index++
				break
			}
			if next.indent >= 0 && parser.interrupts(index, end, indent, true) {
				break
			}
			inline.WriteByte('\n')
			inline.WriteString(next.text)
			index++
		}
		item.end = index
		item.inline = asciiTrim(inline.String())
		parser.emit(item)
	}
	return end
}

func (parser markdownParser) interrupts(index, end, indent int, paragraph bool) bool {
	line := parser.lines[index]
	if line.indent-indent >= 4 || line.text == "" {
		return false
	}
	_, count, _ := fence(line.text)
	_, interrupts := htmlSection(line.text)
	if count != 0 || line.text[0] == '>' || thematicBreak(line.text) || interrupts || headingLevel(line.text) > 0 {
		return true
	}
	if marker, number := listMarker(line.text); marker != 0 {
		if !paragraph || line.indent < indent || ((number < 0 || number == 1) && asciiTrim(line.text[marker:]) != "") {
			return true
		}
	}
	return paragraph && len(parser.tableHeader(index, end, indent)) > 0
}

func (parser markdownParser) quote(start, end, indent, depth int) int {
	var original []markdownLine
	index := start
	empty := false
	for ; index < end; index++ {
		line := parser.lines[index]
		if line.text == "" {
			break
		}
		if line.indent >= indent && line.text[0] == '>' {
			original = append(original, line)
			content := line.text[1:]
			column := line.indent + line.tabOffset + 1
			if strings.HasPrefix(content, " ") || (strings.HasPrefix(content, "\t") && column%4 == 3) {
				content = content[1:]
				column++
			} else if strings.HasPrefix(content, "\t") {
				// One column after the marker belongs to the quote itself.
				column++
			}
			next := makeMarkdownLine(content, column)
			empty = next.text == ""
			parser.lines[index] = next
			continue
		}
		if empty || parser.interrupts(index, end, indent, false) {
			break
		}
		original = append(original, line)
		line.indent = -1 // A possible lazy paragraph continuation.
		parser.lines[index] = line
	}
	next := parser.parse(start, index, 0, depth+1)
	copy(parser.lines[start:], original)
	return max(start+1, next)
}

func (parser markdownParser) listItem(start, end, depth, marker int) int {
	line := parser.lines[start]
	column := line.indent + marker
	content := line.text[marker:]
	skip, next := indentation(content, column+line.tabOffset)
	next -= line.tabOffset
	indent := next
	if skip == len(content) || next-column > 4 {
		indent = column + 1
	}
	if skip == len(content) && (start+1 >= end || parser.lines[start+1].text == "") {
		return min(start+2, end)
	}
	parser.lines[start] = markdownLine{content[skip:], next, line.tabOffset}
	if skip == len(content) {
		parser.lines[start].indent = indent
	}
	nextLine := parser.parse(start, end, indent, depth+2)
	parser.lines[start] = line
	return nextLine
}

func fence(text string) (byte, int, string) {
	if len(text) < 3 || (text[0] != '`' && text[0] != '~') {
		return 0, 0, ""
	}
	marker := text[0]
	count := repeated(text, marker)
	info := text[count:]
	if count < 3 || (marker == '`' && strings.ContainsRune(info, '`')) {
		return 0, 0, ""
	}
	return marker, count, info
}

func repeated(text string, char byte) int {
	index := 0
	for index < len(text) && text[index] == char {
		index++
	}
	return index
}

func headingLevel(text string) int {
	count := repeated(text, '#')
	if count >= 1 && count <= 6 && (len(text) == count || strings.ContainsRune(" \t", rune(text[count]))) {
		return count
	}
	return 0
}

func thematicBreak(text string) bool {
	if text == "" || !strings.ContainsRune("-*_", rune(text[0])) {
		return false
	}
	count := 0
	for _, char := range text {
		switch {
		case char == rune(text[0]):
			count++
		case char != ' ' && char != '\t':
			return false
		}
	}
	return count >= 3
}

func setext(text string) int {
	if text[0] == '=' && asciiTrim(text[repeated(text, '='):]) == "" {
		return 1
	}
	if text[0] == '-' && asciiTrim(text[repeated(text, '-'):]) == "" {
		return 2
	}
	return 0
}

func listMarker(text string) (end, number int) {
	if text == "" {
		return 0, 0
	}
	if strings.ContainsRune("-+*", rune(text[0])) {
		end, number = 1, -1
	} else {
		digits := 0
		for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
			digits++
		}
		if digits < 1 || digits > 9 || digits == len(text) || !strings.ContainsRune(".)", rune(text[digits])) {
			return 0, 0
		}
		end = digits + 1
		number, _ = strconv.Atoi(text[:digits])
	}
	if end < len(text) && !strings.ContainsRune(" \t", rune(text[end])) {
		return 0, 0
	}
	return end, number
}

const htmlAttribute = `(?:[` + space + `]+[a-zA-Z_:][a-zA-Z0-9:._-]*(?:[` + space + `]*=[` + space + `]*(?:[^"'=<>` + "`" + `\x00-\x20]+|'[^']*'|"[^"]*"))?)`
const htmlTag = `(?:<[A-Za-z][A-Za-z0-9-]*` + htmlAttribute + `*[` + space + `]*/?>|</[A-Za-z][A-Za-z0-9-]*[` + space + `]*>)`

var blankHTML = regexp.MustCompile(`^$`)
var htmlSections = []struct{ open, close *regexp.Regexp }{
	{regexp.MustCompile(`(?i)^<(?:script|pre|style|textarea)(?:[` + space + `>]|$)`), regexp.MustCompile(`(?i)</(?:script|pre|style|textarea)>`)},
	{regexp.MustCompile(`^<!--`), regexp.MustCompile(`-->`)},
	{regexp.MustCompile(`^<\?`), regexp.MustCompile(`\?>`)},
	{regexp.MustCompile(`^<![A-Za-z]`), regexp.MustCompile(`>`)},
	{regexp.MustCompile(`^<!\[CDATA\[`), regexp.MustCompile(`\]\]>`)},
	{regexp.MustCompile(`(?i)^</?(?:address|article|aside|base|basefont|blockquote|body|caption|center|col|colgroup|dd|details|dialog|dir|div|dl|dt|fieldset|figcaption|figure|footer|form|frame|frameset|h[1-6]|head|header|hr|html|iframe|legend|li|link|main|menu|menuitem|nav|noframes|ol|optgroup|option|p|param|search|section|summary|table|tbody|td|tfoot|th|thead|title|tr|track|ul)(?:[` + space + `]|/?>|$)`), blankHTML},
	{regexp.MustCompile(`^` + htmlTag + `[` + space + `]*$`), blankHTML},
}

func htmlSection(text string) (*regexp.Regexp, bool) {
	if !strings.HasPrefix(text, "<") {
		return nil, false
	}
	for index, section := range htmlSections {
		if section.open.MatchString(text) {
			return section.close, index < len(htmlSections)-1
		}
	}
	return nil, false
}

var tableDivider = regexp.MustCompile(`^:?-+:?$`)

func (parser markdownParser) tableHeader(start, end, indent int) []string {
	if start+1 >= end || parser.lines[start].indent-indent >= 4 ||
		!strings.Contains(parser.lines[start].text, "|") {
		return nil
	}
	line := parser.lines[start+1]
	if line.indent < indent || line.indent-indent >= 4 || len(line.text) < 2 ||
		(strings.HasPrefix(line.text, "-") && strings.ContainsRune(" \t", rune(line.text[1]))) {
		return nil
	}
	columns := strings.Split(line.text, "|")
	count := 0
	for index, column := range columns {
		column = asciiTrim(column)
		if column == "" && (index == 0 || index == len(columns)-1) {
			continue
		}
		if !tableDivider.MatchString(column) {
			return nil
		}
		count++
	}
	cells := tableCells(TrimSpace(parser.lines[start].text))
	if len(cells) != count {
		return nil
	}
	return cells
}

func tableCells(text string) []string {
	var cells []string
	start := 0
	for index := 0; index < len(text); index++ {
		if text[index] == '|' && (index == 0 || text[index-1] != '\\') {
			cells = append(cells, strings.ReplaceAll(text[start:index], "\\|", "|"))
			start = index + 1
		}
	}
	cells = append(cells, strings.ReplaceAll(text[start:], "\\|", "|"))
	if len(cells) > 0 && cells[0] == "" {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	return cells
}

var referenceLabelEnd = regexp.MustCompile(`^((?:[^\[\]\\]|\\.)+)\]:[ \t]*`)
var referenceLabelLine = regexp.MustCompile(`^(?:[^\[\]\\]|\\.)*\\?$`)
var referenceDestination = regexp.MustCompile(`^(?:<[^<>]*>|[^ \t<>]+)(?:[ \t]+(?:"[^"]*"|'[^']*'|\([^()]*\)))?[ \t]*$`)
var referenceTitle = regexp.MustCompile(`^(?:"[^"]*"|'[^']*'|\([^()]*\))[ \t]*$`)

func (parser markdownParser) referenceEnd(start, end, indent int) (int, string) {
	line := parser.lines[start].text
	if !strings.HasPrefix(line, "[") {
		return start, ""
	}
	line = line[1:]
	var label strings.Builder
	next := start + 1
	match := referenceLabelEnd.FindStringSubmatch(line)
	for match == nil {
		if !referenceLabelLine.MatchString(line) || label.Len()+len(line) > 3996 ||
			next >= end || parser.lines[next].text == "" || parser.interrupts(next, end, indent, false) {
			return start, ""
		}
		label.WriteString(line)
		label.WriteByte('\n')
		line = parser.lines[next].text
		next++
		match = referenceLabelEnd.FindStringSubmatch(line)
	}
	label.WriteString(match[1])
	if TrimSpace(label.String()) == "" || utf8.RuneCountInString(label.String()) > 999 {
		return start, ""
	}
	destination := line[len(match[0]):]
	if destination == "" && next < end && !parser.interrupts(next, end, indent, false) {
		destination = parser.lines[next].text
		next++
	}
	if !referenceDestination.MatchString(destination) || !allowedLink(destination) {
		return start, ""
	}
	if next < end && referenceTitle.MatchString(parser.lines[next].text) {
		next++
	}
	return next, label.String()
}

func allowedLink(destination string) bool {
	lower := strings.ToLower(strings.TrimPrefix(destination, "<"))
	for _, scheme := range []string{"vbscript:", "javascript:", "file:", "data:"} {
		if !strings.HasPrefix(lower, scheme) {
			continue
		}
		for _, image := range []string{"gif", "png", "jpeg", "webp"} {
			if strings.HasPrefix(lower, "data:image/"+image+";") {
				return true
			}
		}
		return false
	}
	return true
}
