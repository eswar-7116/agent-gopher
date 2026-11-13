package tui

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
)

// Renders markdown
type mdRenderer struct {
	width int
	theme Theme
	r     *glamour.TermRenderer
	cache map[string]string
}

func (md *mdRenderer) render(content string, width int) string {
	t := ActiveTheme()
	if md.r == nil || md.width != width || md.theme != t {
		r, err := glamour.NewTermRenderer(glamour.WithStyles(markdownStyle(t)), glamour.WithWordWrap(width))
		if err != nil {
			return content
		}
		md.r, md.width, md.theme, md.cache = r, width, t, map[string]string{}
	}
	if out, ok := md.cache[content]; ok {
		return out
	}
	out, err := md.r.Render(content)
	if err != nil {
		return content
	}
	out = strings.Trim(out, "\n")
	md.cache[content] = out
	return out
}

// Drops cached entries not in keep
func (md *mdRenderer) prune(keep map[string]bool) {
	for k := range md.cache {
		if !keep[k] {
			delete(md.cache, k)
		}
	}
}

// Builds a glamour style from the dark preset using theme colors
func markdownStyle(t Theme) ansi.StyleConfig {
	s := styles.DarkStyleConfig
	var zero uint
	s.Document.Margin = &zero
	s.Document.Color = nil
	s.Heading.Color = &t.Agent
	s.H1.Color, s.H1.BackgroundColor = nil, nil
	s.H1.Prefix, s.H1.Suffix = "", ""
	s.H1.Underline = boolPtr(true)
	for _, h := range []*ansi.StyleBlock{&s.H2, &s.H3, &s.H4, &s.H5, &s.H6} {
		h.Prefix = ""
	}
	s.H6.Color = nil
	s.Code.Color, s.Code.BackgroundColor = &t.Tool, nil
	s.CodeBlock.Color = &t.Dim
	s.CodeBlock.Chroma, s.CodeBlock.Theme = nil, t.Code
	s.Link.Color, s.LinkText.Color = &t.Title, &t.Title
	s.Image.Color, s.ImageText.Color = &t.Title, &t.Dim
	s.HorizontalRule.Color = &t.Dim
	s.BlockQuote.Color = &t.Dim
	s.Item.Color, s.Enumeration.Color = &t.Dim, &t.Dim
	return s
}

func boolPtr(b bool) *bool { return &b }
