package document

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var importPath = regexp.MustCompile(`(?:^|[` + space + `(])@((?:~/|\.{1,2}/|/)?[\w.-][\w./-]*)`)
var entity = regexp.MustCompile(`^&(?:#[xX][0-9a-fA-F]{1,6}|#[0-9]{1,7}|[a-zA-Z][a-zA-Z0-9]{1,31});`)
var inlineHTML = regexp.MustCompile(`^(?:` + htmlTag + `|<!--(?:[^-]|-[^-]|--[^>])*-->|<!---?>|<\?.*?\?>|<![a-zA-Z][^>]*>|<!\[CDATA\[(?s:.*?)\]\]>)`)
var autolink = regexp.MustCompile(`^<(?:[a-zA-Z][a-zA-Z0-9+.\-]{1,31}:[^<>\x00-\x20]*|[a-zA-Z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-zA-Z0-9.-]+)>`)

func Imports(text string) []string {
	var inline []string
	references := make(map[string]bool)
	parseMarkdown(text, func(item markdownItem) {
		if item.reference != "" {
			references[referenceLabel(item.reference)] = true
		}
		if item.inline != "" {
			inline = append(inline, item.inline)
		}
	})
	var paths []string
	for _, content := range inline {
		inlineText(content, references, func(text string) {
			for _, match := range importPath.FindAllStringSubmatchIndex(text, -1) {
				end := match[3]
				next, _ := utf8.DecodeRuneInString(text[end:])
				if end == len(text) || strings.ContainsRune("),;", next) || TrimSpace(string(next)) == "" {
					paths = append(paths, text[match[2]:end])
				}
			}
		}, 0)
	}
	return paths
}

func referenceLabel(text string) string {
	return strings.ToUpper(strings.Join(strings.Fields(TrimSpace(text)), " "))
}

func inlineText(text string, references map[string]bool, emit func(string), depth int) {
	if depth >= 20 {
		return
	}
	var plain strings.Builder
	flush := func() {
		if plain.Len() > 0 {
			emit(plain.String())
			plain.Reset()
		}
	}
	for index := 0; index < len(text); {
		rest := text[index:]
		switch rest[0] {
		case '\\':
			if len(rest) > 1 && (asciiPunctuation(rest[1]) || rest[1] == '\n') {
				if rest[1] == '\n' {
					flush()
				} else {
					plain.WriteByte(rest[1])
				}
				index += 2
				continue
			}
		case '\n':
			flush()
			index++
			continue
		case '`':
			count := repeated(rest, '`')
			if end := closingCode(rest, count); end != 0 {
				flush()
				index += end
			} else {
				plain.WriteString(rest[:count])
				index += count
			}
			continue
		case '&':
			if name := entity.FindString(rest); name != "" {
				plain.WriteString(html.UnescapeString(name))
				index += len(name)
				continue
			}
		case '<':
			if link := autolink.FindString(rest); link != "" {
				flush()
				emit(link[1 : len(link)-1])
				index += len(link)
				continue
			}
			if tag := inlineHTML.FindString(rest); tag != "" {
				flush()
				index += len(tag)
				continue
			}
		case '[', '!':
			image := strings.HasPrefix(rest, "![")
			start := 0
			if image {
				start = 1
			}
			if end := closingBracket(rest, start); end != 0 {
				label := rest[start+1 : end-1]
				if finish := endLink(rest, end, label, references); finish != 0 {
					flush()
					if !image {
						inlineText(label, references, emit, depth+1)
					}
					index += finish
					continue
				}
			}
		case '*', '_', '~':
			count := repeated(rest, rest[0])
			if count <= 3 && (rest[0] != '~' || count == 2) && opensEmphasis(text, index, count) {
				if end := closingEmphasis(text, index, count); end != 0 {
					flush()
					inlineText(text[index+count:end], references, emit, depth+1)
					index = end + count
					continue
				}
			}
			plain.WriteString(rest[:count])
			index += count
			continue
		}
		plain.WriteByte(rest[0])
		index++
	}
	flush()
}

func asciiPunctuation(char byte) bool {
	return char >= '!' && char <= '~' && !((char >= '0' && char <= '9') ||
		(char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z'))
}

func closingCode(text string, count int) int {
	for index := count; index < len(text); {
		next := strings.IndexByte(text[index:], '`')
		if next < 0 {
			break
		}
		index += next
		width := repeated(text[index:], '`')
		index += width
		if width == count {
			return index
		}
	}
	return 0
}

func closingBracket(text string, start int) int {
	if text[start] != '[' {
		return 0
	}
	depth := 1
	for index := start + 1; index < len(text); index++ {
		switch text[index] {
		case '\\':
			if index+1 < len(text) && asciiPunctuation(text[index+1]) {
				index++
			}
		case '`':
			count := repeated(text[index:], '`')
			if end := closingCode(text[index:], count); end != 0 {
				index += end - 1
			} else {
				index += count - 1
			}
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return index + 1
			}
		}
	}
	return 0
}

func endLink(text string, end int, label string, references map[string]bool) int {
	if end < len(text) && text[end] == '(' {
		depth := 1
		for index := end + 1; index < len(text); index++ {
			switch text[index] {
			case '\\':
				if index+1 < len(text) && asciiPunctuation(text[index+1]) {
					index++
				}
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					destination := asciiTrim(text[end+1 : index])
					if destination == "" || (referenceDestination.MatchString(destination) && allowedLink(destination)) {
						return index + 1
					}
					break
				}
			}
			if depth == 0 {
				break
			}
		}
	}
	if end < len(text) && text[end] == '[' {
		if close := closingBracket(text, end); close > 0 {
			if explicit := text[end+1 : close-1]; explicit != "" {
				label = explicit
			}
			if references[referenceLabel(label)] {
				return close
			}
			return 0
		}
	}
	if references[referenceLabel(label)] {
		return end
	}
	return 0
}

func flanking(text string, index, count int) (left, right, beforePunct, afterPunct bool) {
	before, _ := utf8.DecodeLastRuneInString(text[:index])
	after, _ := utf8.DecodeRuneInString(text[index+count:])
	beforeSpace := index == 0 || TrimSpace(string(before)) == ""
	afterSpace := index+count == len(text) || TrimSpace(string(after)) == ""
	beforePunct = unicode.IsPunct(before) || unicode.IsSymbol(before)
	afterPunct = unicode.IsPunct(after) || unicode.IsSymbol(after)
	left = !afterSpace && (!afterPunct || beforeSpace || beforePunct)
	right = !beforeSpace && (!beforePunct || afterSpace || afterPunct)
	return
}

func opensEmphasis(text string, index, count int) bool {
	left, right, beforePunct, _ := flanking(text, index, count)
	return left && (text[index] == '*' || !right || beforePunct)
}

func closingEmphasis(text string, start, count int) int {
	for index := start + count; index < len(text); {
		if text[index] == '\\' {
			index += 2
			continue
		}
		if text[index] != text[start] {
			index++
			continue
		}
		width := repeated(text[index:], text[start])
		left, right, _, afterPunct := flanking(text, index, width)
		if width == count && right && (text[start] == '*' || !left || afterPunct) {
			return index
		}
		index += width
	}
	return 0
}
