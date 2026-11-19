package document

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// RenderHTML renders the Markdown used in the Editor. Raw HTML is always text.
func RenderHTML(text string) string {
	var output strings.Builder
	lines := strings.Split(markdownNormalization.Replace(text), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	renderBlocks(&output, lines, 0, false)
	return output.String()
}

var closingHeading = regexp.MustCompile(`[ \t]+#+[ \t]*$`)

func renderBlocks(output *strings.Builder, lines []string, depth int, tight bool) {
	for index := 0; index < len(lines); {
		line := makeMarkdownLine(lines[index], 0)
		if line.text == "" {
			index++
			continue
		}
		if marker, count, info := fence(line.text); line.indent < 4 && count > 0 {
			index = renderFence(output, lines, index+1, line.indent, marker, count, info)
			continue
		}
		if level := headingLevel(line.text); line.indent < 4 && level > 0 {
			title := asciiTrim(closingHeading.ReplaceAllString(line.text[level:], ""))
			fmt.Fprintf(output, "<h%d>%s</h%d>\n", level, renderInline(title, 0), level)
			index++
			continue
		}
		if line.indent < 4 && thematicBreak(line.text) {
			output.WriteString("<hr>\n")
			index++
			continue
		}
		if marker, number := listMarker(line.text); line.indent < 4 && marker > 0 && depth < 20 {
			index = renderList(output, lines, index, depth, number)
			continue
		}

		start := index
		level := 0
		for index++; index < len(lines); index++ {
			next := makeMarkdownLine(lines[index], 0)
			if next.text == "" {
				break
			}
			if next.indent < 4 {
				if level = setext(next.text); level > 0 {
					break
				}
				_, count, _ := fence(next.text)
				list, number := listMarker(next.text)
				if count > 0 || headingLevel(next.text) > 0 || thematicBreak(next.text) ||
					(depth < 20 && list > 0 && (number < 0 || number == 1)) {
					break
				}
			}
		}
		paragraph := renderInline(asciiTrim(strings.Join(lines[start:index], "\n")), 0)
		switch {
		case level > 0:
			fmt.Fprintf(output, "<h%d>%s</h%d>\n", level, paragraph, level)
			index++
		case tight:
			output.WriteString(paragraph)
		default:
			fmt.Fprintf(output, "<p>%s</p>\n", paragraph)
		}
	}
}

func renderFence(output *strings.Builder, lines []string, index, indent int, marker byte, count int, info string) int {
	output.WriteString("<pre><code")
	if words := strings.Fields(info); len(words) > 0 {
		fmt.Fprintf(output, " class=\"language-%s\"", html.EscapeString(words[0]))
	}
	output.WriteByte('>')
	for ; index < len(lines); index++ {
		line := makeMarkdownLine(lines[index], 0)
		if line.indent < 4 && repeated(line.text, marker) >= count &&
			asciiTrim(strings.TrimLeft(line.text, string(marker))) == "" {
			index++
			break
		}
		raw := lines[index]
		raw = raw[min(indent, repeated(raw, ' ')):]
		output.WriteString(html.EscapeString(raw))
		output.WriteByte('\n')
	}
	output.WriteString("</code></pre>\n")
	return index
}

func renderList(output *strings.Builder, lines []string, index, depth, number int) int {
	tag := "ul"
	if number >= 0 {
		tag = "ol"
	}
	if tag == "ol" && number != 1 {
		fmt.Fprintf(output, "<ol start=\"%d\">\n", number)
	} else {
		fmt.Fprintf(output, "<%s>\n", tag)
	}
	first := makeMarkdownLine(lines[index], 0)
	firstMarker, _ := listMarker(first.text)
	delimiter := first.text[firstMarker-1]
	var items [][]string
	loose := false
	for index < len(lines) {
		line := makeMarkdownLine(lines[index], 0)
		marker, _ := listMarker(line.text)
		if marker == 0 || line.indent >= 4 || line.text[marker-1] != delimiter || thematicBreak(line.text) {
			break
		}
		body := line.text[marker:]
		skip, indent := indentation(body, line.indent+marker)
		indent = max(indent, line.indent+marker+1)
		item := []string{body[skip:]}
		blank := false
		for index++; index < len(lines); index++ {
			next := makeMarkdownLine(lines[index], 0)
			if next.text == "" {
				blank = true
				continue
			}
			if next.indent < indent {
				nextMarker, _ := listMarker(next.text)
				loose = loose || (blank && next.indent < 4 && nextMarker > 0 &&
					next.text[nextMarker-1] == delimiter && !thematicBreak(next.text))
				break
			}
			if blank {
				item = append(item, "")
				loose = true
				blank = false
			}
			item = append(item, strings.Repeat(" ", next.indent-indent)+next.text)
		}
		items = append(items, item)
	}
	for _, item := range items {
		output.WriteString("<li>")
		renderBlocks(output, item, depth+1, !loose)
		output.WriteString("</li>\n")
	}
	fmt.Fprintf(output, "</%s>\n", tag)
	return index
}

var inlineLink = regexp.MustCompile(
	`^\[((?:\\[[:punct:]]|[^\[\]\n])+)\]\((<[^<>\n]+>|(?:[^()<>\s]|\([^()\n]*\))+)(?:[ \t]+["'][^\n]*?["'])?\)`,
)

func renderInline(text string, depth int) string {
	if depth >= 12 {
		return escapeInline(text)
	}
	var output strings.Builder
	start := 0
	for index := 0; index < len(text); {
		rest := text[index:]
		var rendered string
		width := 1
		switch rest[0] {
		case '\\':
			if len(rest) >= 2 && asciiPunctuation(rest[1]) {
				rendered = html.EscapeString(rest[1:2])
				width = 2
			}
		case '`':
			width = repeated(rest, '`')
			if end := closingCode(rest, width); end > 0 {
				code := strings.ReplaceAll(rest[width:end-width], "\n", " ")
				if strings.HasPrefix(code, " ") && strings.HasSuffix(code, " ") && strings.TrimSpace(code) != "" {
					code = code[1 : len(code)-1]
				}
				rendered = "<code>" + html.EscapeString(code) + "</code>"
				width = end
			}
		case '[':
			if match := inlineLink.FindStringSubmatch(rest); match != nil {
				if href := linkURL(match[2]); href != "" {
					rendered = fmt.Sprintf("<a href=\"%s\">%s</a>", html.EscapeString(href), renderInline(match[1], depth+1))
					width = len(match[0])
				}
			}
		case '*', '_':
			width = repeated(rest, rest[0])
			if width <= 3 && opensEmphasis(text, index, width) {
				if end := closingEmphasis(text, index, width); end > 0 {
					tags := [...]string{"", "<em>%s</em>", "<strong>%s</strong>", "<em><strong>%s</strong></em>"}
					rendered = fmt.Sprintf(tags[width], renderInline(text[index+width:end], depth+1))
					width += end - index
				}
			}
		}
		if rendered != "" {
			output.WriteString(escapeInline(text[start:index]))
			output.WriteString(rendered)
			start = index + width
		}
		index += width
	}
	output.WriteString(escapeInline(text[start:]))
	return output.String()
}

func escapeInline(text string) string {
	return html.EscapeString(html.UnescapeString(text))
}

func linkURL(text string) string {
	text = html.UnescapeString(strings.TrimSuffix(strings.TrimPrefix(text, "<"), ">"))
	if strings.ContainsAny(text, "\\\t\r\n") {
		return ""
	}
	link, err := url.Parse(text)
	if err != nil {
		return ""
	}
	switch strings.ToLower(link.Scheme) {
	case "", "http", "https", "mailto":
		return link.String()
	default:
		return ""
	}
}
