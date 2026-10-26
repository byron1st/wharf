package atlassian

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// adfNode is a node of the Atlassian Document Format that Jira uses for rich
// text fields such as description.
type adfNode struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Attrs struct {
		Text      string `json:"text"`
		URL       string `json:"url"`
		Level     int    `json:"level"`
		Order     *int   `json:"order"`
		State     string `json:"state"`
		Language  string `json:"language"`
		Title     string `json:"title"`
		ShortName string `json:"shortName"`
		// Timestamp is epoch milliseconds, kept raw so that a malformed value
		// drops the date instead of failing to decode the whole work item.
		Timestamp json.RawMessage `json:"timestamp"`
	} `json:"attrs"`
	Marks   []adfMark `json:"marks"`
	Content []adfNode `json:"content"`
}

type adfMark struct {
	Type  string `json:"type"`
	Attrs struct {
		Href string `json:"href"`
	} `json:"attrs"`
}

// mdBlock is a rendered Markdown block; list marks list blocks that may follow
// the previous block on the next line, so that a list directly inside a list
// item can stay tight. An ordered list starting at a number other than 1 cannot
// interrupt a paragraph in CommonMark, so it is not marked. family and alt record the list
// marker family ("bullet" or "ordered") and whether the alternate marker was
// used, so an adjacent sibling list of the same family can switch markers.
type mdBlock struct {
	text   string
	list   bool
	family string
	alt    bool
}

// markdown renders the document as CommonMark with GFM tables and task lists.
// It never fails: unknown nodes render their children so their text survives.
// Text is not escaped (only `|` in table cells), so Markdown-like characters in
// the original text may be interpreted as formatting.
func (n *adfNode) markdown() string {
	if n == nil {
		return ""
	}

	return strings.TrimSpace(joinBlocks(renderBlocks([]adfNode{*n}), false))
}

// renderBlocks renders block nodes, grouping consecutive inline nodes (as in
// taskItem or unknown containers) into one paragraph and dropping empty blocks.
// A list directly following a list of the same family alternates its marker,
// since CommonMark otherwise merges the two into one loose list.
func renderBlocks(nodes []adfNode) []mdBlock {
	var (
		blocks []mdBlock
		run    []adfNode
	)

	add := func(b mdBlock) {
		if strings.TrimSpace(b.text) != "" {
			blocks = append(blocks, b)
		}
	}
	flush := func() {
		if len(run) > 0 {
			add(mdBlock{text: renderInline(run)})
			run = nil
		}
	}

	for _, node := range nodes {
		if isInline(node.Type) {
			run = append(run, node)
			continue
		}

		flush()

		if family := listFamily(node.Type); family != "" {
			prev := mdBlock{}
			if len(blocks) > 0 {
				prev = blocks[len(blocks)-1]
			}

			alt := prev.family == family && !prev.alt
			interrupts := node.Type != "orderedList" || node.Attrs.Order == nil || *node.Attrs.Order == 1
			add(mdBlock{text: renderList(node, alt), list: interrupts, family: family, alt: alt})

			continue
		}

		add(renderBlock(node))
	}

	flush()

	return blocks
}

