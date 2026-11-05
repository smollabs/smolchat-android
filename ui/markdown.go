package ui

import (
	"image"
	"image/color"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/richtext"
)

// ---- parsing ----------------------------------------------------------------

type blockKind int

const (
	bPara blockKind = iota
	bHeading
	bCode
	bList
	bQuote
	bRule
	bTable
	bThink
)

type mdItem struct {
	text   string
	indent int
	marker string // "•" or "3."
}

type mdBlock struct {
	kind  blockKind
	level int    // heading level
	lang  string // code fence language
	text  string
	items []mdItem
	open  bool // unterminated (still streaming)
}

var (
	reHeading = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	reRule    = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
	reBullet  = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	reOrdered = regexp.MustCompile(`^(\s*)(\d{1,3})[.)]\s+(.*)$`)
	reFence   = regexp.MustCompile("^\\s*(```|~~~)\\s*([\\w+#.-]*)")
)

// parseMarkdown splits src into blocks. It tolerates incomplete input, so it
// can render a reply while it is still streaming.
func parseMarkdown(src string) []mdBlock {
	var out []mdBlock
	// Reasoning models wrap their scratchpad in <think>…</think>.
	if i := strings.Index(src, "<think>"); i >= 0 {
		before, rest := src[:i], src[i+len("<think>"):]
		out = append(out, parseMarkdown(before)...)
		j := strings.Index(rest, "</think>")
		if j < 0 {
			out = append(out, mdBlock{kind: bThink, text: strings.TrimSpace(rest), open: true})
			return out
		}
		if t := strings.TrimSpace(rest[:j]); t != "" {
			out = append(out, mdBlock{kind: bThink, text: t})
		}
		return append(out, parseMarkdown(rest[j+len("</think>"):])...)
	}

	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, mdBlock{kind: bPara, text: strings.Join(para, " ")})
			para = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		trim := strings.TrimSpace(ln)
		switch {
		case reFence.MatchString(ln):
			flush()
			m := reFence.FindStringSubmatch(ln)
			fence := m[1]
			b := mdBlock{kind: bCode, lang: strings.ToLower(m[2]), open: true}
			var code []string
			for i++; i < len(lines); i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), fence) {
					b.open = false
					break
				}
				code = append(code, lines[i])
			}
			b.text = strings.Join(code, "\n")
			out = append(out, b)
		case trim == "":
			flush()
		case reHeading.MatchString(trim):
			flush()
			m := reHeading.FindStringSubmatch(trim)
			out = append(out, mdBlock{kind: bHeading, level: len(m[1]), text: strings.TrimRight(m[2], "# ")})
		case reRule.MatchString(trim) && len(para) == 0:
			out = append(out, mdBlock{kind: bRule})
		case strings.HasPrefix(trim, ">"):
			flush()
			var q []string
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">"); i++ {
				q = append(q, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")))
			}
			i--
			out = append(out, mdBlock{kind: bQuote, text: strings.Join(q, " ")})
		case strings.HasPrefix(trim, "|"):
			flush()
			var rows []string
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|"); i++ {
				rows = append(rows, strings.TrimSpace(lines[i]))
			}
			i--
			out = append(out, mdBlock{kind: bTable, text: formatTable(rows)})
		case reBullet.MatchString(ln) || reOrdered.MatchString(ln):
			flush()
			b := mdBlock{kind: bList}
			for ; i < len(lines); i++ {
				l := lines[i]
				if m := reBullet.FindStringSubmatch(l); m != nil {
					b.items = append(b.items, mdItem{text: m[2], indent: len(m[1]) / 2, marker: "•"})
				} else if m := reOrdered.FindStringSubmatch(l); m != nil {
					b.items = append(b.items, mdItem{text: m[3], indent: len(m[1]) / 2, marker: m[2] + "."})
				} else if strings.TrimSpace(l) != "" && len(b.items) > 0 && (strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "\t")) {
					b.items[len(b.items)-1].text += " " + strings.TrimSpace(l) // continuation
				} else if strings.TrimSpace(l) == "" && i+1 < len(lines) && (reBullet.MatchString(lines[i+1]) || reOrdered.MatchString(lines[i+1])) {
					continue // loose list
				} else {
					break
				}
			}
			i--
			out = append(out, b)
		default:
			para = append(para, trim)
		}
	}
	flush()
	return out
}