// joinBlocks separates blocks with a blank line. When tight, a list follows the
// previous block on the next line so the enclosing list stays tight.
func joinBlocks(blocks []mdBlock, tight bool) string {
	var b strings.Builder

	for i, block := range blocks {
		if i > 0 {
			if tight && block.list {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}

		b.WriteString(block.text)
	}

	return b.String()
}

func renderBlock(n adfNode) mdBlock {
	switch n.Type {
	case "paragraph":
		return mdBlock{text: renderInline(n.Content)}
	case "heading":
		text := renderInline(n.Content)
		if text == "" {
			return mdBlock{}
		}

		return mdBlock{text: strings.Repeat("#", min(max(n.Attrs.Level, 1), 6)) + " " + text}
	case "codeBlock":
		return mdBlock{text: renderCodeBlock(n)}
	case "blockquote", "panel":
		// panelType has no Markdown equivalent, so panels become plain quotes.
		return mdBlock{text: prefixLines(joinBlocks(renderBlocks(n.Content), false), "> ", ">")}
	case "rule":
		return mdBlock{text: "---"}
	case "table":
		return mdBlock{text: renderTable(n)}
	case "expand", "nestedExpand":
		blocks := renderBlocks(n.Content)
		if n.Attrs.Title != "" {
			blocks = append([]mdBlock{{text: "**" + n.Attrs.Title + "**"}}, blocks...)
		}

		return mdBlock{text: joinBlocks(blocks, false)}
	case "blockCard", "embedCard":
		return mdBlock{text: autolink(n.Attrs.URL)}
	case "mediaSingle", "mediaGroup", "media":
		return mdBlock{}
	}

	return mdBlock{text: joinBlocks(renderBlocks(n.Content), false)}
}

func listFamily(nodeType string) string {
	switch nodeType {
	case "bulletList", "taskList", "decisionList":
		return "bullet"
	case "orderedList":
		return "ordered"
	}

	return ""
}

func isInline(nodeType string) bool {
	switch nodeType {
	case "text", "hardBreak", "mention", "emoji", "status", "date", "inlineCard":
		return true
	}

	return false
}

func renderInline(nodes []adfNode) string {
	var b strings.Builder

	for _, n := range mergeText(nodes) {
		switch n.Type {
		case "text":
			b.WriteString(applyMarks(n.Text, n.Marks))
		case "hardBreak":
			b.WriteString("\\\n")
		case "mention":
			b.WriteString(n.Attrs.Text)
		case "emoji":
			if n.Attrs.Text != "" {
				b.WriteString(n.Attrs.Text)
			} else {
				b.WriteString(n.Attrs.ShortName)
			}
		case "status":
			if n.Attrs.Text != "" {
				b.WriteString("[" + n.Attrs.Text + "]")
			}
		case "date":
			if ms, err := strconv.ParseInt(strings.Trim(string(n.Attrs.Timestamp), `"`), 10, 64); err == nil {
				b.WriteString(time.UnixMilli(ms).UTC().Format(time.DateOnly))
			}
		case "inlineCard":
			b.WriteString(autolink(n.Attrs.URL))
		default:
			b.WriteString(n.Text)
			b.WriteString(renderInline(n.Content))
		}
	}

	text := b.String()
	// A hard break that ends a paragraph would leave a literal backslash.
	for strings.HasSuffix(text, "\\\n") {
		text = strings.TrimSuffix(text, "\\\n")
	}

	return text
}

// mergeText joins consecutive text nodes with the same marks, since rendering
// them separately puts delimiters back to back (`**a****b**`), which CommonMark
// does not parse as close and reopen.
func mergeText(nodes []adfNode) []adfNode {
	merged := make([]adfNode, 0, len(nodes))

	for _, n := range nodes {
		if len(merged) > 0 {
			last := &merged[len(merged)-1]
			if n.Type == "text" && last.Type == "text" && sameMarks(last.Marks, n.Marks) {
				last.Text += n.Text
				continue
			}
		}

		merged = append(merged, n)
	}

	return merged
}

// sameMarks reports whether a and b hold the same marks in any order.
func sameMarks(a, b []adfMark) bool {
	if len(a) != len(b) {
		return false
	}

	for _, m := range a {
		if !slices.Contains(b, m) {
			return false
		}
	}

	return true
}

// applyMarks nests marks as link > strike > strong > em > code and keeps
// surrounding spaces outside the delimiters, which CommonMark requires for
// emphasis. Marks without a Markdown equivalent keep the text only.
func applyMarks(text string, marks []adfMark) string {
	trimmed := strings.TrimSpace(text)
	core := trimmed

	if core == "" || len(marks) == 0 {
		return text
	}

	has := map[string]bool{}

	var href string

	for _, m := range marks {
		has[m.Type] = true
		if m.Type == "link" {
			href = m.Attrs.Href
		}
	}

	if has["code"] {
		core = codeSpan(core)
	}

	for _, d := range []struct{ mark, delim string }{{"em", "*"}, {"strong", "**"}, {"strike", "~~"}} {
		if has[d.mark] {
			core = d.delim + core + d.delim
		}
	}

	if href != "" {
		core = "[" + core + "](" + href + ")"
	}

	start := strings.Index(text, trimmed)

	return text[:start] + core + text[start+len(trimmed):]
}

// codeSpan wraps text in a backtick run longer than any run inside it.
func codeSpan(text string) string {
	delim := strings.Repeat("`", longestRun(text, '`')+1)
	if strings.HasPrefix(text, "`") || strings.HasSuffix(text, "`") {
		text = " " + text + " "
	}

	return delim + text + delim
}

func renderCodeBlock(n adfNode) string {
	var b strings.Builder
	for _, c := range n.Content {
		b.WriteString(c.Text)
	}

	code := strings.TrimSuffix(b.String(), "\n")
	if strings.TrimSpace(code) == "" {
		return ""
	}

	fence := strings.Repeat("`", max(3, longestRun(code, '`')+1))

	return fence + n.Attrs.Language + "\n" + code + "\n" + fence
}

// renderList renders a tight list. Items are rendered with their marker and
// continuation lines are indented to the item content; a list nested directly
// in a list (as ADF does for nested task lists) is indented under the previous
// item. alt selects the alternate marker (`*` or `N)`) instead of `-` / `N.`.
func renderList(n adfNode, alt bool) string {
	number := 1
	if n.Attrs.Order != nil {
		number = *n.Attrs.Order
	}

	var (
		lines  []string
		indent = "  "
	)

	for _, item := range n.Content {
		if listFamily(item.Type) != "" {
			if nested := renderList(item, false); nested != "" {
				lines = append(lines, prefixLines(nested, indent, ""))
			}

			continue
		}

		marker := "- "
		if alt {
			marker = "* "
		}

		if n.Type == "orderedList" {
			delim := "."
			if alt {
				delim = ")"
			}

			marker = fmt.Sprintf("%d%s ", number, delim)
			number++
		}

		indent = strings.Repeat(" ", len(marker))

		body := joinBlocks(renderBlocks(item.Content), true)

		if n.Type == "taskList" {
			box := "[ ] "
			if item.Attrs.State == "DONE" {
				box = "[x] "
			}

			body = box + body
		}

		// The first line follows the marker; the rest align with the item content.
		lines = append(lines, strings.TrimRight(marker+strings.TrimPrefix(prefixLines(body, indent, ""), indent), " "))
	}

	return strings.Join(lines, "\n")
}

// renderTable renders a GFM table using the first row as the header, since GFM
// requires one. Each cell is flattened to one line.
func renderTable(n adfNode) string {
	var (
		rows [][]string
		cols int
	)

	for _, row := range n.Content {
		if len(row.Content) == 0 {
			continue
		}

		var cells []string
		for _, cell := range row.Content {
			cells = append(cells, renderCell(cell))
		}

		rows = append(rows, cells)
		cols = max(cols, len(cells))
	}

	if cols == 0 {
		return ""
	}

	writeRow := func(b *strings.Builder, cells []string) {
		b.WriteString("|")

		for i := range cols {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}

			b.WriteString(" " + cell + " |")
		}
	}

	var b strings.Builder

	writeRow(&b, rows[0])
	b.WriteString("\n|" + strings.Repeat(" --- |", cols))

	for _, row := range rows[1:] {
		b.WriteString("\n")
		writeRow(&b, row)
	}

	return b.String()
}

// renderCell joins the cell's blocks and hard breaks with <br>, folds other
// newlines into spaces and escapes pipes.
func renderCell(cell adfNode) string {
	parts := make([]string, 0, len(cell.Content))
	for _, block := range renderBlocks(cell.Content) {
		parts = append(parts, block.text)
	}

	text := strings.Join(parts, "<br>")
	text = strings.ReplaceAll(text, "\\\n", "<br>")
	text = strings.ReplaceAll(text, "\n", " ")

	return strings.ReplaceAll(text, "|", `\|`)
}

func autolink(url string) string {
	if url == "" {
		return ""
	}

	return "<" + url + ">"
}

// prefixLines prefixes every line with prefix, and empty lines with emptyPrefix.
func prefixLines(text, prefix, emptyPrefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = emptyPrefix
		} else {
			lines[i] = prefix + line
		}
	}

	return strings.Join(lines, "\n")
}

func longestRun(text string, r rune) int {
	longest, current := 0, 0

	for _, c := range text {
		if c == r {
			current++
			longest = max(longest, current)
		} else {
			current = 0
		}
	}

	return longest
}