// formatTable aligns a pipe table into fixed-width columns for monospace
// display, dropping the |---| separator row.
func formatTable(rows []string) string {
	var cells [][]string
	var widths []int
	for _, r := range rows {
		r = strings.Trim(r, "|")
		parts := strings.Split(r, "|")
		sep := true
		for j := range parts {
			parts[j] = stripInline(strings.TrimSpace(parts[j]))
			if strings.Trim(parts[j], ":- ") != "" {
				sep = false
			}
		}
		if sep {
			continue
		}
		for j, p := range parts {
			if j >= len(widths) {
				widths = append(widths, 0)
			}
			widths[j] = max(widths[j], len([]rune(p)))
		}
		cells = append(cells, parts)
	}
	var sb strings.Builder
	for ri, row := range cells {
		for j, c := range row {
			sb.WriteString(c)
			if j < len(row)-1 {
				sb.WriteString(strings.Repeat(" ", widths[j]-len([]rune(c))+3))
			}
		}
		if ri < len(cells)-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func stripInline(s string) string {
	return strings.NewReplacer("**", "", "__", "", "`", "").Replace(s)
}

// inline spans

type inline struct {
	text                     string
	bold, italic, code, link bool
}

// parseInline handles **bold**, *italic*, _italic_, `code` and [links](url).
func parseInline(s string) []inline {
	var out []inline
	var cur strings.Builder
	bold, italic := false, false
	emit := func() {
		if cur.Len() > 0 {
			out = append(out, inline{text: cur.String(), bold: bold, italic: italic})
			cur.Reset()
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '`':
			if j := indexRune(rs, i+1, '`'); j > i {
				emit()
				out = append(out, inline{text: string(rs[i+1 : j]), code: true})
				i = j
				continue
			}
		case (c == '*' || c == '_') && i+1 < len(rs) && rs[i+1] == c:
			emit()
			bold = !bold
			i++
			continue
		case c == '*' || (c == '_' && (i == 0 || !isWord(rs[i-1]) || (i+1 < len(rs) && !isWord(rs[i+1])))):
			// Lone '*' followed by a space is a literal (e.g. "2 * 3").
			if c == '*' && i+1 < len(rs) && rs[i+1] == ' ' && !italic {
				break
			}
			emit()
			italic = !italic
			continue
		case c == '[':
			if j := indexRune(rs, i+1, ']'); j > i && j+1 < len(rs) && rs[j+1] == '(' {
				if k := indexRune(rs, j+2, ')'); k > j {
					emit()
					out = append(out, inline{text: string(rs[i+1 : j]), link: true, bold: bold})
					i = k
					continue
				}
			}
		}
		cur.WriteRune(c)
	}
	emit()
	return out
}

func indexRune(rs []rune, from int, r rune) int {
	for i := from; i < len(rs); i++ {
		if rs[i] == r {
			return i
		}
	}
	return -1
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// ---- rendering ----------------------------------------------------------------

const bodySize = unit.Sp(15.5)

func (th *Theme) spans(src []inline, size unit.Sp, col color.NRGBA, weight font.Weight) []richtext.SpanStyle {
	out := make([]richtext.SpanStyle, 0, len(src))
	for _, in := range src {
		f := font.Font{Typeface: uiFace, Weight: weight}
		c := col
		sz := size
		if in.bold {
			f.Weight = font.SemiBold
			if weight >= font.SemiBold {
				f.Weight = font.Bold
			}
		}
		if in.italic {
			f.Style = font.Italic
		}
		if in.code {
			f = th.Mono
			c = th.Pal.InlineCode
			sz = size - 1
		}
		if in.link {
			c = th.Pal.AccentHi
		}
		out = append(out, richtext.SpanStyle{Font: f, Size: sz, Color: c, Content: in.text})
	}
	return out
}

// Markdown holds the parsed form and widget state of one message.
type Markdown struct {
	pal    Palette
	src    string
	blocks []mdBlock
	spans  [][]richtext.SpanStyle // per block (or per list item, flattened)
	texts  []richtext.InteractiveText
	copies []widget.Clickable
	copied []time.Time
	hlists []layout.List
	think  []widget.Clickable
	expand []bool
}

// Set updates the source, re-parsing only when it changed.
func (m *Markdown) Set(th *Theme, src string) {
	if src == m.src && m.blocks != nil && m.pal == th.Pal {
		return
	}
	m.src, m.pal = src, th.Pal
	m.blocks = parseMarkdown(src)
	m.spans = m.spans[:0]
	for _, b := range m.blocks {
		switch b.kind {
		case bPara:
			m.spans = append(m.spans, th.spans(parseInline(b.text), bodySize, th.Pal.Text, font.Normal))
		case bHeading:
			size := map[int]unit.Sp{1: 21, 2: 18.5, 3: 16.5}[b.level]
			if size == 0 {
				size = 15.5
			}
			m.spans = append(m.spans, th.spans(parseInline(b.text), size, th.Pal.Text, font.SemiBold))
		case bQuote:
			m.spans = append(m.spans, th.spans(parseInline(b.text), bodySize, th.Pal.TextMuted, font.Normal))
		case bList:
			for _, it := range b.items {
				m.spans = append(m.spans, th.spans(parseInline(it.text), bodySize, th.Pal.Text, font.Normal))
			}
		case bCode:
			m.spans = append(m.spans, highlight(th, b.text, b.lang))
		case bTable:
			m.spans = append(m.spans, []richtext.SpanStyle{{Font: th.Mono, Size: 13, Color: th.Pal.Text, Content: b.text}})
		case bThink:
			m.spans = append(m.spans, th.spans(parseInline(b.text), 14, th.Pal.TextFaint, font.Normal))
		}
	}
	grow := func(n int) {
		for len(m.texts) < n {
			m.texts = append(m.texts, richtext.InteractiveText{})
		}
	}
	grow(len(m.spans))
	for len(m.copies) < len(m.blocks) {
		m.copies = append(m.copies, widget.Clickable{})
		m.copied = append(m.copied, time.Time{})
		m.hlists = append(m.hlists, layout.List{Axis: layout.Horizontal})
		m.think = append(m.think, widget.Clickable{})
		m.expand = append(m.expand, false)
	}
}

func (m *Markdown) Layout(gtx C, th *Theme) D {
	var children []layout.FlexChild
	si := 0
	for bi := range m.blocks {
		b := m.blocks[bi]
		bi := bi
		if bi > 0 {
			gap := unit.Dp(10)
			if b.kind == bHeading {
				gap = 16
			}
			children = append(children, layout.Rigid(Gap(gap)))
		}
		switch b.kind {
		case bPara, bHeading:
			idx := si
			si++
			children = append(children, layout.Rigid(func(gtx C) D { return m.rich(gtx, th, idx) }))
		case bQuote:
			idx := si
			si++
			children = append(children, layout.Rigid(func(gtx C) D {
				return layout.Background{}.Layout(gtx,
					func(gtx C) D {
						fillRRect(gtx, image.Pt(gtx.Dp(3), gtx.Constraints.Min.Y), gtx.Dp(2), th.Pal.BorderStrong)
						return D{Size: gtx.Constraints.Min} // must match, or Background re-centres the text
					},
					func(gtx C) D {
						return layout.Inset{Left: 15}.Layout(gtx, func(gtx C) D { return m.rich(gtx, th, idx) })
					})
			}))
		case bList:
			first := si
			si += len(b.items)
			children = append(children, layout.Rigid(func(gtx C) D { return m.list(gtx, th, b, first) }))
		case bCode, bTable:
			idx := si
			si++
			children = append(children, layout.Rigid(func(gtx C) D { return m.code(gtx, th, b, bi, idx) }))
		case bRule:
			children = append(children, layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: 4, Bottom: 4}.Layout(gtx, func(gtx C) D { return Divider(gtx, th.Pal.BorderStrong) })
			}))
		case bThink:
			idx := si
			si++
			children = append(children, layout.Rigid(func(gtx C) D { return m.thinkBlock(gtx, th, b, bi, idx) }))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (m *Markdown) rich(gtx C, th *Theme, idx int) D {
	t := richtext.Text(&m.texts[idx], th.Shaper, m.spans[idx]...)
	t.LineHeightScale = 1.42
	return t.Layout(gtx)
}

func (m *Markdown) list(gtx C, th *Theme, b mdBlock, first int) D {
	var rows []layout.FlexChild
	for i, it := range b.items {
		i, it := i, it
		if i > 0 {
			rows = append(rows, layout.Rigid(Gap(6)))
		}
		rows = append(rows, layout.Rigid(func(gtx C) D {
			return layout.Inset{Left: unit.Dp(4 + 18*min(it.indent, 4))}.Layout(gtx, func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Dp(22)
						col := th.Pal.TextMuted
						l := Lbl(th, bodySize, col, font.Medium, it.marker)
						l.LineHeightScale = 1.42
						return l.Layout(gtx)
					}),
					layout.Flexed(1, func(gtx C) D { return m.rich(gtx, th, first+i) }),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (m *Markdown) code(gtx C, th *Theme, b mdBlock, bi, idx int) D {
	if m.copies[bi].Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(b.text))})
		m.copied[bi] = gtx.Now
	}
	label := b.lang
	if label == "" {
		label = "code"
	}
	if b.kind == bTable {
		label = "table"
	}
	return Surface{Bg: th.Pal.CodeBg, Border: th.Pal.BorderStrong, Radius: 12}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: 14, Right: 4, Top: 2, Bottom: 2}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							s := gtx.Dp(15)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							return ic.Code.Layout(gtx, th.Pal.TextFaint)
						}),
						layout.Rigid(Gap(7)),
						layout.Flexed(1, Lbl(th, 12.5, th.Pal.TextMuted, font.Medium, label).Layout),
						layout.Rigid(func(gtx C) D {
							txt := "Copy"
							if gtx.Now.Sub(m.copied[bi]) < 1800*time.Millisecond {
								txt = "Copied"
								gtx.Execute(op.InvalidateCmd{At: m.copied[bi].Add(1800 * time.Millisecond)})
							}
							return m.copies[bi].Layout(gtx, func(gtx C) D {
								return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
									return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx C) D {
											s := gtx.Dp(14)
											gtx.Constraints = layout.Exact(image.Pt(s, s))
											icon := ic.Copy
											if txt == "Copied" {
												icon = ic.Done
											}
											return icon.Layout(gtx, th.Pal.TextMuted)
										}),
										layout.Rigid(Gap(5)),
										layout.Rigid(Lbl(th, 12, th.Pal.TextMuted, font.Medium, txt).Layout),
									)
								})
							})
						}),
					)
				})
			}),
			layout.Rigid(func(gtx C) D { return Divider(gtx, th.Pal.Border) }),
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Top: 12, Bottom: 14}.Layout(gtx, func(gtx C) D {
					return m.hlists[bi].Layout(gtx, 1, func(gtx C, _ int) D {
						return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx C) D {
							gtx.Constraints.Max.X = 1 << 20 // no wrapping; scroll sideways
							t := richtext.Text(&m.texts[idx], th.Shaper, m.spans[idx]...)
							t.LineHeightScale = 1.35
							return t.Layout(gtx)
						})
					})
				})
			}),
		)
	})
}

func (m *Markdown) thinkBlock(gtx C, th *Theme, b mdBlock, bi, idx int) D {
	if m.think[bi].Clicked(gtx) {
		m.expand[bi] = !m.expand[bi]
	}
	title := "Reasoning"
	if b.open {
		title = "Thinking…"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return m.think[bi].Layout(gtx, func(gtx C) D {
				return layout.Inset{Top: 4, Bottom: 4}.Layout(gtx, func(gtx C) D {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(Lbl(th, 13.5, th.Pal.TextMuted, font.Medium, title).Layout),
						layout.Rigid(func(gtx C) D {
							s := gtx.Dp(18)
							gtx.Constraints = layout.Exact(image.Pt(s, s))
							return ic.ChevronDown.Layout(gtx, th.Pal.TextFaint)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx C) D {
			if !m.expand[bi] && !b.open {
				return D{}
			}
			return layout.Inset{Top: 4, Left: 12}.Layout(gtx, func(gtx C) D { return m.rich(gtx, th, idx) })
		}),
	)
}

// ---- syntax highlighting ----------------------------------------------------

var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var
	and as assert async await class def del elif except finally from global in is lambda nonlocal not or pass raise try while with yield None True False self
	function let new this typeof instanceof void delete throw catch extends export implements static public private protected readonly enum null undefined true false
	auto char double float int long short signed unsigned sizeof typedef union volatile include define namespace template typename using virtual override
	fn impl mut pub use mod crate trait where match loop ref move dyn
	echo then fi do done esac local function
	SELECT FROM WHERE INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE JOIN LEFT RIGHT INNER ON GROUP BY ORDER LIMIT AND OR NOT AS
	val fun object when is package import override data sealed suspend
	bool string byte rune error uint int64 int32 float64 float32 str list dict tuple`) {
		keywords[k] = true
	}
}

func hashComments(lang string) bool {
	switch lang {
	case "python", "py", "bash", "sh", "shell", "zsh", "ruby", "rb", "yaml", "yml", "toml", "r", "perl", "dockerfile", "makefile", "ini", "conf":
		return true
	}
	return false
}

// highlight tokenizes code into coloured spans (a pragmatic lexer covering
// comments, strings, numbers, keywords, types and calls).
func highlight(th *Theme, code, lang string) []richtext.SpanStyle {
	if code == "" {
		code = " "
	}
	var out []richtext.SpanStyle
	add := func(s string, c color.NRGBA, it bool) {
		f := th.Mono
		if it {
			f.Style = font.Italic
		}
		if n := len(out); n > 0 && out[n-1].Color == c && out[n-1].Font == f {
			out[n-1].Content += s
			return
		}
		out = append(out, richtext.SpanStyle{Font: f, Size: 13, Color: c, Content: s})
	}
	hash := hashComments(lang)
	rs := []rune(code)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case c == '/' && i+1 < len(rs) && rs[i+1] == '/' && !hash,
			c == '#' && hash,
			c == '-' && i+1 < len(rs) && rs[i+1] == '-' && (lang == "sql" || lang == "lua"):
			j := i
			for j < len(rs) && rs[j] != '\n' {
				j++
			}
			add(string(rs[i:j]), pal.SynComment, true)
			i = j
		case c == '/' && i+1 < len(rs) && rs[i+1] == '*':
			j := i + 2
			for j+1 < len(rs) && !(rs[j] == '*' && rs[j+1] == '/') {
				j++
			}
			j = min(j+2, len(rs))
			add(string(rs[i:j]), pal.SynComment, true)
			i = j
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			for j < len(rs) && rs[j] != c && !(rs[j] == '\n' && c != '`') {
				if rs[j] == '\\' {
					j++
				}
				j++
			}
			j = min(j+1, len(rs))
			add(string(rs[i:j]), pal.SynString, false)
			i = j
		case unicode.IsDigit(c):
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'x' || rs[j] == '_' || (rs[j] >= 'a' && rs[j] <= 'f')) {
				j++
			}
			add(string(rs[i:j]), pal.SynNumber, false)
			i = j
		case unicode.IsLetter(c) || c == '_' || c == '$':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_' || rs[j] == '$') {
				j++
			}
			w := string(rs[i:j])
			col := pal.SynText
			switch {
			case keywords[w]:
				col = pal.SynKeyword
			case j < len(rs) && rs[j] == '(':
				col = pal.SynFunc
			case unicode.IsUpper(rs[i]) && len(w) > 1:
				col = pal.SynType
			}
			add(w, col, false)
			i = j
		default:
			add(string(c), pal.SynText, false)
			i++
		}
	}
	return out
}
